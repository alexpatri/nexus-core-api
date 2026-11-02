package shadowrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/expr"
)

type resourcesData struct {
	KarmaToNuyen      float64  `json:"karmaToNuyen"`
	StartingNuyenRoll *float64 `json:"startingNuyenRoll"`
	Lifestyles        []struct {
		Key     string   `json:"key"`
		ID      string   `json:"id"`
		Months  *float64 `json:"months"`
		Primary bool     `json:"primary"`
		Options []string `json:"options"`
	} `json:"lifestyles"`
	Identities []struct {
		Key      string   `json:"key"`
		Rating   *float64 `json:"rating"`
		Licenses []struct {
			Key    string   `json:"key"`
			Rating *float64 `json:"rating"`
		} `json:"licenses"`
	} `json:"identities"`
	Weapons []struct {
		Key      string   `json:"key"`
		ID       string   `json:"id"`
		Rating   *float64 `json:"rating"`
		Quantity *float64 `json:"quantity"`
	} `json:"weapons"`
	Armor []struct {
		Key           string   `json:"key"`
		ID            string   `json:"id"`
		Cost          *float64 `json:"cost"`
		Options       []string `json:"options"`
		Modifications []struct {
			Key    string   `json:"key"`
			ID     string   `json:"id"`
			Rating *float64 `json:"rating"`
		} `json:"modifications"`
	} `json:"armor"`
	Augmentations []augItem `json:"augmentations"`
	Devices       []struct {
		Key      string             `json:"key"`
		Kind     string             `json:"kind"`
		ID       string             `json:"id"`
		Options  []string           `json:"options"`
		Array    map[string]float64 `json:"array"`
		Programs []string           `json:"programs"`
	} `json:"devices"`
	Vehicles []struct {
		Key string `json:"key"`
		ID  string `json:"id"`
	} `json:"vehicles"`
	Other []struct {
		Cost     float64  `json:"cost"`
		Quantity *float64 `json:"quantity"`
	} `json:"other"`
}

type augItem struct {
	Key         string   `json:"key"`
	ID          string   `json:"id"`
	Grade       string   `json:"grade"`
	Rating      *float64 `json:"rating"`
	Variant     string   `json:"variant"`
	Parameter   string   `json:"parameter"`
	InstalledIn string   `json:"installedIn"`
}

type ratingRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// availability é { value } (fórmula com rating) ou { add } (soma à do item).
type availability struct {
	Value any `json:"value"`
	Add   any `json:"add"`
}

type gearModifier struct {
	Target string `json:"target"`
	Of     string `json:"of"`
	Add    any    `json:"add"`
}

type lifestyle struct {
	ID            string   `json:"id"`
	Cost          *float64 `json:"cost"`
	Primary       *bool    `json:"primary"`
	StartingNuyen *struct {
		Dice       int `json:"dice"`
		Multiplier int `json:"multiplier"`
	} `json:"startingNuyen"`
}

type gearItem struct {
	ID           string        `json:"id"`
	Category     string        `json:"category"`
	Rating       *ratingRange  `json:"rating"`
	Availability *availability `json:"availability"`
	Cost         any           `json:"cost"`
}

type armorItem struct {
	gearItem
	Armor    any           `json:"armor"`
	Capacity any           `json:"capacity"`
	Options  []armorOption `json:"options"`
}

type armorOption struct {
	ID           string        `json:"id"`
	Armor        any           `json:"armor"`
	Availability *availability `json:"availability"`
	Cost         any           `json:"cost"`
}

type armorMod struct {
	gearItem
	Capacity struct {
		Cost any `json:"cost"`
	} `json:"capacity"`
}

// augPart é o que um nível ou uma variante troca no item.
type augPart struct {
	ID           string          `json:"id"`
	Rating       int             `json:"rating"`
	Essence      any             `json:"essence"`
	Capacity     json.RawMessage `json:"capacity"`
	Availability *availability   `json:"availability"`
	Cost         any             `json:"cost"`
	Modifiers    []gearModifier  `json:"modifiers"`
}

type augmentation struct {
	augPart
	Category  string       `json:"category"`
	Type      string       `json:"type"`
	RatingR   *ratingRange `json:"rating"`
	Levels    []augPart    `json:"levels"`
	Variants  []augPart    `json:"variants"`
	Parameter *struct {
		Kind string `json:"kind"`
	} `json:"parameter"`
	Excludes []catalogRef `json:"excludes"`
	Unique   bool         `json:"unique"`
}

