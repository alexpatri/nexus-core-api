package auth

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const passwordCost = 12

// Regras de senha, na ordem em que o app as mostra embaixo do campo.
const (
	RuleLength  = "length"
	RuleUpper   = "upper"
	RuleLower   = "lower"
	RuleDigit   = "digit"
	RuleSpecial = "special"
)

// MissingPasswordRules devolve as regras que a senha não cumpre. O mínimo conta
// caracteres, como o app; o máximo conta bytes, porque o bcrypt ignora o que
// passa de 72.
func MissingPasswordRules(p string) []string {
	var upper, lower, digit, special bool
	for _, r := range p {
		switch {
		case unicode.Is(unicode.Lu, r):
			upper = true
		case unicode.Is(unicode.Ll, r):
			lower = true
		case unicode.Is(unicode.Nd, r):
			digit = true
		case !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsSpace(r):
			special = true
		}
	}
	var missing []string
	if utf8.RuneCountInString(p) < 8 || len(p) > 72 {
		missing = append(missing, RuleLength)
	}
	for _, rule := range []struct {
		ok   bool
		name string
	}{{upper, RuleUpper}, {lower, RuleLower}, {digit, RuleDigit}, {special, RuleSpecial}} {
		if !rule.ok {
			missing = append(missing, rule.name)
		}
	}
	return missing
}

func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), passwordCost)
	return string(b), err
}

func VerifyPassword(p, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

// O app usa o mesmo padrão no z.email(): domínio com ponto e terminação de
// pelo menos duas letras, o que barra erro de digitação como alex@gmail.
var emailPattern = regexp.MustCompile(`^[^\s@]+@([^\s@.]+\.)+[^\s@.]{2,}$`)

func NormalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !emailPattern.MatchString(email) {
		return "", false
	}
	return email, true
}
