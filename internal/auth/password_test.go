package auth

import "testing"

func TestHashPassword_Roundtrip(t *testing.T) {
	pw := "Sup3rSecret!"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" || hash == pw {
		t.Fatalf("hash looks invalid: %q", hash)
	}
	if !CheckPassword(hash, pw) {
		t.Fatalf("CheckPassword should accept correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatalf("CheckPassword should reject wrong password")
	}
}

func TestHashPassword_DifferentSalts(t *testing.T) {
	pw := "samePassword!"
	a, _ := HashPassword(pw)
	b, _ := HashPassword(pw)
	if a == b {
		t.Fatalf("expected distinct hashes for the same password (salted)")
	}
	if !CheckPassword(a, pw) || !CheckPassword(b, pw) {
		t.Fatalf("both hashes should verify the original password")
	}
}
