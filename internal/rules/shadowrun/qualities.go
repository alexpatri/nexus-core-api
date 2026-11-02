package shadowrun

import (
	"encoding/json"
	"fmt"
	"slices"

	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/expr"
)

type quality struct {
	ID     string          `json:"id"`
	Kind   string          `json:"kind"`
	Karma  json.RawMessage `json:"karma"`
	Levels *struct {
		Max int `json:"max"`
	} `json:"levels"`
	// Ponteiro para o catálogo de onde vem a escolha (tradições, mentores).
	Choice    string              `json:"choice"`
	Modifiers []qualityModifier   `json:"modifiers"`
	Requires  *qualityRequirement `json:"requires"`
	Unique    bool                `json:"unique"`
}

type qualityRequirement struct {
	Metatype []string `json:"metatype"`
	Special  string   `json:"special"`
}

type qualityModifier struct {
	Target string `json:"target"`
	// String com o alvo, ou { choose, except, only } quando o alvo é o choice.
	Of    any `json:"of"`
	Add   any `json:"add"`
	Times any `json:"times"`
}

type qualityOption struct {
	ID    string `json:"id"`
	Karma int    `json:"karma"`
}

// qualityCost é a forma do karma de uma qualidade: um número, por nível, uma
// opção ou a soma de uma opção por grupo.
type qualityCost struct {
	fixed    int
	perLevel int
	options  []qualityOption
	sum      []struct {
		ID      string          `json:"id"`
		Options []qualityOption `json:"options"`
	}
}

func (q quality) cost() qualityCost {
	var c qualityCost
	if json.Unmarshal(q.Karma, &c.fixed) == nil {
		return c
	}
	var k struct {
		PerLevel int             `json:"perLevel"`
		Options  []qualityOption `json:"options"`
		Sum      json.RawMessage `json:"sum"`
	}
	_ = json.Unmarshal(q.Karma, &k)
	c.perLevel, c.options = k.PerLevel, k.Options
	_ = json.Unmarshal(k.Sum, &c.sum)
	return c
}

func optionKarma(opts []qualityOption, id string) (int, bool) {
	for _, o := range opts {
		if o.ID == id {
			return o.Karma, true
		}
	}
	return 0, false
}

func (it qualityItem) rating() int {
	if it.Rating == nil {
		return 1
	}
	return int(*it.Rating)
}

// karma de uma qualidade na ficha; opção que falta vale 0.
func (q quality) karma(it qualityItem) int {
	c := q.cost()
	switch {
	case c.perLevel > 0:
		return c.perLevel * it.rating()
	case c.options != nil:
		k, _ := optionKarma(c.options, it.Option)
		return k
	case c.sum != nil:
		total := 0
		for _, g := range c.sum {
			k, _ := optionKarma(g.Options, it.Options[g.ID])
			total += k
		}
		return total
	}
	return c.fixed
}

// chooser é o modificador cujo alvo é o choice da qualidade, se houver.
func (q quality) chooser() (kind string, except, only []string, ok bool) {
	for _, m := range q.Modifiers {
		sel, isMap := m.Of.(map[string]any)
		if !isMap {
			continue
		}
		kind, _ = sel["choose"].(string)
		return kind, stringList(sel["except"]), stringList(sel["only"]), true
	}
	return "", nil, nil, false
}