type catalogRef struct {
	Catalog string `json:"catalog"`
	ID      string `json:"id"`
}

type grade struct {
	ID           string  `json:"id"`
	Essence      float64 `json:"essence"`
	Availability float64 `json:"availability"`
	Cost         float64 `json:"cost"`
}

type device struct {
	gearItem
	DeviceRating   int   `json:"deviceRating"`
	AttributeArray []int `json:"attributeArray"`
}

type gearCatalogs struct {
	lifestyles    map[string]lifestyle
	lifeOptions   map[string]struct{ Add, Percent float64 }
	identities    map[string]gearItem
	weapons       map[string]gearItem
	armor         map[string]armorItem
	armorMods     map[string]armorMod
	augmentations map[string]augmentation
	grades        map[string]grade
	commlinks     map[string]device
	linkOptions   map[string]gearItem
	cyberdecks    map[string]device
	programs      map[string]gearItem
	vehicles      map[string]gearItem
}

func byID[T any](xs []T, id func(T) string) map[string]T {
	m := make(map[string]T, len(xs))
	for _, x := range xs {
		m[id(x)] = x
	}
	return m
}

func (s *System) loadGear(read func(string, any) error) error {
	var life struct {
		Lifestyles []lifestyle `json:"lifestyles"`
		Options    []struct {
			ID   string `json:"id"`
			Cost struct{ Add, Percent float64 }
		} `json:"options"`
		Identities []gearItem `json:"identities"`
	}
	var weapons struct {
		Weapons []gearItem `json:"weapons"`
	}
	var armor struct {
		Armor         []armorItem `json:"armor"`
		Modifications []armorMod  `json:"modifications"`
	}
	var augs struct {
		Grades        []grade        `json:"grades"`
		Augmentations []augmentation `json:"augmentations"`
	}
	var matrix struct {
		Commlinks       []device   `json:"commlinks"`
		CommlinkOptions []gearItem `json:"commlinkOptions"`
		Cyberdecks      []device   `json:"cyberdecks"`
		Programs        []gearItem `json:"programs"`
	}
	var vehicles struct {
		Vehicles []gearItem `json:"vehicles"`
	}
	for name, v := range map[string]any{
		"lifestyles.json": &life, "weapons.json": &weapons, "armor.json": &armor,
		"augmentations.json": &augs, "matrix.json": &matrix, "vehicles.json": &vehicles,
	} {
		if err := read(name, v); err != nil {
			return err
		}
	}
	g := &s.gear
	g.lifestyles = byID(life.Lifestyles, func(x lifestyle) string { return x.ID })
	g.lifeOptions = map[string]struct{ Add, Percent float64 }{}
	for _, o := range life.Options {
		g.lifeOptions[o.ID] = o.Cost
	}
	item := func(x gearItem) string { return x.ID }
	dev := func(x device) string { return x.ID }
	g.identities = byID(life.Identities, item)
	g.weapons = byID(weapons.Weapons, item)
	g.armor = byID(armor.Armor, func(x armorItem) string { return x.ID })
	g.armorMods = byID(armor.Modifications, func(x armorMod) string { return x.ID })
	g.augmentations = byID(augs.Augmentations, func(x augmentation) string { return x.ID })
	g.grades = byID(augs.Grades, func(x grade) string { return x.ID })
	g.commlinks = byID(matrix.Commlinks, dev)
	g.linkOptions = byID(matrix.CommlinkOptions, item)
	g.cyberdecks = byID(matrix.Cyberdecks, dev)
	g.programs = byID(matrix.Programs, item)
	g.vehicles = byID(vehicles.Vehicles, item)
	if len(g.lifestyles) == 0 || g.identities["fake-sin"].Rating == nil || g.identities["fake-license"].Rating == nil ||
		g.grades["standard"].ID == "" || len(g.augmentations) == 0 {
		return fmt.Errorf("shadowrun: catálogos de recursos incompletos")
	}
	return nil
}

var deviceKinds = []string{"commlink", "cyberdeck"}
var deckAttrs = []string{"attack", "sleaze", "dataProcessing", "firewall"}

