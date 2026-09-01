package config

import "testing"

func TestRealRadarDashboardLoads(t *testing.T) {
	d, err := Load("../../dashboards/radar.toml")
	if err != nil {
		t.Fatalf("the shipped dashboard does not validate:\n%v", err)
	}
	t.Logf("loaded %q: %d panels, %d subscriptions", d.Name, len(d.Panels), len(d.Subscriptions()))
	for _, s := range d.Subscriptions() {
		t.Logf("  subscribes: %s", s)
	}
}
