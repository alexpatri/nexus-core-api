package shadowrun

import (
	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/expr"
)

type skill struct {
	ID         string `json:"id"`
	Attribute  string `json:"attribute"`
	Group      string `json:"group"`
	Category   string `json:"category"`
	Restricted bool   `json:"restricted"`
}

type specialization struct {
	Name     string `json:"name"`
	PaidWith string `json:"paidWith"`
}

type skillBuy struct {
	Points         float64         `json:"points"`
	Karma          float64         `json:"karma"`
	Specialization *specialization `json:"specialization"`
}

type knowledgeItem struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	skillBuy
}

type languageItem struct {
	Key    string `json:"key"`
	Native bool   `json:"native"`
	skillBuy
}

type skillsData struct {
	Active    map[string]skillBuy `json:"active"`
	Groups    map[string]buy      `json:"groups"`
	Knowledge []knowledgeItem     `json:"knowledge"`
	Languages []languageItem      `json:"languages"`
}

type ratedOut struct {
	Rating int  `json:"rating,omitempty"`
	Pool   *int `json:"pool,omitempty"`
	Native bool `json:"native,omitempty"`
}

type skillsOut struct {
	Active    map[string]ratedOut `json:"active,omitempty"`
	Knowledge map[string]ratedOut `json:"knowledge,omitempty"`
	Languages map[string]ratedOut `json:"languages,omitempty"`
}

func paidWith(sp *specialization, with string) int {
	if sp != nil && sp.PaidWith == with {
		return 1
	}
	return 0
}

func (s *System) groupRating(d *sheet, skillID string) int {
	g := d.Skills.Groups[s.skills[skillID].Group]
	return int(g.Points + g.Karma)
}

// activeRatings é o nível de cada perícia ativa: dado pela prioridade + grupo
// comprado + pontos + carma.
func (s *System) activeRatings(d *sheet) map[string]int {
	r := s.freeRatings(d)
	for g, b := range d.Skills.Groups {
		for _, id := range s.groups[g] {
			r[id] += int(b.Points + b.Karma)
		}
	}
	for id, b := range d.Skills.Active {
		r[id] += int(b.Points + b.Karma)
	}
	for id, v := range r {
		if v == 0 {
			delete(r, id)
		}
	}
	return r
}

// skillCosts soma pontos e carma das perícias nos orçamentos da calc.
func (s *System) skillCosts(d *sheet, c *calc, eval func(any, expr.Env) int) (points, groups, knowledge, karma int) {
	costs := s.mech.Progression.Costs
	steps := func(f any, from, n int) int {
		total := 0
		for r := from + 1; r <= from+n; r++ {
			total += eval(f, expr.Env{"rating": float64(r)})
		}
		return total
	}
	free := s.freeRatings(d)
	spec := func(b skillBuy) int {
		return paidWith(b.Specialization, "karma") * eval(costs.Specialization, nil)
	}
	for id, b := range d.Skills.Active {
		points += int(b.Points) + paidWith(b.Specialization, "points")*s.mech.Creation.Skills.SpecializationCost
		karma += steps(costs.ActiveSkill, free[id]+s.groupRating(d, id)+int(b.Points), int(b.Karma)) + spec(b)
	}
	for _, b := range d.Skills.Groups {
		groups += int(b.Points)
		karma += steps(costs.SkillGroup, int(b.Points), int(b.Karma))
	}
	for _, k := range d.Skills.Knowledge {
		knowledge += int(k.Points) + paidWith(k.Specialization, "points")*s.mech.Creation.Skills.SpecializationCost
		karma += steps(costs.KnowledgeSkill, int(k.Points), int(k.Karma)) + spec(k.skillBuy)
	}
	for _, l := range d.Skills.Languages {
		if l.Native {
			continue
		}
		knowledge += int(l.Points) + paidWith(l.Specialization, "points")*s.mech.Creation.Skills.SpecializationCost
		karma += steps(costs.LanguageSkill, int(l.Points), int(l.Karma)) + spec(l.skillBuy)
	}
	return
}

func (c *calc) pool(attr string, rating int) *int {
	v, ok := c.natural[attr]
	if !ok {
		return nil
	}
	p := v + rating
	return &p
}

func (s *System) deriveSkills(d *sheet, c *calc) *skillsOut {
	out := skillsOut{Active: map[string]ratedOut{}, Knowledge: map[string]ratedOut{}, Languages: map[string]ratedOut{}}
	for id, r := range s.activeRatings(d) {
		out.Active[id] = ratedOut{Rating: r, Pool: c.pool(s.skills[id].Attribute, r)}
	}
	for _, k := range d.Skills.Knowledge {
		if r := int(k.Points + k.Karma); r > 0 {
			out.Knowledge[k.Key] = ratedOut{Rating: r, Pool: c.pool(s.knowledge[k.Category], r)}
		}
	}
	for _, l := range d.Skills.Languages {
		if l.Native {
			out.Languages[l.Key] = ratedOut{Native: true}
		} else if r := int(l.Points + l.Karma); r > 0 {
			out.Languages[l.Key] = ratedOut{Rating: r, Pool: c.pool("int", r)}
		}
	}
	if len(out.Active)+len(out.Knowledge)+len(out.Languages) == 0 {
		return nil
	}
	return &out
}

// skillLayer aponta para a camada de compra mais de fora, como nos atributos;
// sem pontos nem carma próprios, o nível veio do grupo.
func (s *System) skillLayer(id string, b skillBuy) string {
	switch {
	case b.Karma > 0:
		return "skills.active." + id + ".karma"
	case b.Points > 0:
		return "skills.active." + id + ".points"
	}
	return "skills.groups." + s.skills[id].Group
}

func (s *System) strictSkills(d *sheet, c *calc) []rules.Violation {
	var vs []rules.Violation
	ratings := s.activeRatings(d)
	for _, id := range sortedKeys(ratings) {
		sk, b := s.skills[id], d.Skills.Active[id]
		limit := s.mech.Creation.Skills.MaxRating + c.effects.skillMax[id]
		if ratings[id] > limit {
			vs = append(vs, violation(s.skillLayer(id, b), "skills.aboveMax",
				map[string]any{"skill": id, "value": ratings[id], "max": limit}))
		}
		if sk.Restricted && !s.restrictedAllowed(d, id) {
			vs = append(vs, violation("skills.active."+id, "skills.restricted", map[string]any{"skill": id}))
		}
	}
	for _, id := range sortedKeys(d.Skills.Active) {
		b, group := d.Skills.Active[id], s.skills[id].Group
		if s.groupRating(d, id) == 0 {
			continue
		}
		if b.Points > 0 {
			vs = append(vs, violation("skills.active."+id+".points", "skills.groupConflict",
				map[string]any{"skill": id, "group": group}))
		}
		if b.Specialization != nil {
			vs = append(vs, violation("skills.active."+id+".specialization", "skills.specializationNotAllowed",
				map[string]any{"skill": id}))
		}
	}
	natives, expected := 0, 1+c.effects.natives
	for _, l := range d.Skills.Languages {
		if l.Native {
			natives++
		}
	}
	if natives != expected {
		vs = append(vs, violation("skills.languages", "skills.nativeLanguageCount",
			map[string]any{"expected": expected, "got": natives}))
	}
	return vs
}
