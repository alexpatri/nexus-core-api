package shadowrun

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/expr"
)

type budget struct {
	Total int `json:"total"`
	Spent int `json:"spent"`
}

type initiativeOut struct {
	Base float64 `json:"base"`
	Dice int     `json:"dice"`
}

type derived struct {
	Attributes     map[string]attrOut       `json:"attributes,omitempty"`
	Essence        *float64                 `json:"essence,omitempty"`
	Limits         map[string]float64       `json:"limits,omitempty"`
	Initiative     map[string]initiativeOut `json:"initiative,omitempty"`
	Monitors       map[string]float64       `json:"monitors,omitempty"`
	Armor          *float64                 `json:"armor,omitempty"`
	AttributeTests map[string]float64       `json:"attributeTests,omitempty"`
	Movement       map[string]float64       `json:"movement,omitempty"`
	Lifting        map[string]float64       `json:"lifting,omitempty"`
	Reputation     map[string]float64       `json:"reputation"`
	Budgets        map[string]budget        `json:"budgets"`
}

type attrOut struct {
	Natural   int `json:"natural"`
	Augmented int `json:"augmented"`
}

// calc é o que Strict, Derive e Summarize compartilham. O que não dá para
// calcular com o data atual fica ausente.
type calc struct {
	natural            map[string]int
	essence            *float64
	metatypeAtPriority bool
	budgets            map[string]budget
	err                error
}

func (s *System) calc(d *sheet) *calc {
	c := &calc{natural: map[string]int{}, budgets: map[string]budget{}}
	m, known := s.metatypes[d.Metatype]
	eval := func(f any, env expr.Env) int {
		v, err := expr.Eval(f, env)
		if err != nil {
			c.err = errors.Join(c.err, err)
		}
		return int(v)
	}

	if known {
		for _, a := range slices.Concat(baseAttrs, []string{"edg"}) {
			b := d.Attributes[a]
			c.natural[a] = m.Attributes[a].Min + int(b.Points) + int(b.Karma)
		}
		e := float64(m.Attributes["ess"].Start)
		c.essence = &e
	}

	if row, ok := s.priorities[d.Priorities["metatype"]]; ok {
		for _, opt := range row.Metatype {
			if opt.Metatype == d.Metatype {
				c.metatypeAtPriority = true
				spent := 0
				for _, a := range specialAttrs {
					spent += int(d.Attributes[a].Points)
				}
				c.budgets["special"] = budget{Total: opt.SpecialPoints, Spent: spent}
			}
		}
	}
	if row, ok := s.priorities[d.Priorities["attributes"]]; ok {
		spent := 0
		for _, a := range baseAttrs {
			spent += int(d.Attributes[a].Points)
		}
		c.budgets["attributes"] = budget{Total: row.Attributes, Spent: spent}
	}
	if row, ok := s.priorities[d.Priorities["skills"]]; ok {
		c.budgets["skills"] = budget{Total: row.Skills.Points}
		c.budgets["skillGroups"] = budget{Total: row.Skills.GroupPoints}
	}
	if row, ok := s.priorities[d.Priorities["resources"]]; ok {
		c.budgets["nuyen"] = budget{Total: row.Resources}
	}
	if known {
		env := c.env()
		c.budgets["knowledge"] = budget{Total: eval(s.mech.Creation.Skills.KnowledgePoints, env)}
		c.budgets["contacts"] = budget{Total: eval(s.mech.Creation.Contacts.Karma, env)}
	}

	// Carma sobe degrau a degrau a partir do valor com os pontos. Sem metatipo,
	// o degrau inicial dos atributos comuns é desconhecido e o orçamento some.
	karma, priced := 0, true
	for _, a := range slices.Concat(baseAttrs, specialAttrs) {
		b := d.Attributes[a]
		if b.Karma == 0 {
			continue
		}
		from := int(b.Points)
		if a != "mag" && a != "res" {
			if !known {
				priced = false
				break
			}
			from += m.Attributes[a].Min
		}
		for r := from + 1; r <= from+int(b.Karma); r++ {
			karma += eval(s.mech.Progression.Costs.Attribute, expr.Env{"rating": float64(r)})
		}
	}
	if priced {
		c.budgets["karma"] = budget{Total: s.mech.Creation.Karma, Spent: karma}
	}
	return c
}

// env tem attr.<id> com o valor ampliado; sem ampliações na fatia 4.1, é o natural.
func (c *calc) env() expr.Env {
	env := expr.Env{"karma.total": 0}
	for a, v := range c.natural {
		env["attr."+a] = float64(v)
	}
	if c.essence != nil {
		env["attr.ess"] = *c.essence
	}
	return env
}