func (s *System) lenientResources(path string, v any, vs *[]rules.Violation, keys map[string]bool) {
	g := &s.gear
	idField := func(p string, v any, catalog string, known bool) {
		if _, ok := v.(string); !ok {
			*vs = append(*vs, shape(p, "string"))
		} else if !known {
			*vs = append(*vs, unknownID(p, catalog, v.(string)))
		}
	}
	str := func(m map[string]any, k string) string {
		x, _ := m[k].(string)
		return x
	}
	items := func(p string, v any, fields func(path, key string, v any, m map[string]any) bool) {
		list(p, v, vs, keys, func(path string, m map[string]any) {
			object(path, m, vs, func(p, key string, v any) bool {
				if key == "key" {
					return true
				}
				return fields(p, key, v, m)
			})
			if _, ok := m["id"]; !ok && fields(path+".id", "id", nil, nil) {
				*vs = append(*vs, shape(path+".id", "string"))
			}
		})
	}
	// prefix vai antes do id quando a opção é do próprio item ("<item>.<opção>").
	ids := func(p string, v any, catalog, prefix string, known func(string) bool) {
		xs, ok := v.([]any)
		if !ok {
			*vs = append(*vs, shape(p, "array"))
			return
		}
		for _, x := range xs {
			id, ok := x.(string)
			switch {
			case !ok:
				*vs = append(*vs, shape(p, "string"))
			case !known(id):
				*vs = append(*vs, unknownID(p, catalog, prefix+id))
			}
		}
	}
	in := func(m map[string]gearItem) func(string) bool {
		return func(id string) bool { _, ok := m[id]; return ok }
	}

	object(path, v, vs, func(p, key string, v any) bool {
		switch key {
		case "karmaToNuyen", "startingNuyenRoll":
			expectInteger(p, v, vs)
		case "lifestyles":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				switch key {
				case "id":
					if m != nil {
						_, ok := g.lifestyles[str(m, "id")]
						idField(p, v, "lifestyles", ok)
					}
				case "months":
					expectInteger(p, v, vs)
				case "primary":
					if _, ok := v.(bool); !ok {
						*vs = append(*vs, shape(p, "boolean"))
					}
				case "options":
					ids(p, v, "lifestyleOptions", "", func(id string) bool { _, ok := g.lifeOptions[id]; return ok })
				default:
					return false
				}
				return true
			})
		case "identities":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				switch key {
				case "name":
					expectString(p, v, vs)
				case "rating":
					expectInteger(p, v, vs)
				case "licenses":
					items(p, v, func(p, key string, v any, m map[string]any) bool {
						switch key {
						case "name":
							expectString(p, v, vs)
						case "rating":
							expectInteger(p, v, vs)
						default:
							return false
						}
						return true
					})
				default:
					return false
				}
				return true
			})
		case "weapons":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				switch key {
				case "id":
					if m != nil {
						_, ok := g.weapons[str(m, "id")]
						idField(p, v, "weapons", ok)
					}
				case "rating", "quantity":
					expectInteger(p, v, vs)
				default:
					return false
				}
				return true
			})
		case "armor":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				if m == nil {
					return true
				}
				a, known := g.armor[str(m, "id")]
				switch key {
				case "id":
					idField(p, v, "armor", known)
				case "cost":
					expectInteger(p, v, vs)
				case "options":
					ids(p, v, "armorOptions", a.ID+".", func(id string) bool {
						return !known || slices.ContainsFunc(a.Options, func(o armorOption) bool { return o.ID == id })
					})
				case "modifications":
					items(p, v, func(p, key string, v any, m map[string]any) bool {
						switch key {
						case "id":
							if m != nil {
								_, ok := g.armorMods[str(m, "id")]
								idField(p, v, "armorModifications", ok)
							}
						case "rating":
							expectInteger(p, v, vs)
						default:
							return false
						}
						return true
					})
				default:
					return false
				}
				return true
			})
		case "augmentations":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				if m == nil {
					return true
				}
				a, known := g.augmentations[str(m, "id")]
				switch key {
				case "id":
					idField(p, v, "augmentations", known)
				case "grade":
					_, ok := g.grades[str(m, "grade")]
					idField(p, v, "grades", ok)
				case "rating":
					expectInteger(p, v, vs)
				case "variant":
					id := str(m, "variant")
					ok := !known || slices.ContainsFunc(a.Variants, func(x augPart) bool { return x.ID == id })
					if _, isStr := v.(string); isStr && !ok {
						*vs = append(*vs, unknownID(p, "augmentationVariants", a.ID+"."+id))
					} else {
						expectString(p, v, vs)
					}
				case "parameter", "installedIn":
					expectString(p, v, vs)
				default:
					return false
				}
				return true
			})
		case "devices":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				if m == nil {
					return true
				}
				kind := str(m, "kind")
				switch key {
				case "kind":
					if !slices.Contains(deviceKinds, kind) {
						*vs = append(*vs, shape(p, "deviceKind"))
					}
				case "id":
					switch kind {
					case "commlink":
						_, ok := g.commlinks[str(m, "id")]
						idField(p, v, "commlinks", ok)
					case "cyberdeck":
						_, ok := g.cyberdecks[str(m, "id")]
						idField(p, v, "cyberdecks", ok)
					default:
						expectString(p, v, vs)
					}
				case "options":
					if kind != "commlink" {
						return false
					}
					ids(p, v, "commlinkOptions", "", in(g.linkOptions))
				case "programs":
					if kind != "cyberdeck" {
						return false
					}
					ids(p, v, "programs", "", in(g.programs))
				case "array":
					if kind != "cyberdeck" {
						return false
					}
					object(p, v, vs, func(p, key string, v any) bool {
						if !slices.Contains(deckAttrs, key) {
							return false
						}
						expectInteger(p, v, vs)
						return true
					})
				default:
					return false
				}
				return true
			})
		case "vehicles":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				if key != "id" {
					return false
				}
				if m != nil {
					_, ok := g.vehicles[str(m, "id")]
					idField(p, v, "vehicles", ok)
				}
				return true
			})
		case "other":
			items(p, v, func(p, key string, v any, m map[string]any) bool {
				switch key {
				case "name":
					expectString(p, v, vs)
				case "cost", "quantity":
					expectInteger(p, v, vs)
				default:
					return false
				}
				return true
			})
		default:
			return false
		}
		return true
	})
}

