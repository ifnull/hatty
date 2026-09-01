package config

import (
	"path/filepath"
	"testing"
)

// Every shipped dashboard must validate. A dashboard that fails at load stops
// the daemon (deliberately), so this is the gate that keeps that from being
// discovered on the panel.
func TestShippedDashboardsValidate(t *testing.T) {
	paths, _ := filepath.Glob("../../dashboards/*.toml")
	if len(paths) == 0 {
		t.Skip("no dashboards")
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			d, err := Load(p)
			if err != nil {
				t.Fatalf("%s does not validate:\n%v", p, err)
			}
			if len(d.Subscriptions()) == 0 {
				t.Errorf("%s subscribes to nothing", p)
			}
			t.Logf("%s: %d panels, %d subscriptions", d.Name, len(d.Panels), len(d.Subscriptions()))
		})
	}
}
