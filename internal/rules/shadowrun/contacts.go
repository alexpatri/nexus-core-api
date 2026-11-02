package shadowrun

import (
	"fmt"

	"rpg-nexus/api/core/internal/rules"
)

func contactKarma(d *sheet) int {
	total := 0
	for _, ct := range d.Contacts {
		connection, loyalty := ct.ratings()
		total += connection + loyalty
	}
	return total
}

func (s *System) strictContacts(d *sheet) []rules.Violation {
	r := s.mech.Creation.Contacts
	var vs []rules.Violation
	for _, ct := range d.Contacts {
		path := fmt.Sprintf("contacts[%s]", ct.Key)
		connection, loyalty := ct.ratings()
		if cost := connection + loyalty; cost < r.MinKarma || cost > r.MaxKarmaPerContact {
			vs = append(vs, violation(path, "contacts.costOutOfRange", map[string]any{"min": r.MinKarma, "max": r.MaxKarmaPerContact}))
		}
		for _, f := range []struct {
			field         string
			value, lo, hi int
		}{
			{"connection", connection, r.MinConnection, r.MaxConnection},
			{"loyalty", loyalty, r.MinLoyalty, r.MaxLoyalty},
		} {
			if f.value < f.lo || f.value > f.hi {
				vs = append(vs, violation(path+"."+f.field, "contacts.ratingOutOfRange",
					map[string]any{"field": f.field, "min": f.lo, "max": f.hi}))
			}
		}
	}
	return vs
}