func isExpr(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, op := m["op"]
	_, ref := m["ref"]
	return op || ref
}

// gearEval avalia número ou fórmula com rating; o que não é nenhum dos dois vale 0.
func gearEval(c *calc, f any, rating float64) float64 {
	if f == nil {
		return 0
	}
	if n, ok := f.(float64); ok {
		return n
	}
	if !isExpr(f) {
		return 0
	}
	v, err := expr.Eval(f, expr.Env{"rating": rating})
	if err != nil {
		c.err = errors.Join(c.err, err)
	}
	return v
}

// price lê o preço de catálogo: número, fórmula, { add }, { value } ou o mínimo da faixa.
func price(c *calc, cost any, rating float64) float64 {
	if m, ok := cost.(map[string]any); ok && !isExpr(cost) {
		for _, k := range []string{"add", "value", "min"} {
			if v, ok := m[k]; ok {
				return gearEval(c, v, rating)
			}
		}
		return 0
	}
	return gearEval(c, cost, rating)
}

func costRange(cost any) (lo, hi float64, ok bool) {
	m, isMap := cost.(map[string]any)
	if !isMap {
		return 0, 0, false
	}
	lo, okLo := m["min"].(float64)
	hi, okHi := m["max"].(float64)
	return lo, hi, okLo && okHi
}

func ratingOf(r *float64, rng *ratingRange) float64 {
	if r != nil {
		return *r
	}
	if rng != nil {
		return float64(rng.Min)
	}
	return 1
}

// part é o item com o nível ou a variante escolhida por cima.
func (a augmentation) part(it augItem) augPart {
	p := a.augPart
	over := func(x augPart) {
		if x.Essence != nil {
			p.Essence = x.Essence
		}
		if x.Capacity != nil {
			p.Capacity = x.Capacity
		}
		if x.Availability != nil {
			p.Availability = x.Availability
		}
		if x.Cost != nil {
			p.Cost = x.Cost
		}
		if x.Modifiers != nil {
			p.Modifiers = x.Modifiers
		}
	}
	r := int(ratingOf(it.Rating, a.RatingR))
	for _, l := range a.Levels {
		if l.Rating == r {
			over(l)
		}
	}
	for _, v := range a.Variants {
		if v.ID == it.Variant {
			over(v)
		}
	}
	return p
}

type capacity struct {
	Cost     any      `json:"cost"`
	Provides *float64 `json:"provides"`
	Accepts  []string `json:"accepts"`
}

func (p augPart) capacity() capacity {
	var c capacity
	_ = json.Unmarshal(p.Capacity, &c)
	return c
}

