package ha

import "strings"

// Secret holds a credential that must never be printed.
//
// Phase 7 finding E8: a redacting type alone is not enough. It guards %v dumps
// and accidental marshalling, but the token's real exposure is the JSON auth
// frame -- where either MarshalJSON redacts and authentication BREAKS, or the
// code unwraps to a string and the marshalled bytes carry the live credential
// into any protocol trace. Protocol tracing is exactly what gets enabled when
// debugging reconnects.
//
// The closure has three parts, all required:
//
//  1. this type, so accidental formatting or marshalling is safe;
//  2. exactly ONE audited unwrap -- reveal(), unexported, called from a single
//     place: authFrame() in client.go, which builds the frame explicitly rather
//     than by marshalling a struct that contains the secret;
//  3. RedactFrame, applied by the trace path before any frame is logged.
//
// TestTokenNeverAppearsInLogs asserts the whole chain.
type Secret string

const redacted = "[redacted]"

func (Secret) String() string               { return redacted }
func (Secret) GoString() string             { return redacted }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
func (s Secret) Empty() bool                { return len(strings.TrimSpace(string(s))) == 0 }

// reveal returns the underlying credential.
//
// THIS IS THE ONLY UNWRAP. It is unexported and must be called from exactly one
// place. TestExactlyOneReveal asserts the call count by scanning the package, so
// a second caller fails the build's test stage rather than being noticed in
// review -- or not.
func (s Secret) reveal() string { return string(s) }

// RedactFrame removes credential material from a protocol frame before it is
// logged. Applied by the trace path to every outbound frame; enabling protocol
// tracing must not be able to leak the token.
func RedactFrame(b []byte) []byte {
	s := string(b)
	i := strings.Index(s, `"access_token"`)
	if i < 0 {
		return b
	}
	// Find the value that follows and replace it wholesale.
	rest := s[i+len(`"access_token"`):]
	c := strings.Index(rest, `:`)
	if c < 0 {
		return []byte(s[:i] + `"access_token":"` + redacted + `"`)
	}
	q1 := strings.Index(rest[c:], `"`)
	if q1 < 0 {
		return []byte(s[:i] + `"access_token":"` + redacted + `"`)
	}
	q1 += c + 1
	q2 := strings.Index(rest[q1:], `"`)
	if q2 < 0 {
		return []byte(s[:i] + `"access_token":"` + redacted + `"`)
	}
	q2 += q1
	return []byte(s[:i] + `"access_token":"` + redacted + `"` + rest[q2+1:])
}
