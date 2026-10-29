package auth

import (
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const passwordCost = 12

// 72 bytes é o limite do bcrypt; acima disso o resto da senha seria ignorado.
func ValidPassword(p string) bool {
	return len(p) >= 8 && len(p) <= 72
}

func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), passwordCost)
	return string(b), err
}

func VerifyPassword(p, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

func NormalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", false
	}
	return email, true
}