// gearOut é o que os recursos somam na ficha.
type gearOut struct {
	nuyen   float64
	essence int // milésimos gastos
	bonus   map[string]float64
	armor   float64
	dice    float64
	limit   map[string]float64
	pool    map[string]float64
	deckDP  *float64
}

func ceilDiv(num, den int64) int64 {
	q := num / den
	if num%den != 0 && num > 0 {
		q++
	}
	return q
}

func hundredths(x float64) int64 { return int64(math.Round(x * 100)) }

func (s *System) gearCalc(d *sheet, c *calc) gearOut {
	g, res := &s.gear, &d.Resources
	out := gearOut{bonus: map[string]float64{}, limit: map[string]float64{}, pool: map[string]float64{}}

	var mults []int64
	if m, ok := s.metatypes[d.Metatype]; ok {
		for _, t := range m.Traits {
			for _, mod := range t.Modifiers {
				if mod.Target == "lifestyle.cost" {
					mults = append(mults, hundredths(mod.Mul))
				}
			}
		}
	}
	for _, l := range res.Lifestyles {
		var pct, add float64
		for _, o := range l.Options {
			pct += g.lifeOptions[o].Percent
			add += g.lifeOptions[o].Add
		}
		base := 0.0
		if c := g.lifestyles[l.ID].Cost; c != nil {
			base = *c
		}
		num := int64(base)*(100+int64(pct)) + int64(add)*100
		den := int64(100)
		for _, m := range mults {
			num, den = num*m, den*100
		}
		months := int64(1)
		if l.Months != nil {
			months = int64(*l.Months)
		}
		out.nuyen += float64(ceilDiv(num*months, den))
	}
	sin, license := g.identities["fake-sin"], g.identities["fake-license"]
	for _, id := range res.Identities {
		out.nuyen += price(c, sin.Cost, ratingOf(id.Rating, sin.Rating))
		for _, l := range id.Licenses {
			out.nuyen += price(c, license.Cost, ratingOf(l.Rating, license.Rating))
		}
	}
	for _, w := range res.Weapons {
		if it, ok := g.weapons[w.ID]; ok {
			out.nuyen += price(c, it.Cost, ratingOf(w.Rating, it.Rating)) * ratingOf(w.Quantity, nil)
		}
	}
	worn, best := 0.0, map[string]float64{}
	for _, a := range res.Armor {
		it, ok := g.armor[a.ID]
		if !ok {
			continue
		}
		if _, _, ranged := costRange(it.Cost); ranged && a.Cost != nil {
			out.nuyen += *a.Cost
		} else {
			out.nuyen += price(c, it.Cost, 1)
		}
		v, _ := it.Armor.(float64)
		for _, o := range it.Options {
			if !slices.Contains(a.Options, o.ID) {
				continue
			}
			out.nuyen += price(c, o.Cost, 1)
			switch x := o.Armor.(type) {
			case float64:
				v = max(v, x)
			case map[string]any:
				v += gearEval(c, x["add"], 1)
			}
		}
		for _, m := range a.Modifications {
			if mod, ok := g.armorMods[m.ID]; ok {
				out.nuyen += price(c, mod.Cost, ratingOf(m.Rating, mod.Rating))
			}
		}
		if it.Category == "helmet" || it.Category == "shield" {
			add, _ := it.Armor.(map[string]any)
			best[it.Category] = max(best[it.Category], gearEval(c, add["add"], 1))
		} else {
			worn = max(worn, v)
		}
	}

	apply := func(ms []gearModifier, rating float64, parameter string) {
		for _, m := range ms {
			of := m.Of
			if of == "parameter" {
				of = parameter
			}
			switch m.Target {
			case "attr":
				out.bonus[of] += gearEval(c, m.Add, rating)
			case "armor":
				out.armor += gearEval(c, m.Add, rating)
			case "initiative.dice":
				out.dice += gearEval(c, m.Add, rating)
			case "limit":
				out.limit[of] += gearEval(c, m.Add, rating)
			case "skill.rating":
				out.pool[of] += gearEval(c, m.Add, rating)
			}
		}
	}

	lossMul := int64(100)
	for _, it := range d.Qualities {
		for _, m := range s.qualities[it.ID].Modifiers {
			if m.Target == "essence.cyberwareLoss" {
				mul, _ := m.Mul.(float64)
				lossMul = lossMul * hundredths(mul) / 100
			}
		}
	}
	for _, it := range res.Augmentations {
		a, ok := g.augmentations[it.ID]
		if !ok {
			continue
		}
		p, r := a.part(it), ratingOf(it.Rating, a.RatingR)
		gr, ok := g.grades[it.Grade]
		if !ok {
			gr = g.grades["standard"]
		}
		cost := int64(math.Round(price(c, p.Cost, r)))
		out.nuyen += float64(ceilDiv(cost*hundredths(gr.Cost), 100))
		if it.InstalledIn == "" && p.Essence != nil {
			milli := int64(math.Round(gearEval(c, p.Essence, r) * 1000))
			mul := int64(100)
			if a.Type == "cyberware" {
				mul = lossMul
			}
			out.essence += int(ceilDiv(milli*hundredths(gr.Essence)*mul, 10000))
		}
		apply(p.Modifiers, r, it.Parameter)
	}
	for _, dv := range res.Devices {
		switch dv.Kind {
		case "commlink":
			out.nuyen += price(c, g.commlinks[dv.ID].Cost, 1)
			for _, o := range dv.Options {
				out.nuyen += price(c, g.linkOptions[o].Cost, 1)
			}
		case "cyberdeck":
			out.nuyen += price(c, g.cyberdecks[dv.ID].Cost, 1)
			for _, p := range dv.Programs {
				out.nuyen += price(c, g.programs[p].Cost, 1)
			}
			if out.deckDP == nil && dv.Array != nil {
				dp := dv.Array["dataProcessing"]
				out.deckDP = &dp
			}
		}
	}
	for _, v := range res.Vehicles {
		out.nuyen += price(c, g.vehicles[v.ID].Cost, 1)
	}
	for _, o := range res.Other {
		out.nuyen += o.Cost * ratingOf(o.Quantity, nil)
	}
	for _, p := range d.Magic.Powers {
		if pw, ok := s.powers[p.ID]; ok {
			apply(pw.Modifiers, float64(p.rating()), p.Parameter)
		}
	}
	if m, ok := s.metatypes[d.Metatype]; ok {
		for _, t := range m.Traits {
			for _, mod := range t.Modifiers {
				if mod.Target == "armor" {
					out.armor += gearEval(c, mod.Add, 1)
				}
			}
		}
	}
	out.armor += worn + best["helmet"] + best["shield"]
	limit := float64(s.mech.Augmentation.MaxAttributeBonus)
	for a, v := range out.bonus {
		out.bonus[a] = min(v, limit)
	}
	return out
}