func stringList(v any) []string {
	var out []string
	for _, x := range asSlice(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func asSlice(v any) []any {
	xs, _ := v.([]any)
	return xs
}

// needsChoice diz se a qualidade pede choice e de qual conjunto ele sai.
func (s *System) needsChoice(q quality) (allowed []string, ok bool) {
	if q.Choice != "" {
		return s.choices[q.Choice], true
	}
	kind, except, only, found := q.chooser()
	if !found {
		return nil, false
	}
	for _, id := range s.choices[kind] {
		if (len(only) == 0 || slices.Contains(only, id)) && !slices.Contains(except, id) {
			allowed = append(allowed, id)
		}
	}
	return allowed, true
}

// Alvos de modificador de qualidade que a fatia 4.2 aplica; os outros entram
// com as fatias de magia, recursos e Matriz.
var appliedTargets = []string{
	"attr.max", "skill.max", "limit", "language.native", "reputation.notoriety", "monitor.overflow",
}

// effects são os modificadores das qualidades que valem na fatia 4.2.
type effects struct {
	attrMax   map[string]int
	skillMax  map[string]int
	limit     map[string]float64
	natives   int
	notoriety float64
	overflow  float64
	positive  int
	negative  int
}

func (s *System) qualityEffects(d *sheet) (effects, error) {
	e := effects{attrMax: map[string]int{}, skillMax: map[string]int{}, limit: map[string]float64{}}
	var errs error
	for _, it := range d.Qualities {
		q, ok := s.qualities[it.ID]
		if !ok {
			continue
		}
		if q.Kind == "negative" {
			e.negative += q.karma(it)
		} else {
			e.positive += q.karma(it)
		}
		env := expr.Env{"rating": float64(it.rating())}
		allowed, choosable := s.needsChoice(q)
		for _, m := range q.Modifiers {
			if !slices.Contains(appliedTargets, m.Target) {
				continue
			}
			target := ""
			switch of := m.Of.(type) {
			case string:
				target = of
			case map[string]any:
				if !choosable || !slices.Contains(allowed, it.Choice) {
					continue
				}
				target = it.Choice
			}
			add, err := expr.Eval(m.Add, env)
			times := 1.0
			if m.Times != nil {
				t, terr := expr.Eval(m.Times, env)
				times, err = t, firstErr(err, terr)
			}
			if err != nil {
				errs = fmt.Errorf("qualidade %s: %w", q.ID, err)
				continue
			}
			v := add * times
			switch m.Target {
			case "attr.max":
				e.attrMax[target] += int(v)
			case "skill.max":
				e.skillMax[target] += int(v)
			case "limit":
				e.limit[target] += v
			case "language.native":
				e.natives += int(v)
			case "reputation.notoriety":
				e.notoriety += v
			case "monitor.overflow":
				e.overflow += v
			}
		}
	}
	return e, errs
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func qualityPath(it qualityItem, field string) string {
	p := "qualities[" + it.Key + "]"
	if field != "" {
		p += "." + field
	}
	return p
}

func (s *System) strictQualities(d *sheet, c *calc) []rules.Violation {
	var vs []rules.Violation
	add := func(it qualityItem, field, code string, params map[string]any) {
		vs = append(vs, violation(qualityPath(it, field), code, params))
	}
	seen := map[string]bool{}
	for _, it := range d.Qualities {
		q, ok := s.qualities[it.ID]
		if !ok {
			continue
		}
		p := map[string]any{"quality": q.ID}
		if q.Levels != nil && (it.rating() < 1 || it.rating() > q.Levels.Max) {
			add(it, "rating", "qualities.levelOutOfRange", map[string]any{"quality": q.ID, "max": q.Levels.Max})
		}
		cost := q.cost()
		if cost.options != nil {
			if _, found := optionKarma(cost.options, it.Option); !found {
				add(it, "option", "qualities.optionRequired", p)
			}
		}
		for _, g := range cost.sum {
			if _, found := optionKarma(g.Options, it.Options[g.ID]); !found {
				add(it, "options", "qualities.optionRequired", p)
				break
			}
		}
		if allowed, needs := s.needsChoice(q); needs {
			switch {
			case it.Choice == "":
				add(it, "choice", "qualities.choiceRequired", p)
			case !slices.Contains(allowed, it.Choice):
				add(it, "choice", "qualities.choiceNotAllowed", map[string]any{"quality": q.ID, "choice": it.Choice})
			}
		}
		if r := q.Requires; r != nil {
			if len(r.Metatype) > 0 && d.Metatype != "" && !slices.Contains(r.Metatype, d.Metatype) {
				add(it, "", "qualities.requirementNotMet", map[string]any{"quality": q.ID, "requirement": "metatype"})
			}
			if r.Special != "" && c.natural[r.Special] <= 0 {
				add(it, "", "qualities.requirementNotMet", map[string]any{"quality": q.ID, "requirement": r.Special})
			}
		}
		key := q.ID + "\x00" + it.Choice
		switch {
		case q.Unique && seen[q.ID]:
			add(it, "", "qualities.unique", p)
		case !q.Unique && seen[key]:
			add(it, "", "qualities.duplicate", p)
		}
		seen[q.ID], seen[key] = true, true
	}
	return vs
}
