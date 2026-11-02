package shadowrun

import (
	"encoding/json"
	"fmt"
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
	Qualities  []qualityItem     `json:"qualities"`
	Skills     skillsData        `json:"skills"`
	Magic      magicData         `json:"magic"`
	Resources  resourcesData     `json:"resources"`
}

type qualityItem struct {
	Key     string            `json:"key"`
	ID      string            `json:"id"`
	Rating  *float64          `json:"rating"`
	Option  string            `json:"option"`
	Options map[string]string `json:"options"`
	Choice  string            `json:"choice"`
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
	case (strings.HasPrefix(path, "attributes.") || strings.HasPrefix(path, "skills.")) &&
		strings.HasSuffix(path, ".karma"):
		return "karma"
	case strings.HasPrefix(path, "magic.spirits"), strings.HasPrefix(path, "magic.sprites"),
		strings.HasPrefix(path, "magic.powerPointsKarma"):
		return "karma"
	case strings.HasPrefix(path, "magic"):
		return "magic"
	case strings.HasPrefix(path, "qualities"):
		return "qualities"
	case strings.HasPrefix(path, "skills"):
		return "skills"
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

// list confere uma lista de itens com key: cada item é objeto, tem key
// string, e a key não se repete em nenhuma lista da ficha. item recebe o path
// do item ("lista[key]") e o objeto.
func list(path string, v any, vs *[]rules.Violation, keys map[string]bool, item func(path string, m map[string]any)) {
	xs, ok := v.([]any)
	if !ok {
		*vs = append(*vs, shape(path, "array"))
		return
	}
	for i, x := range xs {
		m, ok := x.(map[string]any)
		if !ok {
			*vs = append(*vs, shape(fmt.Sprintf("%s[%d]", path, i), "object"))
			continue
		}
		key, ok := m["key"].(string)
		if !ok || key == "" {
			*vs = append(*vs, shape(fmt.Sprintf("%s[%d].key", path, i), "string"))
			continue
		}
		p := path + "[" + key + "]"
		if keys[key] {
			*vs = append(*vs, violation(p, "key.duplicate", map[string]any{"key": key}))
		}
		keys[key] = true
		item(p, m)
	}
}

func expectString(path string, v any, vs *[]rules.Violation) {
	if _, ok := v.(string); !ok {
		*vs = append(*vs, shape(path, "string"))
	}
}

func expectInteger(path string, v any, vs *[]rules.Violation) {
	if !isInteger(v) {
		*vs = append(*vs, shape(path, "integer"))
	}
}

func unknownID(path, catalog, id string) rules.Violation {
	return violation(path, "catalog.unknownId", map[string]any{"catalog": catalog, "id": id})
}

func (s *System) lenientQualities(path string, v any, vs *[]rules.Violation, keys map[string]bool) {
	list(path, v, vs, keys, func(path string, m map[string]any) {
		id, _ := m["id"].(string)
		q, known := s.qualities[id]
		cost := q.cost()
		object(path, m, vs, func(p, key string, v any) bool {
			switch key {
			case "key", "notes":
				expectString(p, v, vs)
			case "id":
				if _, ok := v.(string); !ok {
					*vs = append(*vs, shape(p, "string"))
				} else if !known {
					*vs = append(*vs, unknownID(p, "qualities", id))
				}
			case "rating":
				expectInteger(p, v, vs)
			case "choice":
				expectString(p, v, vs)
			case "option":
				opt, ok := v.(string)
				if !ok {
					*vs = append(*vs, shape(p, "string"))
				} else if known && cost.options != nil {
					if _, found := optionKarma(cost.options, opt); !found {
						*vs = append(*vs, unknownID(p, "qualityOptions", id+"."+opt))
					}
				}
			case "options":
				object(p, v, vs, func(p, group string, v any) bool {
					opt, ok := v.(string)
					if !ok {
						*vs = append(*vs, shape(p, "string"))
						return true
					}
					for _, g := range cost.sum {
						if g.ID == group {
							if _, found := optionKarma(g.Options, opt); !found {
								*vs = append(*vs, unknownID(p, "qualityOptions", id+"."+opt))
							}
							return true
						}
					}
					return !known
				})
			default:
				return false
			}
			return true
		})
		if _, ok := m["id"]; !ok {
			*vs = append(*vs, shape(path+".id", "string"))
		}
	})
}

func idList(path string, v any, vs *[]rules.Violation, catalog string, known []string) {
	xs, ok := v.([]any)
	if !ok {
		*vs = append(*vs, shape(path, "array"))
		return
	}
	for _, x := range xs {
		id, ok := x.(string)
		switch {
		case !ok:
			*vs = append(*vs, shape(path, "string"))
		case !slices.Contains(known, id):
			*vs = append(*vs, unknownID(path, catalog, id))
		}
	}
}

func (s *System) lenientMagic(path string, v any, vs *[]rules.Violation, keys map[string]bool) {
	object(path, v, vs, func(p, key string, v any) bool {
		switch key {
		case "awakening":
			if id, ok := v.(string); !ok {
				*vs = append(*vs, shape(p, "string"))
			} else if _, known := s.awakenings[id]; !known {
				*vs = append(*vs, unknownID(p, "awakenings", id))
			}
		case "tradition":
			if id, ok := v.(string); !ok {
				*vs = append(*vs, shape(p, "string"))
			} else if !slices.Contains(s.magicIDs["traditions"], id) {
				*vs = append(*vs, unknownID(p, "traditions", id))
			}
		case "aspect", "mentorAdvantage":
			allowed := aspects
			if key == "mentorAdvantage" {
				allowed = mentorAdvantages
			}
			if a, _ := v.(string); !slices.Contains(allowed, a) {
				*vs = append(*vs, shape(p, key))
			}
		case "freeSkills":
			idList(p, v, vs, "skills", sortedKeys(s.skills))
		case "freeSkillGroups":
			idList(p, v, vs, "skillGroups", sortedKeys(s.groups))
		case "complexForms":
			idList(p, v, vs, "complexForms", s.magicIDs["complexForms"])
		case "formulas":
			object(p, v, vs, func(p, group string, v any) bool {
				if group != "spells" && group != "rituals" && group != "preparations" {
					return false
				}
				idList(p, v, vs, group, s.magicIDs[group])
				return true
			})
		case "powerPointsKarma":
			expectInteger(p, v, vs)
		case "powers":
			list(p, v, vs, keys, func(path string, m map[string]any) {
				object(path, m, vs, func(p, key string, v any) bool {
					switch key {
					case "key", "parameter":
						expectString(p, v, vs)
					case "id":
						if id, ok := v.(string); !ok {
							*vs = append(*vs, shape(p, "string"))
						} else if _, known := s.powers[id]; !known {
							*vs = append(*vs, unknownID(p, "adeptPowers", id))
						}
					case "rating":
						expectInteger(p, v, vs)
					default:
						return false
					}
					return true
				})
			})
		case "spirits", "sprites":
			catalog, count := "spiritTypes", "services"
			if key == "sprites" {
				catalog, count = "spriteTypes", "tasks"
			}
			list(p, v, vs, keys, func(path string, m map[string]any) {
				object(path, m, vs, func(p, key string, v any) bool {
					switch key {
					case "key":
						expectString(p, v, vs)
					case "type":
						if id, ok := v.(string); !ok {
							*vs = append(*vs, shape(p, "string"))
						} else if !slices.Contains(s.magicIDs[catalog], id) {
							*vs = append(*vs, unknownID(p, catalog, id))
						}
					case count:
						expectInteger(p, v, vs)
					default:
						return false
					}
					return true
				})
			})
		default:
			return false
		}
		return true
	})
}

// skillField confere pontos, carma e especialização, comuns a todo tipo de perícia.
func skillField(p, key string, v any, vs *[]rules.Violation) bool {
	switch key {
	case "points", "karma":
		expectInteger(p, v, vs)
	case "specialization":
		object(p, v, vs, func(p, key string, v any) bool {
			switch key {
			case "name":
				expectString(p, v, vs)
			case "paidWith":
				if w, _ := v.(string); w != "points" && w != "karma" {
					*vs = append(*vs, shape(p, "paidWith"))
				}
			default:
				return false
			}
			return true
		})
	default:
		return false
	}
	return true
}

func (s *System) lenientSkills(path string, v any, vs *[]rules.Violation, keys map[string]bool) {
	object(path, v, vs, func(path, key string, v any) bool {
		switch key {
		case "active":
			object(path, v, vs, func(p, id string, v any) bool {
				if _, ok := s.skills[id]; !ok {
					*vs = append(*vs, unknownID(p, "skills", id))
				}
				object(p, v, vs, func(p, key string, v any) bool { return skillField(p, key, v, vs) })
				return true
			})
		case "groups":
			object(path, v, vs, func(p, id string, v any) bool {
				if _, ok := s.groups[id]; !ok {
					*vs = append(*vs, unknownID(p, "skillGroups", id))
				}
				object(p, v, vs, func(p, key string, v any) bool {
					if key != "points" && key != "karma" {
						return false
					}
					expectInteger(p, v, vs)
					return true
				})
				return true
			})
		case "knowledge":
			list(path, v, vs, keys, func(path string, m map[string]any) {
				object(path, m, vs, func(p, key string, v any) bool {
					switch key {
					case "key", "name":
						expectString(p, v, vs)
					case "category":
						if c, ok := v.(string); !ok {
							*vs = append(*vs, shape(p, "string"))
						} else if _, known := s.knowledge[c]; !known {
							*vs = append(*vs, unknownID(p, "knowledgeCategories", c))
						}
					default:
						return skillField(p, key, v, vs)
					}
					return true
				})
			})
		case "languages":
			list(path, v, vs, keys, func(path string, m map[string]any) {
				object(path, m, vs, func(p, key string, v any) bool {
					switch key {
					case "key", "name":
						expectString(p, v, vs)
					case "native":
						if _, ok := v.(bool); !ok {
							*vs = append(*vs, shape(p, "boolean"))
						}
					default:
						return skillField(p, key, v, vs)
					}
					return true
				})
			})
		default:
			return false
		}
		return true
	})
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
	keys := map[string]bool{}
	object("", root, &vs, func(path, key string, v any) bool {
		switch key {
		case "magic":
			s.lenientMagic(path, v, &vs, keys)
		case "qualities":
			s.lenientQualities(path, v, &vs, keys)
		case "skills":
			s.lenientSkills(path, v, &vs, keys)
		case "resources":
			s.lenientResources(path, v, &vs, keys)
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
		b := d.Attributes[a]
		if _, awakened := s.specialBase(d, a); !awakened {
			if b.Points+b.Karma > 0 {
				vs = append(vs, violation(layer(a, b), "attributes.notAllowed", map[string]any{"attribute": a}))
			}
			continue
		}
		limit := s.mech.Creation.SpecialAttributeMax[a] + c.effects.attrMax[a] - c.loss[a]
		if v := c.natural[a]; v > limit {
			vs = append(vs, violation(layer(a, b), "attributes.aboveMax",
				map[string]any{"attribute": a, "value": v, "max": limit}))
		}
	}
	if m, ok := s.metatypes[d.Metatype]; ok {
		var atMax []string
		for _, a := range slices.Concat(baseAttrs, []string{"edg"}) {
			v, limit := c.natural[a], m.Attributes[a].Max+c.effects.attrMax[a]
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

	vs = append(vs, s.strictMagic(d, c)...)
	vs = append(vs, s.strictQualities(d, c)...)
	vs = append(vs, s.strictSkills(d, c)...)
	vs = append(vs, s.strictResources(d, c)...)

	for _, id := range sortedKeys(c.budgets) {
		// Fórmulas e formas além das gratuitas são compradas com carma.
		if id == "formulas" || id == "complexForms" {
			continue
		}
		if b := c.budgets[id]; b.Spent > b.Total {
			vs = append(vs, violation("budgets."+id, "budget.exceeded",
				map[string]any{"budget": id, "total": b.Total, "spent": b.Spent}))
		}
	}
	return vs
}
