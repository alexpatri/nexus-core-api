package auth

import (
	"slices"
	"strings"
	"testing"
)

func TestMissingPasswordRules(t *testing.T) {
	cases := []struct {
		password string
		want     []string
	}{
		{"Senha-Forte1", nil},
		{"12345678", []string{RuleUpper, RuleLower, RuleSpecial}},
		{"Ab1!", []string{RuleLength}},
		{"Ab1!" + strings.Repeat("a", 69), []string{RuleLength}},
		{"Ab1!" + strings.Repeat("a", 68), nil},
		{"senhaforte1!", []string{RuleUpper}},
		{"SENHAFORTE1!", []string{RuleLower}},
		{"SenhaForte!!", []string{RuleDigit}},
		{"SenhaForte12", []string{RuleSpecial}},
		{"Senha Forte1", []string{RuleSpecial}},
		{"Ação-çãoÉ1", nil},
		{"Áéíóú1!x", nil},
		{"", []string{RuleLength, RuleUpper, RuleLower, RuleDigit, RuleSpecial}},
	}
	for _, tc := range cases {
		got := MissingPasswordRules(tc.password)
		if !slices.Equal(got, tc.want) {
			t.Errorf("MissingPasswordRules(%q) = %v, want %v", tc.password, got, tc.want)
		}
	}
}

// O mínimo conta caracteres: oito letras acentuadas passam, mesmo ocupando
// mais bytes.
func TestPasswordLengthCountsCharacters(t *testing.T) {
	if !slices.Contains(MissingPasswordRules("Çé1!ãõâ"), RuleLength) {
		t.Error("sete caracteres passaram no mínimo")
	}
	if slices.Contains(MissingPasswordRules("Çé1!ãõâê"), RuleLength) {
		t.Error("oito caracteres acentuados não passaram no mínimo")
	}
}
