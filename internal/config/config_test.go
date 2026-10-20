package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDefaultYAMLIsComplete(t *testing.T) {
	d := Default()
	if d.Version != Version {
		t.Errorf("default.yaml is version %d, kfs writes %d", d.Version, Version)
	}
	// Every color must come from default.yaml: nothing else fills them.
	for name, v := range map[string]string{
		"good": d.Colors.Good, "busy": d.Colors.Busy, "warn": d.Colors.Warn, "bad": d.Colors.Bad, "info": d.Colors.Info,
		"flash": d.Colors.Flash, "selected": d.Colors.Selected, "linked": d.Colors.Linked, "menu": d.Colors.Menu,
	} {
		if v == "" {
			t.Errorf("default.yaml has no color %q", name)
		}
	}
	if !d.StateHidden("pods", "completed") {
		t.Error("default.yaml hides completed pods")
	}
}

func TestMissingFileStartsAsDefaultYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kfs", "config.yaml")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || c.Colors != Defaults || !c.StateHidden("replicasets", "old") {
		t.Errorf("defaults: %+v", c)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(defaultYAML) {
		t.Errorf("the first file is default.yaml, comments and all (%v)", err)
	}
}

func TestStateRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	c, _ := Load(path)
	c.SetHidden("replicasets", true)
	c.SetHidden("deployments", true)
	c.SetHidden("deployments", false)
	c.SetStateHidden("pods", "completed", false)
	c.SetStateHidden("deployments", "scaled-down", true)
	c.SetNamespace("dev", "shop")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Hidden("replicasets") || c.Hidden("deployments") || c.Namespaces["dev"] != "shop" ||
		c.StateHidden("pods", "completed") || !c.StateHidden("deployments", "scaled-down") {
		t.Errorf("reloaded: %+v", c)
	}
}

func TestMissingKeysTakeDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	// No hide, no version, just one color.
	os.WriteFile(path, []byte("sections:\n  hidden: [secrets]\ncolors:\n  linked: \"#123456\"\n"), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || !c.Hidden("secrets") || !c.StateHidden("pods", "completed") ||
		c.Colors.Linked != "#123456" || c.Colors.Good != Defaults.Good {
		t.Errorf("filled: %+v", c)
	}
}

func TestCommandKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	// The user remaps shell, adds yaml everywhere, turns off describe for
	// secrets (and so drops open), and says nothing about claims.
	os.WriteFile(path, []byte(`version: 1
commands:
  "*": {describe: d, yaml: y}
  pods: {shell: x, logs: l enter}
  secrets: {describe: ""}
`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		section, command, want string
	}{
		{"pods", "shell", "x"},
		{"pods", "logs", "l enter"},
		{"pods", "forward", ""}, // not listed: not offered
		{"pods", "describe", "d"},
		{"pods", "yaml", "y"},
		{"secrets", "describe", ""}, // turned off for secrets
		{"secrets", "open", ""},     // the user's secrets section has no open
		{"claims", "describe", "d"},
	} {
		if got := strings.Join(c.CommandKeys(tc.section, tc.command), " "); got != tc.want {
			t.Errorf("%s %s: keys %q, want %q", tc.section, tc.command, got, tc.want)
		}
	}
	// A config without commands gets the defaults.
	if got := Default().CommandKeys("pods", "shell"); len(got) != 1 || got[0] != "h" {
		t.Errorf("default shell key: %v", got)
	}
}

func TestStatusItems(t *testing.T) {
	d := Default()
	if got := strings.Join(d.StatusItems("pods"), " "); got != "lights forwards metrics errors hidden" {
		t.Errorf("pods: %q", got)
	}
	if got := strings.Join(d.StatusItems("volumes"), " "); got != "errors hidden" {
		t.Errorf("volumes: %q", got)
	}
}

func TestResourceOrder(t *testing.T) {
	builtIn := []string{"gitops", "deployments", "pods", "secrets"}
	c := &Config{ResourceOrder: []string{"pods", "gitops", "unknown", "pods"}}
	if got := strings.Join(c.Order(builtIn), " "); got != "pods gitops deployments secrets" {
		t.Errorf("order %q", got)
	}
	if got := strings.Join(Default().Order(builtIn), " "); got != "gitops deployments pods secrets" {
		t.Errorf("default order %q", got)
	}
}

func TestEmptyHideIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("version: 1\nhide: {}\n"), 0o644)
	c, _ := Load(path)
	if c.StateHidden("pods", "completed") {
		t.Error("an empty hide means show everything, not the defaults")
	}
}

func TestChangesPctRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ChangesPct(); got != 33 {
		t.Errorf("default changes_pct = %d, want 33", got)
	}
	c.SetChangesPct(50)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(path)
	if got := c2.ChangesPct(); got != 50 {
		t.Errorf("reloaded changes_pct = %d, want 50", got)
	}
}

func TestFileWithoutLayoutGetsItsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("version: 1\nsections:\n  hidden: [pods]\n"), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ChangesPct(); got != 33 {
		t.Errorf("changes_pct = %d, want default 33", got)
	}
}

func TestEveryVersionHasAnUpgrade(t *testing.T) {
	if len(upgrades) != Version-1 {
		t.Errorf("%d upgrades for version %d", len(upgrades), Version)
	}
}

func TestPreflightUpgradesAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("version: 1\ncommands:\n  \"*\": {describe: d}\n"), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || strings.Join(c.CommandKeys("pods", "copy"), " ") != "y" {
		t.Errorf("upgraded: version %d, copy on %v", c.Version, c.CommandKeys("pods", "copy"))
	}
	var saved Config
	data, _ := os.ReadFile(path)
	if yaml.Unmarshal(data, &saved); saved.Version != Version || saved.Commands["*"]["copy"] != "y" {
		t.Errorf("the upgrade is written back:\n%s", data)
	}

	// The user's own choices stay: y taken by yaml, copy set (or off).
	for _, star := range []string{"{describe: d, yaml: y}", "{copy: c}", `{copy: ""}`} {
		os.WriteFile(path, []byte("version: 1\ncommands:\n  \"*\": "+star+"\n"), 0o644)
		c, _ := Load(path)
		before := Config{}
		yaml.Unmarshal([]byte("commands:\n  \"*\": "+star+"\n"), &before)
		if got, want := c.Commands["*"]["copy"], before.Commands["*"]["copy"]; got != want {
			t.Errorf("%s: copy %q, want %q", star, got, want)
		}
	}
}

func TestNewerFileIsNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	newer := "version: 99\nsections:\n  hidden: [pods]\nsomethingNew: true\n"
	os.WriteFile(path, []byte(newer), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.ReadOnly() || !c.Hidden("pods") {
		t.Errorf("newer file: %+v", c)
	}
	c.SetHidden("secrets", true)
	c.Save()
	if data, _ := os.ReadFile(path); string(data) != newer {
		t.Errorf("a newer file was overwritten:\n%s", data)
	}
}
