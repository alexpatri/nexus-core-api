// Package shadowrun é a estratégia de Shadowrun 5e: valida a ficha, calcula os
// derivados e resume para a listagem, com as regras lidas dos catálogos. O
// contrato está em shadowrun-contrato-da-ficha.md.
package shadowrun

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/expr"
)

var key = rules.Key{System: "shadowrun", Version: "5e"}

// Codes são os códigos de violação que a estratégia emite hoje.
var Codes = []string{
	"shape.invalid", "catalog.unknownId", "required", "priorities.duplicate", "metatype.notAtPriority",
	"attributes.aboveMax", "attributes.tooManyAtMax", "attributes.notAllowed", "budget.exceeded",
	"key.duplicate", "qualities.levelOutOfRange", "qualities.optionRequired", "qualities.choiceRequired",
	"qualities.choiceNotAllowed", "qualities.duplicate", "qualities.unique", "qualities.requirementNotMet",
	"skills.aboveMax", "skills.restricted", "skills.groupConflict", "skills.specializationNotAllowed",
	"skills.nativeLanguageCount", "magic.notAtPriority", "magic.notAllowed", "magic.freeSkillsCount",
	"magic.freeSkillNotAllowed", "magic.formulasMax", "magic.complexFormsMax", "magic.powerLevelMax",
	"magic.parameterRequired", "magic.parameterNotAllowed", "magic.spiritsMax", "qualities.excluded",
	"gear.availabilityMax", "gear.ratingOutOfRange", "gear.costOutOfRange", "gear.variantRequired",
	"gear.parameterRequired", "gear.parameterNotAllowed", "gear.gradeNotAllowed", "gear.notInstallable",
	"gear.hostRequired", "gear.excluded", "gear.duplicate", "gear.capacityExceeded", "gear.deviceRatingMax",
	"gear.deckArrayInvalid", "resources.primaryLifestyleDuplicate", "resources.lifestyleNotAllowed",
	"resources.startingRollOutOfRange", "essence.depleted", "contacts.costOutOfRange", "contacts.ratingOutOfRange",
}

// Atributos que a ficha compra, na ordem de attributes.json.
var (
	baseAttrs    = []string{"bod", "agi", "rea", "str", "wil", "log", "int", "cha"}
	specialAttrs = []string{"edg", "mag", "res"}
	columns      = []string{"metatype", "attributes", "magic", "skills", "resources"}
)

type System struct {
	steps      []string
	metatypes  map[string]metatype
	priorities map[string]priorityRow
	mech       mechanics
	qualities  map[string]quality
	skills     map[string]skill
	groups     map[string][]string
	// Atributo de cada categoria de conhecimento.
	knowledge map[string]string
	// Ids que o choice de uma qualidade pode apontar, pelo ponteiro do catálogo.
	choices map[string][]string
	// Atributo de cada despertar (mag ou res).
	awakenings map[string]string
	powers     map[string]adeptPower
	// Ids de cada catálogo de magia que o Lenient confere.
	magicIDs map[string][]string
	gear     gearCatalogs
}

type metatype struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Attributes map[string]struct {
		Min   int `json:"min"`
		Max   int `json:"max"`
		Start int `json:"start"`
	} `json:"attributes"`
	Traits []struct {
		Modifiers []modifier `json:"modifiers"`
	} `json:"traits"`
	SprintPerHit int `json:"sprintPerHit"`
}

type modifier struct {
	Target string  `json:"target"`
	Add    any     `json:"add"`
	Mul    float64 `json:"mul"`
}

type priorityRow struct {
	Priority string `json:"priority"`
	Metatype []struct {
		Metatype      string `json:"metatype"`
		SpecialPoints int    `json:"specialPoints"`
	} `json:"metatype"`
	Attributes int          `json:"attributes"`
	Magic      []magicEntry `json:"magic"`
	Skills     struct {
		Points      int `json:"points"`
		GroupPoints int `json:"groupPoints"`
	} `json:"skills"`
	Resources int `json:"resources"`
}

// O que uma letra da coluna Magia ou Ressonância dá a um despertar.
type magicEntry struct {
	Awakening string `json:"awakening"`
	Attribute struct {
		ID    string `json:"id"`
		Value int    `json:"value"`
	} `json:"attribute"`
	Skills       *freeSkillRule `json:"skills"`
	SkillGroups  *freeSkillRule `json:"skillGroups"`
	Formulas     int            `json:"formulas"`
	ComplexForms int            `json:"complexForms"`
}

type freeSkillRule struct {
	Count  int `json:"count"`
	Rating int `json:"rating"`
	From   struct {
		Attribute string   `json:"attribute"`
		Groups    []string `json:"groups"`
		Kind      string   `json:"kind"`
	} `json:"from"`
}

