package rulestest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"rpg-nexus/api/core/internal/rules"
)

// toy é uma estratégia mínima: um nome obrigatório e até 10 pontos.
type toy struct{}

type toyData struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

var toyCodes = []string{"shape.invalid", "required", "budget.exceeded"}

func (toy) Key() rules.Key { return rules.Key{System: "toy", Version: "1"} }

func (toy) Validate(data json.RawMessage, mode rules.Mode) []rules.Violation {
	var d toyData
	if err := json.Unmarshal(data, &d); err != nil {
		return []rules.Violation{{Step: "points", Path: "points", Code: "shape.invalid"}}
	}
	if mode == rules.Lenient {
		return nil
	}
	var v []rules.Violation
	if d.Name == "" {
		v = append(v, rules.Violation{Step: "concept", Path: "name", Code: "required"})
	}
	if d.Points > 10 {
		v = append(v, rules.Violation{Step: "points", Path: "points", Code: "budget.exceeded",
			Params: map[string]any{"budget": "points", "spent": d.Points, "total": 10}})
	}
	return v
}

func (toy) Derive(data json.RawMessage) (json.RawMessage, error) {
	var d toyData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"budgets": map[string]any{"points": map[string]int{"spent": d.Points, "total": 10}},
		"tags":    []map[string]string{{"id": "a", "label": "A"}},
	})
}

func (toy) Summarize(data json.RawMessage) (rules.Summary, error) {
	var d toyData
	if err := json.Unmarshal(data, &d); err != nil {
		return rules.Summary{}, err
	}
	return rules.Summary{Subtitle: d.Name, Progress: &rules.Progress{Step: 1, Total: 2}}, nil
}

// toyV2 renomeou points para pts no schemaVersion 2.
type toyV2 struct{ toy }

func (toyV2) SchemaVersion() int { return 2 }

func (toyV2) Migrate(data json.RawMessage, from int) (json.RawMessage, error) {
	if from != 1 {
		return nil, fmt.Errorf("de %d", from)
	}
	return json.RawMessage(strings.Replace(string(data), `"points"`, `"pts"`, 1)), nil
}

func (t toyV2) Validate(data json.RawMessage, mode rules.Mode) []rules.Violation {
	return t.toy.Validate(json.RawMessage(strings.Replace(string(data), `"pts"`, `"points"`, 1)), mode)
}

func TestRunWithToy(t *testing.T) {
	Run(t, toy{}, Suite{Fixtures: os.DirFS("testdata/toy"), Codes: toyCodes})
}

func TestMigratesOlderFixture(t *testing.T) {
	f := Fixture{
		Name:          "old",
		SchemaVersion: 1,
		Data:          json.RawMessage(`{"name":"Grok","points":12}`),
		Expect: Expect{Strict: &[]rules.Violation{{Step: "points", Path: "points", Code: "budget.exceeded",
			Params: map[string]any{"budget": "points", "spent": 12, "total": 10}}}},
	}
	if p := Check(toyV2{}, f, toyCodes); len(p) > 0 {
		t.Errorf("problemas: %v", p)
	}
	f.SchemaVersion = 3
	if p := Check(toyV2{}, f, toyCodes); len(p) != 1 {
		t.Errorf("schemaVersion do futuro: %v", p)
	}
}

func TestCheckFindsDivergences(t *testing.T) {
	none := &[]rules.Violation{}
	cases := []struct {
		name   string
		data   string
		expect Expect
		codes  []string
		want   string
	}{
		{
			name:   "violação faltando",
			data:   `{"name":"","points":0}`,
			expect: Expect{Strict: none},
			codes:  toyCodes,
			want:   "strict:",
		},
		{
			name:   "violação a mais na fixture",
			data:   `{"name":"Grok","points":0}`,
			expect: Expect{Strict: &[]rules.Violation{{Step: "concept", Path: "name", Code: "required"}}},
			codes:  toyCodes,
			want:   "strict:",
		},
		{
			name:   "params diferentes",
			data:   `{"name":"Grok","points":11}`,
			expect: Expect{Strict: &[]rules.Violation{{Step: "points", Path: "points", Code: "budget.exceeded"}}},
			codes:  toyCodes,
			want:   "strict:",
		},
		{
			name:  "código não declarado",
			data:  `{"name":"","points":0}`,
			codes: []string{"shape.invalid"},
			want:  `strict: código não declarado "required"`,
		},
		{
			name:   "derivado diferente",
			data:   `{"name":"Grok","points":3}`,
			expect: Expect{Derived: json.RawMessage(`{"budgets":{"points":{"spent":4}}}`)},
			codes:  toyCodes,
			want:   "derived.budgets.points.spent: got 3, want 4",
		},
		{
			name:   "chave ausente no derivado",
			data:   `{"name":"Grok","points":3}`,
			expect: Expect{Derived: json.RawMessage(`{"limits":{}}`)},
			codes:  toyCodes,
			want:   "derived.limits: ausente",
		},
		{
			name:   "lista de tamanho diferente",
			data:   `{"name":"Grok","points":3}`,
			expect: Expect{Derived: json.RawMessage(`{"tags":[]}`)},
			codes:  toyCodes,
			want:   "derived.tags:",
		},
		{
			name:   "resumo diferente",
			data:   `{"name":"Grok","points":3}`,
			expect: Expect{Summary: json.RawMessage(`{"subtitle":"Kestrel"}`)},
			codes:  toyCodes,
			want:   `summary.subtitle: got "Grok", want "Kestrel"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := Fixture{Name: "x", SchemaVersion: 1, Data: json.RawMessage(tc.data), Expect: tc.expect}
			problems := Check(toy{}, f, tc.codes)
			if len(problems) != 1 || !strings.HasPrefix(problems[0], tc.want) {
				t.Errorf("problemas = %q, want um começando com %q", problems, tc.want)
			}
		})
	}
}

type flaky struct {
	toy
	calls *int
}

func (f flaky) Derive(json.RawMessage) (json.RawMessage, error) {
	*f.calls++
	return json.RawMessage(fmt.Sprintf(`{"n":%d}`, *f.calls)), nil
}

func TestCheckFindsNondeterminism(t *testing.T) {
	f := Fixture{Name: "x", SchemaVersion: 1, Data: json.RawMessage(`{"name":"Grok"}`)}
	problems := Check(flaky{calls: new(int)}, f, toyCodes)
	if len(problems) != 1 || !strings.Contains(problems[0], "não é determinístico") {
		t.Errorf("problemas = %q", problems)
	}
}

func TestLoadRejectsNameMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/a.json", []byte(`{"name":"b","schemaVersion":1,"data":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(os.DirFS(dir)); err == nil {
		t.Error("name diferente do arquivo aceito")
	}
}
