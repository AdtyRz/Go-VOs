package security

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("kata-sandi-yang-kuat")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash tidak memakai argon2id: %s", hash)
	}
	ok, err := VerifyPassword("kata-sandi-yang-kuat", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("password benar ditolak")
	}
}

func TestHashPasswordRejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("kata-sandi-yang-kuat")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword("kata-sandi-yang-salah", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("password salah diterima")
	}
}

func TestHashPasswordUsesUniqueSalt(t *testing.T) {
	first, err := HashPassword("kata-sandi-yang-kuat")
	if err != nil {
		t.Fatalf("HashPassword pertama: %v", err)
	}
	second, err := HashPassword("kata-sandi-yang-kuat")
	if err != nil {
		t.Fatalf("HashPassword kedua: %v", err)
	}
	if first == second {
		t.Fatal("dua hash memakai salt yang sama")
	}
}

func TestHashPasswordRejectsShortPassword(t *testing.T) {
	if _, err := HashPassword("pendek"); err == nil {
		t.Fatal("password pendek diterima")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"bukan-hash",
		"$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=19$m=999999999,t=3,p=2$c2FsdA$a2V5",
		"$argon2id$v=19$m=65536,t=3,p=0$c2FsdA$a2V5",
		"$argon2id$v=19$m=65536,t=3,p=2$!!!$a2V5",
	}
	for _, encoded := range cases {
		if _, err := VerifyPassword("apa-saja", encoded); err == nil {
			t.Fatalf("hash rusak diterima: %q", encoded)
		}
	}
}
