package shadowrun

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/alexpatri/nexus-systems-api/data"
)

func catalogs(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for _, name := range []string{
		"system.json", "metatypes.json", "priorities.json", "mechanics.json",
		"qualities.json", "skills.json", "mentors.json", "traditions.json",
	} {
		raw, err := fs.ReadFile(data.Systems(), "shadowrun/5e/"+name)
		if err != nil {
			t.Fatal(err)
		}
		m["shadowrun/5e/"+name] = &fstest.MapFile{Data: raw}
	}
	return m
}

func TestNewFailsOnBrokenCatalogs(t *testing.T) {
	missing := catalogs(t)
	delete(missing, "shadowrun/5e/priorities.json")

	badFormula := catalogs(t)
	mech := string(badFormula["shadowrun/5e/mechanics.json"].Data)
	badFormula["shadowrun/5e/mechanics.json"] = &fstest.MapFile{
		Data: []byte(strings.Replace(mech, `"attr.rea"`, `"attr.reaction"`, 1)),
	}

	for name, fsys := range map[string]fstest.MapFS{"catálogo ausente": missing, "ref desconhecida": badFormula} {
		if _, err := New(fsys); err == nil {
			t.Errorf("%s: New não falhou", name)
		}
	}
	if _, err := New(catalogs(t)); err != nil {
		t.Errorf("catálogos íntegros: %v", err)
	}
}
