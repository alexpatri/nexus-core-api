package rules

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type stub struct{ key Key }

func (s stub) Key() Key                                      { return s.key }
func (stub) Validate(json.RawMessage, Mode) []Violation      { return nil }
func (stub) Derive(json.RawMessage) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }
func (stub) Summarize(json.RawMessage) (Summary, error)      { return Summary{}, nil }

type migrating struct{ stub }

func (migrating) SchemaVersion() int { return 3 }
func (migrating) Migrate(d json.RawMessage, _ int) (json.RawMessage, error) {
	return d, nil
}

type playable struct{ migrating }

func (playable) Apply(d json.RawMessage, _ Action) (json.RawMessage, []Event, error) {
	return d, nil, nil
}

func TestNewRegistryRejectsBadKeys(t *testing.T) {
	sr := stub{Key{"shadowrun", "5e"}}
	if _, err := NewRegistry(sr, sr); err == nil {
		t.Error("chave duplicada aceita")
	}
	if _, err := NewRegistry(stub{Key{"shadowrun", ""}}); err == nil {
		t.Error("chave sem versão aceita")
	}
}

func TestGet(t *testing.T) {
	r, err := NewRegistry(stub{Key{"shadowrun", "5e"}}, stub{Key{"vileborn", "v1"}})
	if err != nil {
		t.Fatal(err)
	}
	sys, err := r.Get("vileborn", "v1")
	if err != nil || sys.Key() != (Key{"vileborn", "v1"}) {
		t.Errorf("Get = %v, %v", sys, err)
	}
	if _, err := r.Get("shadowrun", "6e"); !errors.Is(err, ErrUnknownSystem) {
		t.Errorf("versão desconhecida: err = %v", err)
	}
}

func TestSystems(t *testing.T) {
	r, err := NewRegistry(
		playable{migrating{stub{Key{"vileborn", "v1"}}}},
		stub{Key{"shadowrun", "5e"}},
		migrating{stub{Key{"dnd", "5e"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []Info{
		{Key: Key{"dnd", "5e"}, SchemaVersion: 3, Capabilities: []string{CapMigrate}},
		{Key: Key{"shadowrun", "5e"}, SchemaVersion: 1, Capabilities: []string{}},
		{Key: Key{"vileborn", "v1"}, SchemaVersion: 3, Capabilities: []string{CapMigrate, CapActions}},
	}
	if got := r.Systems(); !reflect.DeepEqual(got, want) {
		t.Errorf("Systems() =\n  %+v\nwant\n  %+v", got, want)
	}
}
