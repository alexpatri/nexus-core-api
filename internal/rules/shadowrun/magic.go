package shadowrun

import (
	"encoding/json"
	"slices"

	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/expr"
)

type magicData struct {
	Awakening       string   `json:"awakening"`
	Tradition       string   `json:"tradition"`
	Aspect          string   `json:"aspect"`
	MentorAdvantage string   `json:"mentorAdvantage"`
	FreeSkills      []string `json:"freeSkills"`
	FreeSkillGroups []string `json:"freeSkillGroups"`
	Formulas        struct {
		Spells       []string `json:"spells"`
		Rituals      []string `json:"rituals"`
		Preparations []string `json:"preparations"`
	} `json:"formulas"`
	ComplexForms     []string    `json:"complexForms"`
	Powers           []powerItem `json:"powers"`
	PowerPointsKarma float64     `json:"powerPointsKarma"`
	Spirits          []struct {
		Key      string  `json:"key"`
		Type     string  `json:"type"`
		Services float64 `json:"services"`
	} `json:"spirits"`
	Sprites []struct {
		Key   string  `json:"key"`
		Type  string  `json:"type"`
		Tasks float64 `json:"tasks"`
	} `json:"sprites"`
}

type powerItem struct {
	Key       string   `json:"key"`
	ID        string   `json:"id"`
	Rating    *float64 `json:"rating"`
	Parameter string   `json:"parameter"`
}

func (p powerItem) rating() int {
	if p.Rating == nil {
		return 1
	}
	return int(*p.Rating)
}

type adeptPower struct {
	ID          string          `json:"id"`
	PowerPoints json.RawMessage `json:"powerPoints"`
	Parameter   *struct {
		Kind       string   `json:"kind"`
		Options    []string `json:"options"`
		Categories []string `json:"categories"`
		Except     []string `json:"except"`
	} `json:"parameter"`
	Levels *struct {
		Max int `json:"max"`
	} `json:"levels"`
}

// cost em pontos de poder: fixo, por nível, por nível em tabela ou por sentido.
func (p adeptPower) cost(rating int) float64 {
	var fixed float64
	if json.Unmarshal(p.PowerPoints, &fixed) == nil {
		return fixed
	}
	var c struct {
		PerLevel *float64  `json:"perLevel"`
		ByLevel  []float64 `json:"byLevel"`
		Each     *float64  `json:"each"`
	}
	_ = json.Unmarshal(p.PowerPoints, &c)
	switch {
	case c.PerLevel != nil:
		return *c.PerLevel * float64(rating)
	case c.ByLevel != nil:
		if rating < 1 || rating > len(c.ByLevel) {
			return 0
		}
		return c.ByLevel[rating-1]
	case c.Each != nil:
		return *c.Each * float64(rating)
	}
	return 0
}

// leveled diz se o poder tem nível; poder de custo fixo não tem.
func (p adeptPower) leveled() bool {
	var fixed float64
	return json.Unmarshal(p.PowerPoints, &fixed) != nil
}

var (
	aspects          = []string{"sorcery", "conjuring", "enchanting"}
	mentorAdvantages = []string{"magician", "adept"}
	limitIDs         = []string{"physical", "mental", "social"}
)

// Campos de magia que cada despertar usa; o resto é magic.notAllowed.
var magicFields = map[string][]string{
	"magician":          {"tradition", "freeSkills", "formulas", "spirits"},
	"mystic-adept":      {"tradition", "mentorAdvantage", "freeSkills", "formulas", "spirits", "powers", "powerPointsKarma"},
	"aspected-magician": {"tradition", "aspect", "freeSkillGroups", "formulas", "spirits"},
	"adept":             {"freeSkills", "powers"},
	"technomancer":      {"freeSkills", "complexForms", "sprites"},
}

// usedFields são os campos de magia preenchidos, na ordem da spec.
func (m magicData) usedFields() []string {
	used := map[string]bool{
		"tradition":        m.Tradition != "",
		"aspect":           m.Aspect != "",
		"mentorAdvantage":  m.MentorAdvantage != "",
		"freeSkills":       len(m.FreeSkills) > 0,
		"freeSkillGroups":  len(m.FreeSkillGroups) > 0,
		"formulas":         m.formulaCount() > 0,
		"complexForms":     len(m.ComplexForms) > 0,
		"powers":           len(m.Powers) > 0,
		"powerPointsKarma": m.PowerPointsKarma > 0,
		"spirits":          len(m.Spirits) > 0,
		"sprites":          len(m.Sprites) > 0,
	}
	var out []string
	for _, f := range []string{"tradition", "aspect", "mentorAdvantage", "freeSkills", "freeSkillGroups",
		"formulas", "complexForms", "powers", "powerPointsKarma", "spirits", "sprites"} {
		if used[f] {
			out = append(out, f)
		}
	}
	return out
}

