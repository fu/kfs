package commands

import (
	"testing"

	"github.com/fu/kfs/internal/resources"
	"github.com/fu/kfs/internal/resources/pods"
	"github.com/fu/kfs/internal/resources/restest"
)

func TestSubRowSelectsContainerDirectly(t *testing.T) {
	h := &restest.Host{}
	opened := ""
	open := func(_ pods.Row, c pods.Container) resources.Step { opened = c.Name; return &resources.Viewer{} }

	// Sub-row: picks the named container directly.
	r := pods.New()
	r.AddForTest(pods.Row{Namespace: "ns", Name: "a", Containers: []pods.Container{
		{Name: "init", Kind: pods.KindInit}, {Name: "app"}, {Name: "proxy", Kind: pods.KindSidecar},
	}})
	tgt := resources.Target{Host: h, Kind: r, Key: "ns/a/proxy", Sub: "proxy"}
	step := pickContainer(tgt, false, open)
	if _, ok := step.(*resources.Viewer); !ok || opened != "proxy" {
		t.Errorf("sub-row proxy: opened %q", opened)
	}

	// Multi-container parent row (no Sub): notice, no picker.
	opened = ""
	tgt = resources.Target{Host: h, Kind: r, Key: "ns/a"}
	step = pickContainer(tgt, false, open)
	if step != nil {
		t.Error("multi-container parent: no step expected")
	}
	if h.Notices() == "" {
		t.Error("multi-container parent: expected a notice")
	}

	// Single-container pod: opens straight away.
	r2 := pods.New()
	r2.AddForTest(pods.Row{Namespace: "ns", Name: "b", Containers: []pods.Container{{Name: "app"}}})
	opened = ""
	tgt = resources.Target{Host: h, Kind: r2, Key: "ns/b"}
	step = pickContainer(tgt, false, open)
	if _, ok := step.(*resources.Viewer); !ok || opened != "app" {
		t.Errorf("single container: opened %q", opened)
	}
}

func TestCommandNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range All() {
		if seen[c.Name] || c.Run == nil {
			t.Errorf("command %q: duplicate or without Run", c.Name)
		}
		seen[c.Name] = true
	}
}
