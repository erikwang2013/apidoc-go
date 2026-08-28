package auth

import (
	"testing"
	"time"
)

func TestCheckPassword(t *testing.T) {
	if !CheckPassword("secret", "hunter2", "hunter2") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword("secret", "hunter2", "hunter3") {
		t.Fatal("wrong password accepted")
	}
}

func TestTokenIssueVerify(t *testing.T) {
	tok, err := Issue("secret", time.Hour, "session")
	if err != nil {
		t.Fatal(err)
	}
	data, ok := Verify("secret", tok)
	if !ok || data != "session" {
		t.Fatalf("verify failed: ok=%v data=%q", ok, data)
	}
	if _, ok := Verify("other-secret", tok); ok {
		t.Fatal("token verified with wrong secret")
	}
	if _, err := Issue("", time.Hour, "x"); err == nil {
		t.Fatal("Issue with empty secret must error")
	}
	if _, ok := Verify("", tok); ok {
		t.Fatal("Verify with empty secret must fail")
	}
}

func TestTokenTamper(t *testing.T) {
	tok, _ := Issue("secret", time.Hour, "session")
	for i := 0; i < len(tok); i++ {
		tampered := tok[:i] + string(rune(tok[i])^1) + tok[i+1:]
		if _, ok := Verify("secret", tampered); ok {
			t.Fatalf("tampered token verified at byte %d", i)
		}
	}
	if _, ok := Verify("secret", "not.a.token"); ok {
		t.Fatal("garbage token verified")
	}
	if _, ok := Verify("secret", ""); ok {
		t.Fatal("empty token verified")
	}
}

func TestTokenExpired(t *testing.T) {
	tok, _ := Issue("secret", -time.Second, "session")
	if _, ok := Verify("secret", tok); ok {
		t.Fatal("expired token verified")
	}
}