func gearPath(list, key, field string) string {
	p := "resources." + list + "[" + key + "]"
	if field != "" {
		p += "." + field
	}
	return p
}

func (s *System) strictResources(d *sheet, c *calc) []rules.Violation {
	g, res, mech := &s.gear, &d.Resources, &s.mech.Creation.Resources
	var vs []rules.Violation
	add := func(path, code string, params map[string]any) {
		vs = append(vs, violation(path, code, params))
	}
	maxAv := float64(mech.MaxAvailability)
	avail := func(path, item string, value float64) {
		if value > maxAv {
			add(path, "gear.availabilityMax", map[string]any{"item": item, "availability": value, "max": maxAv})
		}
	}
	rating := func(path, item string, r *float64, rng *ratingRange) {
		if r != nil && rng != nil && (*r < float64(rng.Min) || *r > float64(rng.Max)) {
			add(path, "gear.ratingOutOfRange", map[string]any{"item": item, "min": rng.Min, "max": rng.Max})
		}
	}
	value := func(a *availability, r float64) float64 {
		if a == nil {
			return 0
		}
		return gearEval(c, a.Value, r)
	}
	addOf := func(a *availability) float64 {
		if a == nil {
			return 0
		}
		return gearEval(c, a.Add, 1)
	}

	primary := ""
	for _, l := range res.Lifestyles {
		st, ok := g.lifestyles[l.ID]
		if !ok {
			continue
		}
		if st.Primary != nil && !*st.Primary {
			add(gearPath("lifestyles", l.Key, ""), "resources.lifestyleNotAllowed", map[string]any{"lifestyle": l.ID})
		}
		if !l.Primary {
			continue
		}
		if primary != "" {
			add(gearPath("lifestyles", l.Key, "primary"), "resources.primaryLifestyleDuplicate", nil)
			continue
		}
		primary = l.ID
	}
	if primary == "" {
		vs = append(vs, required("resources.lifestyles"))
	}
	if res.StartingNuyenRoll == nil {
		vs = append(vs, required("resources.startingNuyenRoll"))
	} else if sn := g.lifestyles[primary].StartingNuyen; sn != nil {
		if r := *res.StartingNuyenRoll; r < float64(sn.Dice) || r > float64(sn.Dice*6) {
			add("resources.startingNuyenRoll", "resources.startingRollOutOfRange", map[string]any{"min": sn.Dice, "max": sn.Dice * 6})
		}
	}

	sin, license := g.identities["fake-sin"], g.identities["fake-license"]
	for _, id := range res.Identities {
		rating(gearPath("identities", id.Key, "rating"), sin.ID, id.Rating, sin.Rating)
		avail(gearPath("identities", id.Key, ""), sin.ID, value(sin.Availability, ratingOf(id.Rating, sin.Rating)))
		for _, l := range id.Licenses {
			p := gearPath("identities", id.Key, "licenses["+l.Key+"]")
			rating(p+".rating", license.ID, l.Rating, license.Rating)
			avail(p, license.ID, value(license.Availability, ratingOf(l.Rating, license.Rating)))
		}
	}
	for _, w := range res.Weapons {
		it, ok := g.weapons[w.ID]
		if !ok {
			continue
		}
		rating(gearPath("weapons", w.Key, "rating"), w.ID, w.Rating, it.Rating)
		avail(gearPath("weapons", w.Key, ""), w.ID, value(it.Availability, ratingOf(w.Rating, it.Rating)))
	}
	for _, a := range res.Armor {
		it, ok := g.armor[a.ID]
		if !ok {
			continue
		}
		p := gearPath("armor", a.Key, "")
		if lo, hi, ranged := costRange(it.Cost); ranged && a.Cost != nil && (*a.Cost < lo || *a.Cost > hi) {
			add(p+".cost", "gear.costOutOfRange", map[string]any{"item": a.ID, "min": lo, "max": hi})
		}
		av := value(it.Availability, 1)
		for _, o := range it.Options {
			if !slices.Contains(a.Options, o.ID) {
				continue
			}
			av += addOf(o.Availability)
			if o.Availability != nil && o.Availability.Value != nil {
				avail(p+".options", o.ID, value(o.Availability, 1))
			}
		}
		avail(p, a.ID, av)
		used := 0.0
		for _, m := range a.Modifications {
			mod, ok := g.armorMods[m.ID]
			if !ok {
				continue
			}
			mp := p + ".modifications[" + m.Key + "]"
			r := ratingOf(m.Rating, mod.Rating)
			rating(mp+".rating", m.ID, m.Rating, mod.Rating)
			avail(mp, m.ID, value(mod.Availability, r))
			used += gearEval(c, mod.Capacity.Cost, r)
		}
		if capacity, _ := it.Capacity.(float64); used > capacity {
			add(p, "gear.capacityExceeded", map[string]any{"item": a.ID, "capacity": capacity, "used": used})
		}
	}

	hosts := map[string]augItem{}
	for _, it := range res.Augmentations {
		hosts[it.Key] = it
	}
	used := map[string]float64{}
	var order []string
	for i, it := range res.Augmentations {
		a, ok := g.augmentations[it.ID]
		if !ok {
			continue
		}
		p, r := a.part(it), ratingOf(it.Rating, a.RatingR)
		path := gearPath("augmentations", it.Key, "")
		rating(path+".rating", it.ID, it.Rating, a.RatingR)
		gr, known := g.grades[it.Grade]
		if !known {
			gr = g.grades["standard"]
		}
		if it.Grade != "" && !slices.Contains(mech.Grades, it.Grade) {
			add(path+".grade", "gear.gradeNotAllowed", map[string]any{"item": it.ID, "grade": it.Grade})
		}
		avail(path, it.ID, value(p.Availability, r)+gr.Availability)
		if len(a.Variants) > 0 && it.Variant == "" {
			add(path+".variant", "gear.variantRequired", map[string]any{"item": it.ID})
		}
		if a.Parameter != nil {
			switch {
			case it.Parameter == "":
				add(path+".parameter", "gear.parameterRequired", map[string]any{"item": it.ID})
			case a.Parameter.Kind == "skill" && s.skills[it.Parameter].ID == "":
				add(path+".parameter", "gear.parameterNotAllowed", map[string]any{"item": it.ID, "parameter": it.Parameter})
			}
		}
		cp := p.capacity()
		if it.InstalledIn != "" {
			host, found := hosts[it.InstalledIn]
			ha := g.augmentations[host.ID]
			hp := ha.part(host).capacity()
			if !found || hp.Provides == nil || cp.Cost == nil || !slices.Contains(ha.capacity().Accepts, a.Category) {
				add(path+".installedIn", "gear.notInstallable", map[string]any{"item": it.ID})
			} else {
				if _, seen := used[host.Key]; !seen {
					order = append(order, host.Key)
				}
				used[host.Key] += gearEval(c, cp.Cost, r)
			}
		} else if p.Essence == nil && cp.Cost != nil {
			add(path, "gear.hostRequired", map[string]any{"item": it.ID})
		}
		for _, prev := range res.Augmentations[:i] {
			pa, ok := g.augmentations[prev.ID]
			if !ok {
				continue
			}
			if a.excludes(prev.ID) || pa.excludes(it.ID) {
				add(path, "gear.excluded", map[string]any{"item": it.ID, "with": prev.ID})
			}
			if a.Unique && prev.ID == it.ID && prev.Parameter == it.Parameter {
				add(path, "gear.duplicate", map[string]any{"item": it.ID})
			}
		}
	}
	for _, key := range order {
		host := hosts[key]
		provides := *g.augmentations[host.ID].part(host).capacity().Provides
		if used[key] > provides {
			add(gearPath("augmentations", key, ""), "gear.capacityExceeded",
				map[string]any{"item": host.ID, "capacity": provides, "used": used[key]})
		}
	}

	maxDR := mech.MaxDeviceRating
	for _, dv := range res.Devices {
		p := gearPath("devices", dv.Key, "")
		var it device
		switch dv.Kind {
		case "commlink":
			it = g.commlinks[dv.ID]
		case "cyberdeck":
			it = g.cyberdecks[dv.ID]
		}
		if it.ID == "" {
			continue
		}
		if it.DeviceRating > maxDR {
			add(p, "gear.deviceRatingMax", map[string]any{"item": dv.ID, "max": maxDR})
		}
		av := value(it.Availability, 1)
		for _, o := range dv.Options {
			av += addOf(g.linkOptions[o].Availability)
		}
		avail(p, dv.ID, av)
		for _, pr := range dv.Programs {
			avail(p+".programs", pr, value(g.programs[pr].Availability, 1))
		}
		if dv.Kind == "cyberdeck" && !permutation(dv.Array, it.AttributeArray) {
			add(p+".array", "gear.deckArrayInvalid", map[string]any{"item": dv.ID})
		}
	}
	for _, v := range res.Vehicles {
		if it, ok := g.vehicles[v.ID]; ok {
			avail(gearPath("vehicles", v.Key, ""), v.ID, value(it.Availability, 1))
		}
	}
	if c.essence != nil && *c.essence <= 0 {
		add("resources.augmentations", "essence.depleted", map[string]any{"essence": *c.essence})
	}
	return vs
}

