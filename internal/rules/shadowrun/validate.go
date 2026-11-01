package shadowrun

import (
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strings"

	"rpg-nexus/api/core/internal/rules"
)

// sheet é o data já conferido pelo Lenient. Números vêm como float64 porque o
// JSON da ficha pode trazer 1.0 para um inteiro.
type sheet struct {
	Identity   map[string]string `json:"identity"`
	Priorities map[string]string `json:"priorities"`
	Metatype   string            `json:"metatype"`
	Attributes map[string]buy    `json:"attributes"`
}

type buy struct {
	Points float64 `json:"points"`
	Karma  float64 `json:"karma"`
}

var identityFields = []string{"name", "concept", "ethnicity", "age", "sex", "height", "weight", "notes"}

func violation(path, code string, params map[string]any) rules.Violation {
	return rules.Violation{Step: stepOf(path), Path: path, Code: code, Params: params}
}

// stepOf é o passo de system.json onde o campo é editado.
func stepOf(path string) string {
	switch {
	case path == "identity.concept":
		return "concept"
	case strings.HasPrefix(path, "identity"):
		return "finishing"
	case strings.HasPrefix(path, "attributes.") && strings.HasSuffix(path, ".karma"):
		return "karma"
	case strings.HasPrefix(path, "priorities"), strings.HasPrefix(path, "metatype"),
		strings.HasPrefix(path, "attributes"):
		return "metatype"
	case strings.HasPrefix(path, "budgets."):
		return budgetSteps[strings.TrimPrefix(path, "budgets.")]
	case strings.HasPrefix(path, "resources.startingNuyenRoll"):
		return "final-calculations"
	case strings.HasPrefix(path, "resources"):
		return "resources"
	}
	return "concept"
}

var budgetSteps = map[string]string{
	"special": "metatype", "attributes": "metatype", "powerPoints": "magic",
	"positiveQualities": "qualities", "negativeQualities": "qualities",
	"skills": "skills", "skillGroups": "skills", "knowledge": "skills",
	"nuyen": "resources", "karmaToNuyen": "resources", "karma": "karma", "contacts": "karma",
}

func shape(path, expected string) rules.Violation {
	return violation(path, "shape.invalid", map[string]any{"expected": expected})
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// object confere que v é objeto e passa cada campo para field, que devolve
// false para campo que o formato não tem.
func object(path string, v any, vs *[]rules.Violation, field func(path, key string, v any) bool) {
	m, ok := v.(map[string]any)
	if !ok {
		*vs = append(*vs, shape(path, "object"))
		return
	}
	for _, k := range sortedKeys(m) {
		if !field(join(path, k), k, m[k]) {
			*vs = append(*vs, shape(join(path, k), "absent"))
		}
	}
}

func isInteger(v any) bool {
	n, ok := v.(float64)
	return ok && n >= 0 && n == math.Trunc(n)
}

// lenient confere forma, ids e chaves. Seção que ainda não tem fatia é campo
// desconhecido.
func (s *System) lenient(data json.RawMessage) ([]rules.Violation, *sheet) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return []rules.Violation{shape("", "object")}, nil
	}
	var vs []rules.Violation
	object("", root, &vs, func(path, key string, v any) bool {
		switch key {
		case "identity":
			object(path, v, &vs, func(path, key string, v any) bool {
				if !slices.Contains(identityFields, key) {
					return false
				}
				if _, ok := v.(string); !ok {
					vs = append(vs, shape(path, "string"))
				}
				return true
			})
		case "priorities":
			object(path, v, &vs, func(path, key string, v any) bool {
				if !slices.Contains(columns, key) {
					return false
				}
				if l, ok := v.(string); !ok || len(l) != 1 || !strings.Contains("ABCDE", l) {
					vs = append(vs, shape(path, "priority"))
				}
				return true
			})
		case "metatype":
			id, ok := v.(string)
			switch {
			case !ok:
				vs = append(vs, shape(path, "string"))
			case s.metatypes[id].ID == "":
				vs = append(vs, violation(path, "catalog.unknownId", map[string]any{"catalog": "metatypes", "id": id}))
			}
		case "attributes":
			object(path, v, &vs, func(path, key string, v any) bool {
				if !slices.Contains(baseAttrs, key) && !slices.Contains(specialAttrs, key) {
					return false
				}
				object(path, v, &vs, func(path, key string, v any) bool {
					if key != "points" && key != "karma" {
						return false
					}
					if !isInteger(v) {
						vs = append(vs, shape(path, "integer"))
					}
					return true
				})
				return true
			})
		default:
			return false
		}
		return true
	})
	if len(vs) > 0 {
		return vs, nil
	}
	var d sheet
	if err := json.Unmarshal(data, &d); err != nil {
		return []rules.Violation{shape("", "object")}, nil
	}
	return nil, &d
}

