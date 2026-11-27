package auth_test

import (
	"testing"

	"github.com/metacubex/sing/common/auth"
)

func TestAuthenticatorConstantTimeVerify(t *testing.T) {
	au := auth.NewAuthenticator([]auth.User{
		{Username: "alice", Password: "secret-password-1"},
		{Username: "alice", Password: "secret-password-2"},
	})
	if !au.Verify("alice", "secret-password-1") {
		t.Fatalf("expected first password to verify")
	}
	if !au.Verify("alice", "secret-password-2") {
		t.Fatalf("expected second password to verify")
	}
	if au.Verify("alice", "secret-password") {
		t.Fatalf("prefix password must not verify")
	}
	if au.Verify("alice", "secret-password-1-extra") {
		t.Fatalf("longer password must not verify")
	}
	if au.Verify("bob", "secret-password-1") {
		t.Fatalf("unknown user must not verify")
	}
}