type initiative struct {
	Base any `json:"base"`
	Dice int `json:"dice"`
}

// Só os trechos de mechanics.json que a estratégia usa; as fórmulas ficam como
// saem do json.Unmarshal, que é o que expr.Eval recebe.
type mechanics struct {
	Derived struct {
		Initiative struct {
			Physical     initiative `json:"physical"`
			Astral       initiative `json:"astral"`
			MatrixAR     initiative `json:"matrixAR"`
			MatrixVRCold initiative `json:"matrixVRCold"`
			MatrixVRHot  initiative `json:"matrixVRHot"`
			MaxDice      int        `json:"maxDice"`
		} `json:"initiative"`
		Limits struct {
			Physical struct{ Value any } `json:"physical"`
			Mental   struct{ Value any } `json:"mental"`
			Social   struct{ Value any } `json:"social"`
		} `json:"limits"`
		Monitors struct {
			Physical struct{ Boxes any } `json:"physical"`
			Stun     struct{ Boxes any } `json:"stun"`
			Overflow struct{ Boxes any } `json:"overflow"`
		} `json:"monitors"`
		AttributeTests struct {
			Composure       struct{ Pool any } `json:"composure"`
			JudgeIntentions struct{ Pool any } `json:"judgeIntentions"`
			Memory          struct{ Pool any } `json:"memory"`
			LiftCarry       struct{ Pool any } `json:"liftCarry"`
		} `json:"attributeTests"`
		Lifting struct {
			Lift     any `json:"lift"`
			Overhead any `json:"overhead"`
			Carry    any `json:"carry"`
		} `json:"lifting"`
		Movement struct {
			Walk any `json:"walk"`
			Run  any `json:"run"`
		} `json:"movement"`
		LivingPersona struct {
			DataProcessing any `json:"dataProcessing"`
		} `json:"livingPersona"`
		EssenceLoss struct {
			Attributes []string `json:"attributes"`
			Loss       any      `json:"loss"`
		} `json:"essenceLoss"`
		Reputation struct {
			StreetCred      struct{ Value any } `json:"streetCred"`
			Notoriety       struct{ Base int }  `json:"notoriety"`
			PublicAwareness struct{ Base int }  `json:"publicAwareness"`
		} `json:"reputation"`
	} `json:"derived"`
	Creation struct {
		Karma     int `json:"karma"`
		Qualities struct {
			PositiveMax int `json:"positiveMax"`
			NegativeMax int `json:"negativeMax"`
		} `json:"qualities"`
		Skills struct {
			KnowledgePoints    any `json:"knowledgePoints"`
			MaxRating          int `json:"maxRating"`
			SpecializationCost int `json:"specializationCost"`
		} `json:"skills"`
		Contacts struct {
			Karma              any `json:"karma"`
			MinConnection      int `json:"minConnection"`
			MaxConnection      int `json:"maxConnection"`
			MinLoyalty         int `json:"minLoyalty"`
			MaxLoyalty         int `json:"maxLoyalty"`
			MinKarma           int `json:"minKarma"`
			MaxKarmaPerContact int `json:"maxKarmaPerContact"`
		} `json:"contacts"`
		SpecialAttributeMax map[string]int `json:"specialAttributeMax"`
		Resources           struct {
			KarmaToNuyen struct {
				Rate     int `json:"rate"`
				MaxKarma int `json:"maxKarma"`
			} `json:"karmaToNuyen"`
			NuyenCarryOver  int      `json:"nuyenCarryOver"`
			MaxAvailability int      `json:"maxAvailability"`
			MaxDeviceRating int      `json:"maxDeviceRating"`
			Grades          []string `json:"grades"`
		} `json:"resources"`
		Purchases struct {
			Spell struct {
				Karma int `json:"karma"`
				Max   any `json:"max"`
			} `json:"spell"`
			ComplexForm struct {
				Karma int `json:"karma"`
				Max   any `json:"max"`
			} `json:"complexForm"`
			PowerPoint struct {
				Karma int `json:"karma"`
			} `json:"powerPoint"`
			BoundSpiritService struct {
				Karma      int `json:"karma"`
				MaxSpirits any `json:"maxSpirits"`
			} `json:"boundSpiritService"`
			RegisteredSpriteTask struct {
				Karma int `json:"karma"`
			} `json:"registeredSpriteTask"`
		} `json:"purchases"`
	} `json:"creation"`
	Augmentation struct {
		MaxAttributeBonus int `json:"maxAttributeBonus"`
	} `json:"augmentation"`
	Progression struct {
		Costs struct {
			Attribute      any `json:"attribute"`
			ActiveSkill    any `json:"activeSkill"`
			SkillGroup     any `json:"skillGroup"`
			KnowledgeSkill any `json:"knowledgeSkill"`
			LanguageSkill  any `json:"languageSkill"`
			Specialization any `json:"specialization"`
		} `json:"costs"`
	} `json:"progression"`
}

