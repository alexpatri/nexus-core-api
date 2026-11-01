package shadowrun

import (
	"io/fs"
	"testing"

	"github.com/alexpatri/nexus-systems-api/data"
	"github.com/alexpatri/nexus-systems-api/fixtures"

	"rpg-nexus/api/core/internal/rules/rulestest"
)

func TestContract(t *testing.T) {
	sys, err := New(data.Systems())
	if err != nil {
		t.Fatal(err)
	}
	dir, err := fs.Sub(fixtures.FS(), "shadowrun/5e")
	if err != nil {
		t.Fatal(err)
	}
	rulestest.Run(t, sys, rulestest.Suite{Fixtures: dir, Codes: Codes})
}