func (m magicData) formulaCount() int {
	return len(m.Formulas.Spells) + len(m.Formulas.Rituals) + len(m.Formulas.Preparations)
}

func (m magicData) has(field string) bool {
	return slices.Contains(magicFields[m.Awakening], field)
}

// entry é o que a letra de magia dá ao despertar escolhido.
func (s *System) entry(d *sheet) (magicEntry, bool) {
	row, ok := s.priorities[d.Priorities["magic"]]
	if !ok {
		return magicEntry{}, false
	}
	for _, e := range row.Magic {
		if e.Awakening == d.Magic.Awakening {
			return e, true
		}
	}
	return magicEntry{}, false
}

// specialBase é o valor de Magia ou Ressonância antes dos pontos e do carma.
func (s *System) specialBase(d *sheet, attr string) (int, bool) {
	if s.awakenings[d.Magic.Awakening] != attr {
		return 0, false
	}
	e, _ := s.entry(d)
	return e.Attribute.Value, true
}

// freeRatings são os níveis que a prioridade dá às perícias escolhidas.
func (s *System) freeRatings(d *sheet) map[string]int {
	r := map[string]int{}
	e, ok := s.entry(d)
	if !ok {
		return r
	}
	if e.Skills != nil {
		for _, id := range d.Magic.FreeSkills {
			r[id] += e.Skills.Rating
		}
	}
	if e.SkillGroups != nil {
		for _, g := range d.Magic.FreeSkillGroups {
			for _, id := range s.groups[g] {
				r[id] += e.SkillGroups.Rating
			}
		}
	}
	return r
}

func (s *System) hasAstralPerception(d *sheet) bool {
	switch d.Magic.Awakening {
	case "magician", "aspected-magician":
		return true
	case "adept", "mystic-adept":
		return slices.ContainsFunc(d.Magic.Powers, func(p powerItem) bool { return p.ID == "astral-perception" })
	}
	return false
}

// magicCosts soma os orçamentos de magia e o carma que eles gastam.
func (s *System) magicCosts(d *sheet, c *calc) (karma int) {
	m, p := d.Magic, s.mech.Creation.Purchases
	e, _ := s.entry(d)
	if m.has("formulas") || m.formulaCount() > 0 {
		c.budgets["formulas"] = budget{Total: float64(e.Formulas), Spent: float64(m.formulaCount())}
		karma += max(0, m.formulaCount()-e.Formulas) * p.Spell.Karma
	}
	if m.Awakening == "technomancer" || len(m.ComplexForms) > 0 {
		c.budgets["complexForms"] = budget{Total: float64(e.ComplexForms), Spent: float64(len(m.ComplexForms))}
		karma += max(0, len(m.ComplexForms)-e.ComplexForms) * p.ComplexForm.Karma
	}
	if m.Awakening == "adept" || m.Awakening == "mystic-adept" {
		spent := 0.0
		for _, it := range m.Powers {
			if pw, ok := s.powers[it.ID]; ok {
				spent += pw.cost(it.rating())
			}
		}
		total := m.PowerPointsKarma
		if m.Awakening == "adept" {
			total = float64(c.natural["mag"])
		}
		c.budgets["powerPoints"] = budget{Total: total, Spent: spent}
	}
	karma += int(m.PowerPointsKarma) * p.PowerPoint.Karma
	for _, x := range m.Spirits {
		karma += int(x.Services) * p.BoundSpiritService.Karma
	}
	for _, x := range m.Sprites {
		karma += int(x.Tasks) * p.RegisteredSpriteTask.Karma
	}
	return karma
}

func magicPath(field string) string { return "magic." + field }