// New lê os catálogos de <sistema>/<versão>/ e falha se faltar algo que a
// estratégia usa, para o erro aparecer no boot e não na primeira ficha.
func New(catalogs fs.FS) (*System, error) {
	dir := key.System + "/" + key.Version + "/"
	read := func(name string, v any) error {
		raw, err := fs.ReadFile(catalogs, dir+name)
		if err != nil {
			return fmt.Errorf("shadowrun: %w", err)
		}
		if err := json.Unmarshal(raw, v); err != nil {
			return fmt.Errorf("shadowrun: %s: %w", name, err)
		}
		return nil
	}

	var manifest struct {
		Creation struct {
			Steps []struct {
				ID string `json:"id"`
			} `json:"steps"`
		} `json:"creation"`
	}
	var metas struct {
		Metatypes []metatype `json:"metatypes"`
	}
	var prios struct {
		Rows []priorityRow `json:"rows"`
	}
	var quals struct {
		Qualities []quality `json:"qualities"`
	}
	var skillCat struct {
		Skills []skill `json:"skills"`
		Groups []struct {
			ID     string   `json:"id"`
			Skills []string `json:"skills"`
		} `json:"groups"`
		Knowledge struct {
			Categories []struct {
				ID        string `json:"id"`
				Attribute string `json:"attribute"`
			} `json:"categories"`
		} `json:"knowledge"`
	}
	var awakenings struct {
		Awakenings []struct {
			ID        string `json:"id"`
			Attribute string `json:"attribute"`
		} `json:"awakenings"`
	}
	var adept struct {
		Powers []adeptPower `json:"powers"`
	}
	var forms struct {
		ComplexForms []struct{ ID string } `json:"complexForms"`
		SpriteTypes  []struct{ ID string } `json:"spriteTypes"`
	}
	var spells struct {
		Spells  []struct{ ID string } `json:"spells"`
		Rituals []struct{ ID string } `json:"rituals"`
	}
	var mentors struct {
		Mentors []struct{ ID string } `json:"mentors"`
	}
	var traditions struct {
		SpiritTypes []struct{ ID string } `json:"spiritTypes"`
		Traditions  []struct{ ID string } `json:"traditions"`
	}
	s := &System{
		metatypes: map[string]metatype{}, priorities: map[string]priorityRow{}, qualities: map[string]quality{},
		skills: map[string]skill{}, groups: map[string][]string{}, knowledge: map[string]string{},
		awakenings: map[string]string{}, powers: map[string]adeptPower{}, magicIDs: map[string][]string{},
	}
	for name, v := range map[string]any{
		"system.json": &manifest, "metatypes.json": &metas, "priorities.json": &prios, "mechanics.json": &s.mech,
		"qualities.json": &quals, "skills.json": &skillCat, "mentors.json": &mentors, "traditions.json": &traditions,
		"awakening.json": &awakenings, "adept-powers.json": &adept, "complex-forms.json": &forms, "spells.json": &spells,
	} {
		if err := read(name, v); err != nil {
			return nil, err
		}
	}
	if err := s.loadGear(read); err != nil {
		return nil, err
	}
	for _, st := range manifest.Creation.Steps {
		s.steps = append(s.steps, st.ID)
	}
	for _, m := range metas.Metatypes {
		s.metatypes[m.ID] = m
	}
	for _, r := range prios.Rows {
		s.priorities[r.Priority] = r
	}
	for _, q := range quals.Qualities {
		s.qualities[q.ID] = q
	}
	for _, sk := range skillCat.Skills {
		s.skills[sk.ID] = sk
	}
	for _, g := range skillCat.Groups {
		s.groups[g.ID] = g.Skills
	}
	for _, k := range skillCat.Knowledge.Categories {
		s.knowledge[k.ID] = k.Attribute
	}
	s.choices = map[string][]string{
		"skill":     slices.Sorted(maps.Keys(s.skills)),
		"attribute": slices.Concat(baseAttrs, []string{"ess"}, specialAttrs),
		"limit":     {"physical", "mental", "social"},
	}
	for _, m := range mentors.Mentors {
		s.choices["mentors.json#mentors"] = append(s.choices["mentors.json#mentors"], m.ID)
	}
	for _, t := range traditions.SpiritTypes {
		s.choices["traditions.json#spiritTypes"] = append(s.choices["traditions.json#spiritTypes"], t.ID)
	}
	for _, a := range awakenings.Awakenings {
		s.awakenings[a.ID] = a.Attribute
	}
	for _, p := range adept.Powers {
		s.powers[p.ID] = p
	}
	ids := func(xs []struct{ ID string }) []string {
		out := make([]string, len(xs))
		for i, x := range xs {
			out[i] = x.ID
		}
		return out
	}
	s.magicIDs["traditions"] = ids(traditions.Traditions)
	s.magicIDs["spiritTypes"] = ids(traditions.SpiritTypes)
	s.magicIDs["spriteTypes"] = ids(forms.SpriteTypes)
	s.magicIDs["complexForms"] = ids(forms.ComplexForms)
	s.magicIDs["spells"] = ids(spells.Spells)
	s.magicIDs["rituals"] = ids(spells.Rituals)
	// Preparativo é a fórmula alquímica de um feitiço.
	s.magicIDs["preparations"] = s.magicIDs["spells"]
	if err := s.check(); err != nil {
		return nil, fmt.Errorf("shadowrun: %w", err)
	}
	return s, nil
}

