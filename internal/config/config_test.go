package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDefaultYAMLIsComplete(t *testing.T) {
	d := Default()
	if d.Version != Version {
		t.Errorf("default.yaml is version %d, kfs writes %d", d.Version, Version)
	}
	// Every color must come from theme.yaml: nothing else fills them.
	v := reflect.ValueOf(d.Theme)
	for i := range v.NumField() {
		if f := v.Field(i); f.Kind() == reflect.String && f.String() == "" {
			t.Errorf("theme.yaml has no %s", v.Type().Field(i).Tag.Get("yaml"))
		}
	}
	for _, s := range d.ResourceOrder {
		if d.Theme.Frames["frame-color-"+s] == "" {
			t.Errorf("theme.yaml has no frame color for %s", s)
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
	if c.Version != Version || !reflect.DeepEqual(c.Theme, DefaultTheme()) || !c.StateHidden("replicasets", "old") {
		t.Errorf("defaults: %+v", c)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(defaultYAML) {
		t.Errorf("the first file is default.yaml, comments and all (%v)", err)
	}
	if data, err := os.ReadFile(ThemePath(path)); err != nil || string(data) != string(defaultThemeYAML) {
		t.Errorf("the first theme is theme.yaml, comments and all (%v)", err)
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
	// No hide, no version; a theme with just one color.
	os.WriteFile(path, []byte("sections:\n  hidden: [secrets]\n"), 0o644)
	os.WriteFile(ThemePath(path), []byte("row-referenced: \"#123456\"\n"), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || !c.Hidden("secrets") || !c.StateHidden("pods", "completed") ||
		c.Theme.RowReferenced != "#123456" || c.Theme.StatusOK != DefaultTheme().StatusOK {
		t.Errorf("filled: %+v", c)
	}
}

// A theme from an older kfs gets the colors it lacks added, and keeps its
// own (and its comments).
func TestThemeGetsNewColors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	old := "# mine\nmenu: '#111111'\n"
	os.WriteFile(ThemePath(path), []byte(old), 0o644)
	c, _ := Load(path)
	if c.ThemeErr != nil || c.Theme.Menu != "#111111" || c.Theme.FrameColor("pods") != DefaultTheme().FrameColor("pods") {
		t.Fatalf("theme: %v %+v", c.ThemeErr, c.Theme)
	}
	data, _ := os.ReadFile(ThemePath(path))
	if !strings.HasPrefix(string(data), old) || !strings.Contains(string(data), "frame-color-pods:") || strings.Count(string(data), "\nmenu:") != 1 {
		t.Errorf("theme file:\n%s", data)
	}
	if c2, _ := Load(path); c2.ThemeErr != nil || !reflect.DeepEqual(c2.Theme, c.Theme) {
		t.Errorf("reloaded: %v", c2.ThemeErr)
	}
	if after, _ := os.ReadFile(ThemePath(path)); string(after) != string(data) {
		t.Error("a complete theme is left alone")
	}
}

// A version 2 file's colors move to theme.yaml, and leave config.yaml.
func TestColorsMoveToTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("version: 2\ncolors:\n  linked: \"#123456\"\n  bad: \"#ff0000\"\n"), 0o644)
	c, err := Load(path)
	if err != nil || c.ThemeErr != nil {
		t.Fatal(err, c.ThemeErr)
	}
	if c.Theme.RowReferenced != "#123456" || c.Theme.StatusFailed != "#ff0000" || c.Theme.Menu != DefaultTheme().Menu {
		t.Errorf("theme: %+v", c.Theme)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "colors") {
		t.Errorf("config.yaml keeps its colors:\n%s", data)
	}
	if c, _ := Load(path); c.Theme.RowReferenced != "#123456" {
		t.Errorf("theme.yaml was not written: %+v", c.Theme)
	}
}

// Version 3 moves the old default keys to the k9s-like ones, but not keys
// the user chose.
func TestUpgradeToK9sKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(`version: 2
commands:
  "*": {describe: d, copy: y}
  pods: {logs: l enter, shell: h, forward: p}
  secrets: {open: enter}
`), 0o644)
	c, _ := Load(path)
	for _, tc := range []struct{ section, command, want string }{
		{"pods", "describe", "enter"},
		{"pods", "logs", "l"},
		{"pods", "shell", "s"},
		{"pods", "forward", "p"}, // the user's own
		{"secrets", "extract", "x"},
	} {
		if got := strings.Join(c.CommandKeys(tc.section, tc.command), " "); got != tc.want {
			t.Errorf("%s %s: keys %q, want %q", tc.section, tc.command, got, tc.want)
		}
	}
}

func TestCommandKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	// The user remaps shell, adds yaml everywhere, turns off describe for
	// secrets (and so drops open), and says nothing about claims.
	os.WriteFile(path, []byte(`version: 3
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
		{"secrets", "extract", ""},  // the user's secrets section has no extract
		{"claims", "describe", "d"},
	} {
		if got := strings.Join(c.CommandKeys(tc.section, tc.command), " "); got != tc.want {
			t.Errorf("%s %s: keys %q, want %q", tc.section, tc.command, got, tc.want)
		}
	}
	// A config without commands gets the defaults.
	if got := Default().CommandKeys("pods", "shell"); len(got) != 1 || got[0] != "s" {
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
