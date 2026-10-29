package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// O prefixo deixa o token reconhecível por scanners de segredo em log e repositório.
const tokenPrefix = "nxs_"

func NewToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
}

// Sem sal: o token já tem 256 bits de entropia, e a busca precisa ser por índice.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
