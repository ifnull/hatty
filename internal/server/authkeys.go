package server

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"

	gossh "golang.org/x/crypto/ssh"
)

// ErrNoAuthorizedKeys means the daemon must not start.
//
// FINDING C1: wish's default PublicKeyHandler is PERMISSIVE -- with no handler
// configured, ANY key is accepted. Revisions 1 and 2 both said "public-key
// authentication is enforced by the app", describing protection that did not
// exist by default.
//
// This daemon holds a Home Assistant token and serves home-relative aircraft
// bearings and distances, which against public ADS-B history locate the house.
// Default-open on a LAN is not acceptable for that, so a missing or empty
// authorized_keys is a startup failure, not a warning.
var ErrNoAuthorizedKeys = errors.New("server: authorized_keys is missing or empty; refusing to start")

// AuthorizedKeys is a reloadable allow-list.
type AuthorizedKeys struct {
	path string

	mu   sync.RWMutex
	keys [][]byte // marshalled public keys, for constant-shape comparison
}

// LoadAuthorizedKeys reads the allow-list. It fails closed.
func LoadAuthorizedKeys(path string) (*AuthorizedKeys, error) {
	a := &AuthorizedKeys{path: path}
	if err := a.Reload(); err != nil {
		return nil, err
	}
	return a, nil
}

// Reload re-reads the file. Wired to SIGHUP (finding E9) so a key can be added
// or revoked without dropping every live session.
//
// Revocation takes effect for NEW connections; sessions already established are
// unaffected until they reconnect. Stated here rather than implied, because
// "fail closed" suggests a stronger guarantee than static keys deliver.
func (a *AuthorizedKeys) Reload() error {
	raw, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNoAuthorizedKeys, a.path)
		}
		return err
	}
	var keys [][]byte
	rest := raw
	for len(bytes.TrimSpace(rest)) > 0 {
		pk, _, _, next, err := gossh.ParseAuthorizedKey(rest)
		if err != nil {
			// Tolerate a malformed line rather than locking the operator out
			// of a file that is otherwise fine -- but never tolerate an empty
			// result, which is checked below.
			if next == nil || len(next) == len(rest) {
				break
			}
			rest = next
			continue
		}
		keys = append(keys, pk.Marshal())
		rest = next
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: %s", ErrNoAuthorizedKeys, a.path)
	}
	a.mu.Lock()
	a.keys = keys
	a.mu.Unlock()
	return nil
}

// Contains reports whether key is allowed.
func (a *AuthorizedKeys) Contains(key gossh.PublicKey) bool {
	if key == nil {
		return false
	}
	want := key.Marshal()
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, k := range a.keys {
		if bytes.Equal(k, want) {
			return true
		}
	}
	return false
}

// Len reports how many keys are currently allowed.
func (a *AuthorizedKeys) Len() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.keys)
}