func (s *System) Validate(data json.RawMessage, mode rules.Mode) []rules.Violation {
	vs, d := s.lenient(data)
	if len(vs) > 0 || mode == rules.Lenient {
		return vs
	}
	return s.strict(d, s.calc(d))
}

func required(field string) rules.Violation {
	return violation(field, "required", map[string]any{"field": field})
}

// layer é o campo de compra mais de fora: o carma sobe por cima dos pontos.
func layer(attr string, b buy) string {
	if b.Karma > 0 {
		return "attributes." + attr + ".karma"
	}
	return "attributes." + attr + ".points"
}

func (s *System) strict(d *sheet, c *calc) []rules.Violation {
	var vs []rules.Violation

	for _, col := range columns {
		if d.Priorities[col] == "" {
			vs = append(vs, required("priorities."+col))
		}
	}
	if d.Metatype == "" {
		vs = append(vs, required("metatype"))
	}
	if strings.TrimSpace(d.Identity["name"]) == "" {
		vs = append(vs, required("identity.name"))
	}
	// resources entra na fatia 4.4; até lá, os dois sempre faltam.
	vs = append(vs, required("resources.lifestyles"), required("resources.startingNuyenRoll"))

	byLetter := map[string][]string{}
	for _, col := range columns {
		if l := d.Priorities[col]; l != "" {
			byLetter[l] = append(byLetter[l], col)
		}
	}
	for _, l := range sortedKeys(byLetter) {
		if cols := byLetter[l]; len(cols) > 1 {
			vs = append(vs, violation("priorities", "priorities.duplicate",
				map[string]any{"priority": l, "columns": cols}))
		}
	}

	if l := d.Priorities["metatype"]; d.Metatype != "" && l != "" && !c.metatypeAtPriority {
		vs = append(vs, violation("metatype", "metatype.notAtPriority",
			map[string]any{"metatype": d.Metatype, "priority": l}))
	}

	for _, a := range []string{"mag", "res"} {
		if b := d.Attributes[a]; b.Points+b.Karma > 0 {
			vs = append(vs, violation(layer(a, b), "attributes.notAllowed", map[string]any{"attribute": a}))
		}
	}
	if m, ok := s.metatypes[d.Metatype]; ok {
		var atMax []string
		for _, a := range slices.Concat(baseAttrs, []string{"edg"}) {
			v, limit := c.natural[a], m.Attributes[a].Max
			if v > limit {
				vs = append(vs, violation(layer(a, d.Attributes[a]), "attributes.aboveMax",
					map[string]any{"attribute": a, "value": v, "max": limit}))
			}
			if a != "edg" && v >= limit {
				atMax = append(atMax, a)
			}
		}
		if len(atMax) > 1 {
			vs = append(vs, violation("attributes", "attributes.tooManyAtMax",
				map[string]any{"attributes": atMax, "allowed": 1}))
		}
	}

	for _, id := range sortedKeys(c.budgets) {
		if b := c.budgets[id]; b.Spent > b.Total {
			vs = append(vs, violation("budgets."+id, "budget.exceeded",
				map[string]any{"budget": id, "total": b.Total, "spent": b.Spent}))
		}
	}
	return vs
}
