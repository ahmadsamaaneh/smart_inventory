package auth

import (
	"testing"
	"time"

	"github.com/ahmad/smart-inventory/internal/models"
)

func TestJWTIssueAndParse(t *testing.T) {
	mgr := NewManager("test-secret-please-change", 1)
	u := &models.User{ID: 42, Email: "a@b.com", Role: models.RoleAdmin}

	tok, exp, err := mgr.Issue(u)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if !exp.After(time.Now()) {
		t.Fatalf("expiry should be in the future, got %v", exp)
	}

	claims, err := mgr.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != u.ID || claims.Email != u.Email || claims.Role != u.Role {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestJWTRejectsTamperedToken(t *testing.T) {
	mgr := NewManager("secret-1", 1)
	u := &models.User{ID: 1, Email: "x@y", Role: models.RoleCustomer}
	tok, _, _ := mgr.Issue(u)

	// flip last char
	bad := tok[:len(tok)-1] + flip(tok[len(tok)-1])
	if _, err := mgr.Parse(bad); err == nil {
		t.Fatal("tampered token should not parse")
	}
}

func TestJWTRejectsWrongSecret(t *testing.T) {
	a := NewManager("secret-A", 1)
	b := NewManager("secret-B", 1)
	tok, _, _ := a.Issue(&models.User{ID: 1, Email: "x@y", Role: models.RoleCustomer})
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("token signed with different secret should be rejected")
	}
}

func TestJWTRejectsExpiredToken(t *testing.T) {
	mgr := NewManager("secret", 0) // ttl = 0 hours
	tok, _, _ := mgr.Issue(&models.User{ID: 1, Email: "x@y", Role: models.RoleCustomer})
	time.Sleep(50 * time.Millisecond)
	if _, err := mgr.Parse(tok); err == nil {
		t.Fatal("expired token should be rejected")
	}
}

func flip(b byte) string {
	if b == 'a' {
		return "b"
	}
	return "a"
}