func (s *System) Derive(data json.RawMessage) (json.RawMessage, error) {
	vs, d := s.lenient(data)
	if len(vs) > 0 {
		return nil, errors.New("shadowrun: data não passa em Lenient")
	}
	c := s.calc(d)
	out := derived{Budgets: c.budgets}
	mech := s.mech.Derived
	env := c.env()
	eval := func(f any) float64 {
		v, err := expr.Eval(f, env)
		if err != nil {
			c.err = errors.Join(c.err, err)
		}
		return v
	}

	out.Reputation = map[string]float64{
		"streetCred":      eval(mech.Reputation.StreetCred.Value),
		"notoriety":       float64(mech.Reputation.Notoriety.Base),
		"publicAwareness": float64(mech.Reputation.PublicAwareness.Base),
	}

	if m, known := s.metatypes[d.Metatype]; known {
		out.Attributes = map[string]attrOut{}
		for a, v := range c.natural {
			out.Attributes[a] = attrOut{Natural: v, Augmented: v}
		}
		out.Essence = c.essence
		out.Limits = map[string]float64{}
		for id, f := range s.mech.limits() {
			out.Limits[id] = eval(f)
		}
		dice := func(n int) int { return min(n, mech.Initiative.MaxDice) }
		out.Initiative = map[string]initiativeOut{
			"physical": {Base: eval(mech.Initiative.Physical.Base), Dice: dice(mech.Initiative.Physical.Dice)},
			"matrixAR": {Base: eval(mech.Initiative.MatrixAR.Base), Dice: dice(mech.Initiative.MatrixAR.Dice)},
		}
		out.Monitors = map[string]float64{
			"physical":     eval(mech.Monitors.Physical.Boxes),
			"stun":         eval(mech.Monitors.Stun.Boxes),
			"overflow":     eval(mech.Monitors.Overflow.Boxes),
			"woundPenalty": 0,
		}
		armor := 0.0
		for _, t := range m.Traits {
			for _, mod := range t.Modifiers {
				if mod.Target == "armor" {
					armor += eval(mod.Add)
				}
			}
		}
		out.Armor = &armor
		out.AttributeTests = map[string]float64{}
		for id, f := range s.mech.attributeTests() {
			out.AttributeTests[id] = eval(f)
		}
		out.Movement = map[string]float64{
			"walk": eval(mech.Movement.Walk), "run": eval(mech.Movement.Run), "sprintPerHit": float64(m.SprintPerHit),
		}
		out.Lifting = map[string]float64{
			"lift": eval(mech.Lifting.Lift), "overhead": eval(mech.Lifting.Overhead), "carry": eval(mech.Lifting.Carry),
		}
	}
	if c.err != nil {
		return nil, c.err
	}
	return json.Marshal(out)
}

// number escreve no formato pt-BR, sem zeros à direita.
func number(x float64) string {
	return strings.Replace(strconv.FormatFloat(x, 'f', -1, 64), ".", ",", 1)
}

func (s *System) Summarize(data json.RawMessage) (rules.Summary, error) {
	vs, d := s.lenient(data)
	if len(vs) > 0 {
		return rules.Summary{}, errors.New("shadowrun: data não passa em Lenient")
	}
	c := s.calc(d)
	if c.err != nil {
		return rules.Summary{}, c.err
	}

	var parts []string
	if m, ok := s.metatypes[d.Metatype]; ok {
		parts = append(parts, m.Name)
	}
	concept := strings.TrimSpace(d.Identity["concept"])
	if concept != "" {
		parts = append(parts, concept)
	}
	sum := rules.Summary{Title: strings.TrimSpace(d.Identity["name"]), Subtitle: "Sem metatipo"}
	if len(parts) > 0 {
		sum.Subtitle = strings.Join(parts, " · ")
	}

	if c.essence != nil {
		sum.Highlights = append(sum.Highlights, rules.Highlight{Label: "Essência", Value: number(*c.essence)})
	}
	if k, ok := c.budgets["karma"]; ok {
		sum.Highlights = append(sum.Highlights, rules.Highlight{Label: "Carma", Value: number(float64(k.Total - k.Spent))})
	}

	// Progresso é o primeiro passo que ainda falta, calculado de data.
	step := len(s.steps)
	if concept == "" {
		step = 1
	} else {
		for _, v := range s.strict(d, c) {
			step = min(step, s.stepOrder(v.Step))
		}
	}
	sum.Progress = &rules.Progress{Step: step, Total: len(s.steps)}
	return sum, nil
}
