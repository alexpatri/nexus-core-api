package rules

import (
	"errors"
	"fmt"
	"sort"
)

var ErrUnknownSystem = errors.New("rules: unknown system")

const (
	CapMigrate = "migrate"
	CapActions = "actions"
)

type Registry struct {
	systems map[Key]System
}

func NewRegistry(systems ...System) (*Registry, error) {
	r := &Registry{systems: make(map[Key]System, len(systems))}
	for _, sys := range systems {
		k := sys.Key()
		if k.System == "" || k.Version == "" {
			return nil, fmt.Errorf("rules: chave incompleta %q", k)
		}
		if _, dup := r.systems[k]; dup {
			return nil, fmt.Errorf("rules: %s registrado duas vezes", k)
		}
		r.systems[k] = sys
	}
	return r, nil
}

func (r *Registry) Get(system, version string) (System, error) {
	sys, ok := r.systems[Key{System: system, Version: version}]
	if !ok {
		return nil, ErrUnknownSystem
	}
	return sys, nil
}

type Info struct {
	Key
	SchemaVersion int      `json:"schemaVersion"`
	Capabilities  []string `json:"capabilities"`
}

// Systems descreve o que cada sistema sabe fazer, para o app decidir quais telas mostrar.
func (r *Registry) Systems() []Info {
	infos := make([]Info, 0, len(r.systems))
	for k, sys := range r.systems {
		caps := []string{}
		if _, ok := sys.(Migrator); ok {
			caps = append(caps, CapMigrate)
		}
		if _, ok := sys.(ActionApplier); ok {
			caps = append(caps, CapActions)
		}
		infos = append(infos, Info{Key: k, SchemaVersion: SchemaVersion(sys), Capabilities: caps})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Key.String() < infos[j].Key.String()
	})
	return infos
}
