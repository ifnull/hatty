package ha

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

const fakeToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.THIS_MUST_NEVER_APPEAR"

func TestSecretNeverFormatsItsContents(t *testing.T) {
	s := Secret(fakeToken)
	for _, got := range []string{
		fmt.Sprint(s), fmt.Sprintf("%v", s), fmt.Sprintf("%s", s), fmt.Sprintf("%#v", s),
		fmt.Sprintf("%+v", struct{ Token Secret }{s}),
	} {
		if strings.Contains(got, "THIS_MUST_NEVER_APPEAR") {
			t.Errorf("formatting leaked the secret: %s", got)
		}
	}
	b, err := json.Marshal(struct {
		Token Secret `json:"token"`
	}{s})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "THIS_MUST_NEVER_APPEAR") {
		t.Errorf("marshalling leaked the secret: %s", b)
	}
}

func TestRedactFrameScrubsTheAuthFrame(t *testing.T) {
	frame := []byte(`{"type":"auth","access_token":"` + fakeToken + `"}`)
	got := string(RedactFrame(frame))
	if strings.Contains(got, "THIS_MUST_NEVER_APPEAR") {
		t.Fatalf("RedactFrame left the token in: %s", got)
	}
	if !strings.Contains(got, `"type":"auth"`) {
		t.Fatalf("RedactFrame destroyed the frame: %s", got)
	}
	// Frames without a token must pass through untouched.
	other := []byte(`{"id":1,"type":"subscribe_entities"}`)
	if string(RedactFrame(other)) != string(other) {
		t.Fatalf("RedactFrame altered an unrelated frame")
	}
}

// E8 requires exactly ONE unwrap of the credential. A second caller must fail
// here rather than be noticed in review -- or not.
func TestExactlyOneReveal(t *testing.T) {
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	callers := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "reveal" {
				return true
			}
			callers[fmt.Sprintf("%s:%d", f, fset.Position(call.Pos()).Line)]++
			return true
		})
	}
	if len(callers) != 1 {
		t.Fatalf("reveal() has %d call sites, want exactly 1: %v", len(callers), callers)
	}
}
