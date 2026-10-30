package expr

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/alexpatri/nexus-systems-api/data"
	"github.com/alexpatri/nexus-systems-api/fixtures"
)

type caseFile struct {
	Name  string         `json:"name"`
	Envs  map[string]Env `json:"envs"`
	Cases []struct {
		Name    string          `json:"name"`
		Expr    json.RawMessage `json:"expr"`
		Formula string          `json:"formula"`
		Base    string          `json:"base"`
		Env     Env             `json:"env"`
		Want    *float64        `json:"want"`
		Error   string          `json:"error"`
	} `json:"cases"`
}

func TestFixtures(t *testing.T) {
	const dir = "shadowrun/5e"
	paths, err := fs.Glob(fixtures.FS(), dir+"/expr/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("nenhuma fixture: %v", err)
	}
	catalogs := data.Systems()

	for _, p := range paths {
		raw, _ := fs.ReadFile(fixtures.FS(), p)
		var f caseFile
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for _, c := range f.Cases {
			t.Run(f.Name+"/"+c.Name, func(t *testing.T) {
				var node any
				if c.Formula != "" {
					node = resolve(t, catalogs, dir, c.Formula)
				} else if err := json.Unmarshal(c.Expr, &node); err != nil {
					t.Fatal(err)
				}
				env := Env{}
				for k, v := range f.Envs[c.Base] {
					env[k] = v
				}
				for k, v := range c.Env {
					env[k] = v
				}

				got, err := Eval(node, env)
				if c.Error != "" {
					var e *Error
					if !errors.As(err, &e) || e.Code != c.Error {
						t.Errorf("got %v, %v; want erro %s", got, err, c.Error)
					}
					return
				}
				if err != nil || c.Want == nil || got != *c.Want {
					t.Errorf("got %v, %v; want %v", got, err, *c.Want)
				}
			})
		}
	}
}

// resolve segue um ponteiro "arquivo.json#a.b.c" nos catálogos.
func resolve(t *testing.T, catalogs fs.FS, dir, pointer string) any {
	t.Helper()
	file, path, _ := strings.Cut(pointer, "#")
	raw, err := fs.ReadFile(catalogs, dir+"/"+file)
	if err != nil {
		t.Fatal(err)
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatal(err)
	}
	for _, part := range strings.Split(path, ".") {
		m, ok := node.(map[string]any)
		if !ok || m[part] == nil {
			t.Fatalf("ponteiro não resolve: %s", pointer)
		}
		node = m[part]
	}
	return node
}

func TestNonStringOpIsUnknown(t *testing.T) {
	_, err := Eval(map[string]any{"op": 3.0, "of": []any{1.0}}, nil)
	var e *Error
	if !errors.As(err, &e) || e.Code != UnknownOp {
		t.Errorf("err = %v", err)
	}
}
