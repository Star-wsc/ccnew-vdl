package auth

import (
	"path/filepath"
	"testing"
)

func TestSetupLoginLogout(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Configured() {
		t.Fatal("should not be configured")
	}
	if err := st.Setup("admin", "short"); err != ErrWeakPassword {
		t.Fatalf("weak password: %v", err)
	}
	if err := st.Setup("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if err := st.Setup("admin", "password123"); err != ErrAlreadySetup {
		t.Fatalf("double setup: %v", err)
	}
	if _, err := st.Login("admin", "wrongpass1"); err != ErrBadCredentials {
		t.Fatalf("bad login: %v", err)
	}
	tok, err := st.Login("admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Validate(tok) {
		t.Fatal("token should validate")
	}
	if st.Validate("nope") {
		t.Fatal("invalid token accepted")
	}
	st.Logout(tok)
	if st.Validate(tok) {
		t.Fatal("logged out token still valid")
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	st, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Setup("u", "password123"); err != nil {
		t.Fatal(err)
	}
	st2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st2.Configured() || st2.Username() != "u" {
		t.Fatal("reload failed")
	}
	if _, err := st2.Login("u", "password123"); err != nil {
		t.Fatal(err)
	}
}

func TestResolveMode(t *testing.T) {
	if ResolveMode("off", false) != "off" {
		t.Fatal("explicit off")
	}
	if ResolveMode("ON", true) != "on" {
		t.Fatal("explicit on overrides desktop")
	}
	if ResolveMode("", true) != "off" {
		t.Fatal("desktop default off")
	}
	if ResolveMode("", false) != "on" {
		t.Fatal("server default on")
	}
}
