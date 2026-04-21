package utils

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	plain := "s3cret-Password!"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("unexpected hash error: %v", err)
	}
	if hash == plain {
		t.Fatal("hash must not equal plaintext")
	}
	if !CheckPassword(plain, hash) {
		t.Fatal("correct password should verify")
	}
	if CheckPassword("wrong", hash) {
		t.Fatal("wrong password must not verify")
	}
}

func TestHashUniquePerInvocation(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("bcrypt hashes should differ due to random salt")
	}
}