func (s *System) strictMagic(d *sheet, c *calc) []rules.Violation {
	var vs []rules.Violation
	m := d.Magic
	add := func(path, code string, params map[string]any) {
		vs = append(vs, violation(path, code, params))
	}
	if m.Awakening == "" {
		for _, f := range m.usedFields() {
			add(magicPath(f), "magic.notAllowed", map[string]any{"field": f, "awakening": ""})
		}
		return vs
	}
	e, atPriority := s.entry(d)
	if l := d.Priorities["magic"]; l != "" && !atPriority {
		add("magic.awakening", "magic.notAtPriority", map[string]any{"awakening": m.Awakening, "priority": l})
	}
	switch m.Awakening {
	case "magician", "mystic-adept", "aspected-magician":
		if m.Tradition == "" {
			add("magic.tradition", "required", map[string]any{"field": "magic.tradition"})
		}
	}
	if m.Awakening == "aspected-magician" && m.Aspect == "" {
		add("magic.aspect", "required", map[string]any{"field": "magic.aspect"})
	}
	mentor := slices.ContainsFunc(d.Qualities, func(q qualityItem) bool { return q.ID == "mentor-spirit" })
	if m.Awakening == "mystic-adept" && mentor && m.MentorAdvantage == "" {
		add("magic.mentorAdvantage", "required", map[string]any{"field": "magic.mentorAdvantage"})
	}
	for _, f := range m.usedFields() {
		if !m.has(f) {
			add(magicPath(f), "magic.notAllowed", map[string]any{"field": f, "awakening": m.Awakening})
		}
	}

	if atPriority {
		vs = append(vs, s.strictFreeSkills(d, e)...)
	}

	env := c.env()
	if m.has("formulas") {
		limit := int(evalOr(s.mech.Creation.Purchases.Spell.Max, env))
		for _, g := range []struct {
			name string
			ids  []string
		}{{"spells", m.Formulas.Spells}, {"rituals", m.Formulas.Rituals}, {"preparations", m.Formulas.Preparations}} {
			if len(g.ids) > limit {
				add("magic.formulas."+g.name, "magic.formulasMax", map[string]any{"group": g.name, "max": limit})
			}
		}
	}
	if m.has("complexForms") {
		if limit := int(evalOr(s.mech.Creation.Purchases.ComplexForm.Max, env)); len(m.ComplexForms) > limit {
			add("magic.complexForms", "magic.complexFormsMax", map[string]any{"max": limit})
		}
	}
	if m.has("powers") {
		mag := c.natural["mag"]
		for _, it := range m.Powers {
			pw, ok := s.powers[it.ID]
			if !ok {
				continue
			}
			p := "magic.powers[" + it.Key + "]"
			if pw.leveled() {
				limit := mag
				if pw.Levels != nil {
					limit = min(limit, pw.Levels.Max)
				}
				if it.rating() > limit {
					add(p+".rating", "magic.powerLevelMax", map[string]any{"power": it.ID, "max": limit})
				}
			}
			if pw.Parameter == nil {
				continue
			}
			switch {
			case it.Parameter == "":
				add(p+".parameter", "magic.parameterRequired", map[string]any{"power": it.ID})
			case !s.parameterAllowed(pw, it.Parameter):
				add(p+".parameter", "magic.parameterNotAllowed", map[string]any{"power": it.ID, "parameter": it.Parameter})
			}
		}
	}
	cha := c.natural["cha"]
	if m.has("spirits") && len(m.Spirits) > cha {
		add("magic.spirits", "magic.spiritsMax", map[string]any{"max": cha})
	}
	if m.has("sprites") && len(m.Sprites) > cha {
		add("magic.sprites", "magic.spiritsMax", map[string]any{"max": cha})
	}
	return vs
}

func evalOr(f any, env expr.Env) float64 {
	v, _ := expr.Eval(f, env)
	return v
}

func (s *System) strictFreeSkills(d *sheet, e magicEntry) []rules.Violation {
	var vs []rules.Violation
	m := d.Magic
	rule, chosen, field := e.Skills, m.FreeSkills, "freeSkills"
	if m.Awakening == "aspected-magician" {
		rule, chosen, field = e.SkillGroups, m.FreeSkillGroups, "freeSkillGroups"
	}
	expected := 0
	if rule != nil {
		expected = rule.Count
	}
	if len(chosen) != expected {
		vs = append(vs, violation(magicPath(field), "magic.freeSkillsCount",
			map[string]any{"expected": expected, "got": len(chosen)}))
	}
	if rule == nil {
		return vs
	}
	for _, id := range chosen {
		ok := true
		if field == "freeSkillGroups" {
			ok = slices.Contains(rule.From.Groups, id) && (m.Aspect == "" || id == m.Aspect)
		} else {
			sk := s.skills[id]
			switch {
			case rule.From.Attribute != "":
				ok = sk.Attribute == rule.From.Attribute
			case rule.From.Groups != nil:
				ok = slices.Contains(rule.From.Groups, sk.Group)
			}
		}
		if !ok {
			vs = append(vs, violation(magicPath(field), "magic.freeSkillNotAllowed", map[string]any{"skill": id}))
		}
	}
	return vs
}

func (s *System) parameterAllowed(pw adeptPower, value string) bool {
	p := pw.Parameter
	var pool []string
	switch p.Kind {
	case "attribute":
		pool = p.Options
	case "limit":
		pool = limitIDs
	case "skill":
		if p.Options != nil {
			pool = p.Options
		} else {
			for _, id := range sortedKeys(s.skills) {
				if slices.Contains(p.Categories, s.skills[id].Category) {
					pool = append(pool, id)
				}
			}
		}
	}
	return slices.Contains(pool, value) && !slices.Contains(p.Except, value)
}

// restrictedAllowed diz se o despertar pode ter nível numa perícia restrita.
func (s *System) restrictedAllowed(d *sheet, id string) bool {
	switch s.skills[id].Category {
	case "magical":
		switch d.Magic.Awakening {
		case "magician", "mystic-adept":
			return true
		case "aspected-magician":
			return slices.Contains(s.groups[d.Magic.Aspect], id)
		}
	case "resonance":
		return d.Magic.Awakening == "technomancer"
	}
	return false
}
