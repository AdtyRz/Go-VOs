package security

import (
	"bytes"
	"regexp"
	"testing"
)

func TestNormalizeToken(t *testing.T) {
	cases := map[string]string{
		"abcde":   "ABCDE",
		"  a1b2c": "A1B2C",
		"K9M2P":   "K9M2P",
	}
	for input, want := range cases {
		got, ok := NormalizeToken(input)
		if !ok || got != want {
			t.Fatalf("NormalizeToken(%q) = %q, %v; mau %q, true", input, got, ok, want)
		}
	}
}

func TestNormalizeTokenRejectsInvalid(t *testing.T) {
	cases := []string{"", "ABCD", "ABCDEF", "ABC-1", "ABC 1", "abcde!"}
	for _, input := range cases {
		if got, ok := NormalizeToken(input); ok {
			t.Fatalf("NormalizeToken(%q) diterima sebagai %q", input, got)
		}
	}
}

func TestGenerateTokenFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Z0-9]{5}$`)
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		if !pattern.MatchString(token) {
			t.Fatalf("format token tidak valid: %q", token)
		}
		if seen[token] {
			t.Fatalf("token duplikat dalam 200 percobaan: %q", token)
		}
		seen[token] = true
	}
}

func TestHashTokenStableAndCaseInsensitive(t *testing.T) {
	secret := []byte("rahasia-uji-yang-panjang")
	first := HashToken(secret, "abcde")
	second := HashToken(secret, " ABCDE ")
	if !bytes.Equal(first, second) {
		t.Fatal("hash token berbeda untuk input yang setara")
	}
	other := HashToken([]byte("rahasia-lain"), "ABCDE")
	if bytes.Equal(first, other) {
		t.Fatal("hash token tidak bergantung pada secret")
	}
}

func TestHashSessionDeterministic(t *testing.T) {
	first := HashSession("sesi-rahasia")
	second := HashSession("sesi-rahasia")
	if !bytes.Equal(first, second) {
		t.Fatal("hash sesi tidak deterministik")
	}
	if len(first) != 32 {
		t.Fatalf("panjang hash sesi = %d, mau 32", len(first))
	}
	if bytes.Equal(first, HashSession("sesi-lain")) {
		t.Fatal("sesi berbeda menghasilkan hash yang sama")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("token", "token") {
		t.Fatal("nilai sama dianggap berbeda")
	}
	if ConstantTimeEqual("token", "tokem") {
		t.Fatal("nilai berbeda dianggap sama")
	}
}
