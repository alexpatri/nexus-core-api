// Package rulestest é a suíte de contrato que toda estratégia de rules roda no
// próprio contract_test.go, com as fixtures compartilhadas com o app.
package rulestest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"rpg-nexus/api/core/internal/rules"
)

type Suite struct {
	// Fixtures é a pasta <sistema>/<versão> do pacote de fixtures do systems.
	Fixtures fs.FS
	// Codes são todos os códigos de violação que a estratégia pode emitir.
	Codes []string
}

type Fixture struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	SchemaVersion int             `json:"schemaVersion"`
	Data          json.RawMessage `json:"data"`
	Expect        Expect          `json:"expect"`
}

// Campo nil não é conferido; lista vazia exige zero violações.
type Expect struct {
	Lenient *[]rules.Violation `json:"lenient"`
	Strict  *[]rules.Violation `json:"strict"`
	Derived json.RawMessage    `json:"derived"`
	Summary json.RawMessage    `json:"summary"`
}

func Load(fsys fs.FS) ([]Fixture, error) {
	paths, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	fixtures := make([]Fixture, 0, len(paths))
	for _, p := range paths {
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		var f Fixture
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if want := strings.TrimSuffix(path.Base(p), ".json"); f.Name != want {
			return nil, fmt.Errorf("%s: name %q difere do arquivo", p, f.Name)
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

func Run(t *testing.T, sys rules.System, s Suite) {
	t.Helper()

	t.Run("rascunho vazio passa em Lenient", func(t *testing.T) {
		if v := sys.Validate(json.RawMessage(`{}`), rules.Lenient); len(v) > 0 {
			t.Errorf("violações: %v", v)
		}
	})

	fixtures, err := Load(s.Fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("nenhuma fixture")
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			for _, problem := range Check(sys, f, s.Codes) {
				t.Error(problem)
			}
		})
	}
}

// Check devolve tudo em que a estratégia diverge da fixture.
func Check(sys rules.System, f Fixture, codes []string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	data, err := migrate(sys, f)
	if err != nil {
		return []string{err.Error()}
	}

	for _, c := range []struct {
		name   string
		mode   rules.Mode
		expect *[]rules.Violation
	}{
		{"lenient", rules.Lenient, f.Expect.Lenient},
		{"strict", rules.Strict, f.Expect.Strict},
	} {
		got := normalize(sys.Validate(data, c.mode))
		for _, v := range got {
			if !slices.Contains(codes, v.Code) {
				report("%s: código não declarado %q", c.name, v.Code)
			}
		}
		if c.expect == nil {
			continue
		}
		want := normalize(*c.expect)
		if !reflect.DeepEqual(got, want) {
			report("%s:\n  got  %s\n  want %s", c.name, toJSON(got), toJSON(want))
		}
	}

	derived, derr := sys.Derive(data)
	again, aerr := sys.Derive(data)
	if !bytes.Equal(derived, again) || fmt.Sprint(derr) != fmt.Sprint(aerr) {
		report("derived: Derive não é determinístico")
	}
	if f.Expect.Derived != nil {
		if derr != nil {
			report("derived: %v", derr)
		} else {
			problems = append(problems, subset("derived", f.Expect.Derived, derived)...)
		}
	}

	summary, serr := sys.Summarize(data)
	summaryAgain, _ := sys.Summarize(data)
	if !reflect.DeepEqual(summary, summaryAgain) {
		report("summary: Summarize não é determinístico")
	}
	if f.Expect.Summary != nil {
		if serr != nil {
			report("summary: %v", serr)
		} else {
			problems = append(problems, subset("summary", f.Expect.Summary, toJSON(summary))...)
		}
	}

	return problems
}

func migrate(sys rules.System, f Fixture) (json.RawMessage, error) {
	current := rules.SchemaVersion(sys)
	switch {
	case f.SchemaVersion < 1 || f.SchemaVersion > current:
		return nil, fmt.Errorf("schemaVersion %d fora de 1..%d", f.SchemaVersion, current)
	case f.SchemaVersion == current:
		return f.Data, nil
	}
	m := sys.(rules.Migrator)
	data, err := m.Migrate(f.Data, f.SchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("migrate de %d: %w", f.SchemaVersion, err)
	}
	return data, nil
}

// normalize faz as violações passarem pelo JSON, como na resposta da API, e
// ordena, porque a ordem não faz parte do contrato.
func normalize(vs []rules.Violation) []rules.Violation {
	var out []rules.Violation
	_ = json.Unmarshal(toJSON(vs), &out)
	if out == nil {
		out = []rules.Violation{}
	}
	for i := range out {
		if len(out[i].Params) == 0 {
			out[i].Params = nil
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Step != b.Step {
			return a.Step < b.Step
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Code < b.Code
	})
	return out
}

func toJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// subset confere que toda chave de want existe em got com o mesmo valor; listas
// precisam ter o mesmo tamanho, e cada item é conferido como subconjunto.
func subset(where string, wantRaw, gotRaw json.RawMessage) []string {
	var want, got any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		return []string{fmt.Sprintf("%s: fixture: %v", where, err)}
	}
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		return []string{fmt.Sprintf("%s: %v", where, err)}
	}
	var problems []string
	var walk func(p string, want, got any)
	walk = func(p string, want, got any) {
		switch w := want.(type) {
		case map[string]any:
			g, ok := got.(map[string]any)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: got %s, want objeto", p, toJSON(got)))
				return
			}
			keys := make([]string, 0, len(w))
			for k := range w {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				gv, ok := g[k]
				if !ok {
					problems = append(problems, fmt.Sprintf("%s.%s: ausente", p, k))
					continue
				}
				walk(p+"."+k, w[k], gv)
			}
		case []any:
			g, ok := got.([]any)
			if !ok || len(g) != len(w) {
				problems = append(problems, fmt.Sprintf("%s: got %s, want %s", p, toJSON(got), toJSON(want)))
				return
			}
			for i := range w {
				walk(fmt.Sprintf("%s[%d]", p, i), w[i], g[i])
			}
		default:
			if !reflect.DeepEqual(want, got) {
				problems = append(problems, fmt.Sprintf("%s: got %s, want %s", p, toJSON(got), toJSON(want)))
			}
		}
	}
	walk(where, want, got)
	return problems
}
