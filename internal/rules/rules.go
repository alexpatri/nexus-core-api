package rules

import (
	"encoding/json"
	"time"
)

type Key struct {
	System  string `json:"system"`
	Version string `json:"version"`
}

func (k Key) String() string {
	return k.System + "/" + k.Version
}

type Mode int

const (
	// Lenient roda em todo salvamento e só recusa o que é estruturalmente inválido.
	Lenient Mode = iota
	// Strict aplica as regras do livro; bloqueia no finalize e vira aviso na ficha pronta.
	Strict
)

type System interface {
	Key() Key
	Validate(data json.RawMessage, mode Mode) []Violation
	Derive(data json.RawMessage) (json.RawMessage, error)
	Summarize(data json.RawMessage) (Summary, error)
}

type Summary struct {
	Subtitle   string      `json:"subtitle"`
	Highlights []Highlight `json:"highlights,omitempty"`
	Progress   *Progress   `json:"progress,omitempty"`
}

type Progress struct {
	Step  int `json:"step"`
	Total int `json:"total"`
}

type Highlight struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Violation struct {
	Step   string         `json:"step"`
	Path   string         `json:"path"`
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

type Migrator interface {
	SchemaVersion() int
	Migrate(data json.RawMessage, from int) (json.RawMessage, error)
}

type ActionApplier interface {
	Apply(data json.RawMessage, a Action) (json.RawMessage, []Event, error)
}

type Action struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	At      time.Time       `json:"at"`
}

// SchemaVersion é a versão atual do formato de data; sem Migrator, o formato nunca mudou.
func SchemaVersion(sys System) int {
	if m, ok := sys.(Migrator); ok {
		return m.SchemaVersion()
	}
	return 1
}