// check avalia toda fórmula usada com um ambiente completo: ref faltando ou
// fórmula malformada no catálogo derruba o boot.
func (s *System) check() error {
	if len(s.steps) == 0 || len(s.metatypes) == 0 || len(s.priorities) != 5 || len(s.qualities) == 0 ||
		len(s.skills) == 0 || len(s.knowledge) == 0 || len(s.awakenings) == 0 || len(s.powers) == 0 ||
		len(s.magicIDs["spriteTypes"]) == 0 || s.mech.Creation.SpecialAttributeMax["mag"] == 0 ||
		s.mech.Creation.Resources.MaxAvailability == 0 || s.mech.Creation.Contacts.MaxLoyalty == 0 || s.mech.Augmentation.MaxAttributeBonus == 0 ||
		len(s.mech.Derived.EssenceLoss.Attributes) == 0 {
		return fmt.Errorf("catálogos incompletos")
	}
	env := expr.Env{"rating": 1, "karma.total": 0, "attr.ess": 6, "essence.lost": 0.5}
	for _, a := range append(baseAttrs, specialAttrs...) {
		env["attr."+a] = 1
	}
	m := &s.mech
	formulas := map[string]any{
		"initiative.physical":        m.Derived.Initiative.Physical.Base,
		"initiative.matrixAR":        m.Derived.Initiative.MatrixAR.Base,
		"monitors.physical":          m.Derived.Monitors.Physical.Boxes,
		"monitors.stun":              m.Derived.Monitors.Stun.Boxes,
		"monitors.overflow":          m.Derived.Monitors.Overflow.Boxes,
		"lifting.lift":               m.Derived.Lifting.Lift,
		"lifting.overhead":           m.Derived.Lifting.Overhead,
		"lifting.carry":              m.Derived.Lifting.Carry,
		"movement.walk":              m.Derived.Movement.Walk,
		"movement.run":               m.Derived.Movement.Run,
		"reputation.streetCred":      m.Derived.Reputation.StreetCred.Value,
		"creation.knowledgePoints":   m.Creation.Skills.KnowledgePoints,
		"creation.contacts":          m.Creation.Contacts.Karma,
		"progression.attribute":      m.Progression.Costs.Attribute,
		"progression.activeSkill":    m.Progression.Costs.ActiveSkill,
		"progression.skillGroup":     m.Progression.Costs.SkillGroup,
		"progression.knowledgeSkill": m.Progression.Costs.KnowledgeSkill,
		"progression.languageSkill":  m.Progression.Costs.LanguageSkill,
		"progression.specialization": m.Progression.Costs.Specialization,
		"initiative.astral":          m.Derived.Initiative.Astral.Base,
		"purchases.spell.max":        m.Creation.Purchases.Spell.Max,
		"purchases.complexForm.max":  m.Creation.Purchases.ComplexForm.Max,
		"essenceLoss.loss":           m.Derived.EssenceLoss.Loss,
	}
	for id, f := range m.limits() {
		formulas["limits."+id] = f
	}
	for id, f := range m.attributeTests() {
		formulas["attributeTests."+id] = f
	}
	for name, f := range formulas {
		if _, err := expr.Eval(f, env); err != nil {
			return fmt.Errorf("mechanics.json %s: %w", name, err)
		}
	}
	return nil
}

func (m *mechanics) limits() map[string]any {
	l := m.Derived.Limits
	return map[string]any{"physical": l.Physical.Value, "mental": l.Mental.Value, "social": l.Social.Value}
}

func (m *mechanics) attributeTests() map[string]any {
	t := m.Derived.AttributeTests
	return map[string]any{
		"composure": t.Composure.Pool, "judgeIntentions": t.JudgeIntentions.Pool,
		"memory": t.Memory.Pool, "liftCarry": t.LiftCarry.Pool,
	}
}

func (s *System) Key() rules.Key { return key }

func (s *System) stepOrder(step string) int {
	for i, id := range s.steps {
		if id == step {
			return i + 1
		}
	}
	return len(s.steps) + 1
}
