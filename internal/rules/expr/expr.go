// Package expr avalia as fórmulas dos catálogos: árvores de {"op", "of"} e
// {"ref"} com números nas folhas. É o porte do sr_eval do systems, e as mesmas
// fixtures rodam aqui, lá e no app.
package expr

import "math"

const (
	Malformed  = "malformed"
	UnknownOp  = "unknownOp"
	Arity      = "arity"
	MissingRef = "missingRef"
	DivByZero  = "divByZero"
)

type Error struct {
	Code string
}

func (e *Error) Error() string {
	return "expr: " + e.Code
}

type Env map[string]float64

type op struct {
	min, max int // max 0 é sem limite
	apply    func(xs []float64) float64
}

var ops = map[string]op{
	"add": {1, 0, func(xs []float64) float64 {
		sum := 0.0
		for _, x := range xs {
			sum += x
		}
		return sum
	}},
	"mul": {1, 0, func(xs []float64) float64 {
		p := 1.0
		for _, x := range xs {
			p *= x
		}
		return p
	}},
	"min":   {1, 0, func(xs []float64) float64 { return fold(xs, math.Min) }},
	"max":   {1, 0, func(xs []float64) float64 { return fold(xs, math.Max) }},
	"sub":   {2, 2, func(xs []float64) float64 { return xs[0] - xs[1] }},
	"div":   {2, 2, func(xs []float64) float64 { return xs[0] / xs[1] }},
	"ceil":  {1, 1, func(xs []float64) float64 { return math.Ceil(xs[0]) }},
	"floor": {1, 1, func(xs []float64) float64 { return math.Floor(xs[0]) }},
}

func fold(xs []float64, f func(a, b float64) float64) float64 {
	acc := xs[0]
	for _, x := range xs[1:] {
		acc = f(acc, x)
	}
	return acc
}

// Eval recebe a fórmula como sai de json.Unmarshal em any. A aridade é
// conferida antes de avaliar os filhos, da esquerda para a direita, para o
// primeiro erro ser o mesmo nos três avaliadores.
func Eval(node any, env Env) (float64, error) {
	switch n := node.(type) {
	case float64:
		return n, nil
	case map[string]any:
		if ref, ok := n["ref"]; ok && len(n) == 1 {
			name, ok := ref.(string)
			if !ok {
				return 0, &Error{Malformed}
			}
			v, ok := env[name]
			if !ok {
				return 0, &Error{MissingRef}
			}
			return v, nil
		}
		return evalOp(n, env)
	}
	return 0, &Error{Malformed}
}

func evalOp(n map[string]any, env Env) (float64, error) {
	name, hasOp := n["op"]
	args, isList := n["of"].([]any)
	if len(n) != 2 || !hasOp || !isList {
		return 0, &Error{Malformed}
	}
	s, _ := name.(string)
	o, ok := ops[s]
	if !ok {
		return 0, &Error{UnknownOp}
	}
	if len(args) < o.min || (o.max > 0 && len(args) > o.max) {
		return 0, &Error{Arity}
	}

	xs := make([]float64, len(args))
	for i, a := range args {
		x, err := Eval(a, env)
		if err != nil {
			return 0, err
		}
		xs[i] = x
	}
	if s == "div" && xs[1] == 0 {
		return 0, &Error{DivByZero}
	}
	return o.apply(xs), nil
}
