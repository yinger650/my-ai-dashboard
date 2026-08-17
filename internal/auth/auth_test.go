package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("verify correct = %v, %v", ok, err)
	}
	bad, _ := VerifyPassword("wrong", hash)
	if bad {
		t.Fatal("verify wrong password should fail")
	}
}

func TestGenerateAPIToken(t *testing.T) {
	cases := map[string]string{
		ScopeMachine: "abp_m_",
		ScopeService: "abp_s_",
		ScopeViewer:  "abp_v_",
	}
	for scope, tag := range cases {
		full, prefix, hash, err := GenerateAPIToken(scope)
		if err != nil {
			t.Fatalf("generate %s: %v", scope, err)
		}
		if !strings.HasPrefix(full, tag) {
			t.Errorf("token %q missing prefix %q", full, tag)
		}
		if len(prefix) != 12 {
			t.Errorf("prefix len = %d, want 12", len(prefix))
		}
		if hash != HashToken(full) {
			t.Errorf("hash mismatch")
		}
		if got, _ := ScopeFromToken(full); got != scope {
			t.Errorf("ScopeFromToken = %q, want %q", got, scope)
		}
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("abc", "abc") {
		t.Error("equal strings should match")
	}
	if ConstantTimeEqual("abc", "abd") {
		t.Error("different strings should not match")
	}
}
