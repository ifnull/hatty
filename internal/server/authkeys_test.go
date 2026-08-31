package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func newKey(t *testing.T) (gossh.PublicKey, string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return sp, string(gossh.MarshalAuthorizedKey(sp))
}

// C1: wish accepts ANY key by default. A missing allow-list must stop the
// daemon, not open it.
func TestMissingAuthorizedKeysRefusesToStart(t *testing.T) {
	_, err := LoadAuthorizedKeys(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, ErrNoAuthorizedKeys) {
		t.Fatalf("got %v, want ErrNoAuthorizedKeys", err)
	}
}

func TestEmptyAuthorizedKeysRefusesToStart(t *testing.T) {
	p := filepath.Join(t.TempDir(), "authorized_keys")
	for _, content := range []string{"", "   \n\n", "# only a comment\n"} {
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadAuthorizedKeys(p); !errors.Is(err, ErrNoAuthorizedKeys) {
			t.Errorf("content %q: got %v, want ErrNoAuthorizedKeys", content, err)
		}
	}
}

func TestOnlyListedKeysAreAccepted(t *testing.T) {
	allowed, line := newKey(t)
	stranger, _ := newKey(t)

	p := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := LoadAuthorizedKeys(p)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Contains(allowed) {
		t.Error("an authorised key was rejected")
	}
	if a.Contains(stranger) {
		t.Error("an unauthorised key was accepted -- this is the C1 hole")
	}
	if a.Contains(nil) {
		t.Error("a nil key was accepted")
	}
}

// E9: SIGHUP reloads, so a key can be added or revoked without dropping every
// live session.
func TestReloadPicksUpAddedAndRevokedKeys(t *testing.T) {
	first, l1 := newKey(t)
	second, l2 := newKey(t)

	p := filepath.Join(t.TempDir(), "authorized_keys")
	os.WriteFile(p, []byte(l1), 0o600)
	a, err := LoadAuthorizedKeys(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Contains(second) {
		t.Fatal("second key accepted before it was added")
	}

	os.WriteFile(p, []byte(l1+l2), 0o600)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if !a.Contains(second) {
		t.Error("reload did not pick up the added key")
	}

	// Revoke the first.
	os.WriteFile(p, []byte(l2), 0o600)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.Contains(first) {
		t.Error("reload did not revoke the removed key")
	}
	if a.Len() != 1 {
		t.Errorf("Len = %d, want 1", a.Len())
	}
}

// A reload that would empty the list must be REFUSED, leaving the previous set
// in place -- otherwise a truncated file silently opens or closes the door
// depending on which way the check falls.
func TestReloadToAnEmptyFileIsRefusedAndKeepsTheOldSet(t *testing.T) {
	k, line := newKey(t)
	p := filepath.Join(t.TempDir(), "authorized_keys")
	os.WriteFile(p, []byte(line), 0o600)
	a, _ := LoadAuthorizedKeys(p)

	os.WriteFile(p, []byte(""), 0o600)
	if err := a.Reload(); !errors.Is(err, ErrNoAuthorizedKeys) {
		t.Fatalf("emptying the file returned %v, want ErrNoAuthorizedKeys", err)
	}
	if !a.Contains(k) {
		t.Error("a refused reload discarded the working key set")
	}
}

func TestMalformedLinesDoNotDiscardValidOnes(t *testing.T) {
	k, line := newKey(t)
	p := filepath.Join(t.TempDir(), "authorized_keys")
	os.WriteFile(p, []byte("this-is-not-a-key\n"+line), 0o600)
	a, err := LoadAuthorizedKeys(p)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Contains(k) {
		t.Error("a malformed line discarded the valid key that followed it")
	}
}
