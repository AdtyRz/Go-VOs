package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"regexp"
	"strings"
)

const tokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

const (
	TokenLength                = 5
	MaxBulkTokenCount          = 1000
	maxTokenGenerationAttempts = 12
)

var tokenPattern = regexp.MustCompile(`^[A-Z0-9]{5}$`)

func NormalizeToken(raw string) (string, bool) {
	t := strings.ToUpper(strings.TrimSpace(raw))
	if !tokenPattern.MatchString(t) {
		return "", false
	}
	return t, true
}

func GenerateToken() (string, error) {
	max := big.NewInt(int64(len(tokenAlphabet)))
	out := make([]byte, TokenLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = tokenAlphabet[n.Int64()]
	}
	return string(out), nil
}

func HashToken(secret []byte, token string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strings.ToUpper(strings.TrimSpace(token))))
	return mac.Sum(nil)
}

func HashSession(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func ConstantTimeEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
