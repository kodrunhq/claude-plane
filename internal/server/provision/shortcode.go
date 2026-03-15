package provision

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// shortCodeAlphabet excludes ambiguous characters: O/0, I/1, L.
const shortCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
const shortCodeLength = 6

// GenerateShortCode produces a cryptographically random 6-character code
// from a 31-character alphabet (no ambiguous chars).
func GenerateShortCode() (string, error) {
	alphabetLen := big.NewInt(int64(len(shortCodeAlphabet)))
	code := make([]byte, shortCodeLength)
	for i := range code {
		idx, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		code[i] = shortCodeAlphabet[idx.Int64()]
	}
	return string(code), nil
}

// ValidateShortCode checks that a code is exactly 6 characters from the
// valid alphabet. Returns false for any invalid input.
func ValidateShortCode(code string) bool {
	if len(code) != shortCodeLength {
		return false
	}
	for _, ch := range code {
		if !strings.ContainsRune(shortCodeAlphabet, ch) {
			return false
		}
	}
	return true
}
