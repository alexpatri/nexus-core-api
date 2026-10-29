package auth

import (
	"strings"
	"testing"
)

func TestNewToken(t *testing.T) {
	a, b := NewToken(), NewToken()
	if !strings.HasPrefix(a, tokenPrefix) || len(a) != len(tokenPrefix)+43 {
		t.Errorf("token = %q", a)
	}
	if a == b {
		t.Error("dois tokens iguais")
	}
	if HashToken(a) != HashToken(a) || HashToken(a) == HashToken(b) {
		t.Error("hash não é determinístico ou colide")
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		" Alex@Exemplo.COM ": "alex@exemplo.com",
		"alex@exemplo.com":   "alex@exemplo.com",
		"Alex <a@b.com>":     "",
		"sem-arroba":         "",
		"":                   "",
	}
	for in, want := range cases {
		got, ok := NormalizeEmail(in)
		if got != want || ok != (want != "") {
			t.Errorf("NormalizeEmail(%q) = %q, %v", in, got, ok)
		}
	}
}