func (a augmentation) excludes(id string) bool {
	return slices.ContainsFunc(a.Excludes, func(r catalogRef) bool { return r.Catalog == "augmentations" && r.ID == id })
}

func permutation(array map[string]float64, want []int) bool {
	if array == nil {
		return false
	}
	got := make([]int, 0, len(deckAttrs))
	for _, k := range deckAttrs {
		got = append(got, int(array[k]))
	}
	w := slices.Clone(want)
	slices.Sort(got)
	slices.Sort(w)
	return slices.Equal(got, w)
}

// present diz se o item de outro catálogo está na ficha.
func (s *System) present(d *sheet, x catalogRef) bool {
	switch x.Catalog {
	case "augmentations":
		return slices.ContainsFunc(d.Resources.Augmentations, func(a augItem) bool { return a.ID == x.ID })
	case "adept-powers":
		return slices.ContainsFunc(d.Magic.Powers, func(p powerItem) bool { return p.ID == x.ID })
	}
	return false
}

// startingLifestyle é o primeiro estilo de vida primário da ficha.
func (s *System) startingLifestyle(d *sheet) *lifestyle {
	for _, l := range d.Resources.Lifestyles {
		if st, ok := s.gear.lifestyles[l.ID]; ok && l.Primary {
			return &st
		}
	}
	return nil
}
