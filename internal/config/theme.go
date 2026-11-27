package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Theme is kfs' colors, theme.yaml next to the config. The defaults are
// theme.yaml, compiled in. Colors are "#rrggbb" or ANSI numbers ("0"–"255").
type Theme struct {
	Badge          string `yaml:"badge"`
	TextHeading    string `yaml:"text-heading"`
	TextDim        string `yaml:"text-dim"`
	TextOnColor    string `yaml:"text-on-color"`
	TextMenu       string `yaml:"text-menu"`
	KeyGlobal      string `yaml:"key-global"`
	KeyCommand     string `yaml:"key-command"`
	Menu           string `yaml:"menu"`
	Index          string `yaml:"index"`
	RowHighlighted string `yaml:"row-highlighted"`
	RowReferenced  string `yaml:"row-referenced"`
	Info           string `yaml:"info"`
	StatusOK       string `yaml:"status-ok"`
	StatusBusy     string `yaml:"status-busy"`
	StatusWarn     string `yaml:"status-warn"`
	StatusFailed   string `yaml:"status-failed"`
	Flash          string `yaml:"flash"`
	RailOK         string `yaml:"rail-ok"`
	RailWarn       string `yaml:"rail-warn"`
	RailFailed     string `yaml:"rail-failed"`
	RailIdle       string `yaml:"rail-idle"`
	RailCursor     string `yaml:"rail-cursor"`
	Pulse          string `yaml:"pulse"`
	// Frame is the color of a frame the theme does not name; Frames are
	// the named ones, "frame-color-<section>" (and -kfs, the header).
	Frame  string            `yaml:"frame-color"`
	Frames map[string]string `yaml:",inline"`
}

// FrameColor is the color of a section's frame ("kfs": the header's).
func (t Theme) FrameColor(section string) string {
	if c := t.Frames["frame-color-"+section]; c != "" {
		return c
	}
	return t.Frame
}

//go:embed theme.yaml
var defaultThemeYAML []byte

// DefaultTheme returns the built-in theme (theme.yaml).
func DefaultTheme() Theme {
	var t Theme
	if err := yaml.Unmarshal(defaultThemeYAML, &t); err != nil {
		panic("config: theme.yaml: " + err.Error())
	}
	return t
}

// ThemePath is the theme next to the config at configPath.
func ThemePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "theme.yaml")
}

// LoadTheme reads the theme at path; colors it does not set take their
// defaults and are added to the file, so it always lists every color. A
// missing file is written: the defaults (comments and all), with the colors
// of a config that still had its own.
func LoadTheme(path string, legacy *legacyColors) (Theme, error) {
	t := Theme{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		data = defaultThemeYAML
		if legacy != nil {
			if data, err = withColors(legacy.theme()); err != nil {
				return DefaultTheme(), err
			}
		}
		if err := write(path, data); err != nil {
			return DefaultTheme(), err
		}
	case err != nil:
		return DefaultTheme(), err
	}
	if err := yaml.Unmarshal(data, &t); err != nil {
		return DefaultTheme(), fmt.Errorf("%s: %w", path, err)
	}
	if missing := missingColors(data); len(missing) > 0 {
		add := "\n# Added by kfs: colors this file did not have yet.\n" + strings.Join(missing, "\n") + "\n"
		if !bytes.HasSuffix(data, []byte("\n")) {
			add = "\n" + add
		}
		_ = write(path, append(data, add...)) // the colors apply even if the file cannot be written
	}
	t.fill(DefaultTheme())
	return t, nil
}

// defaultColors are theme.yaml's keys in order, each with its value node.
func defaultColors() (keys []string, values map[string]*yaml.Node, doc *yaml.Node) {
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(defaultThemeYAML, doc); err != nil {
		panic("config: theme.yaml: " + err.Error())
	}
	m := doc.Content[0]
	values = map[string]*yaml.Node{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		keys = append(keys, m.Content[i].Value)
		values[m.Content[i].Value] = m.Content[i+1]
	}
	return keys, values, doc
}

// missingColors are the lines (key: value  # comment) of the default colors
// a theme file does not set.
func missingColors(data []byte) []string {
	var have map[string]any
	_ = yaml.Unmarshal(data, &have)
	keys, values, _ := defaultColors()
	var out []string
	for _, k := range keys {
		if _, ok := have[k]; ok {
			continue
		}
		line := fmt.Sprintf("%s: %q", k, values[k].Value)
		if c := values[k].LineComment; c != "" {
			line += "  " + c
		}
		out = append(out, line)
	}
	return out
}

// withColors is theme.yaml with the colors t sets instead of its own,
// comments and all.
func withColors(t Theme) ([]byte, error) {
	var set map[string]string
	b, err := yaml.Marshal(t)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &set); err != nil {
		return nil, err
	}
	_, values, doc := defaultColors()
	for k, v := range set {
		if n, ok := values[k]; ok && v != "" {
			n.Value = v
		}
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (t *Theme) fill(d Theme) {
	for _, f := range []struct{ v, d *string }{
		{&t.Badge, &d.Badge}, {&t.KeyGlobal, &d.KeyGlobal}, {&t.KeyCommand, &d.KeyCommand},
		{&t.Menu, &d.Menu}, {&t.Index, &d.Index}, {&t.RowHighlighted, &d.RowHighlighted},
		{&t.RowReferenced, &d.RowReferenced}, {&t.Info, &d.Info}, {&t.StatusOK, &d.StatusOK},
		{&t.StatusBusy, &d.StatusBusy}, {&t.StatusWarn, &d.StatusWarn},
		{&t.StatusFailed, &d.StatusFailed}, {&t.Flash, &d.Flash},
		{&t.TextHeading, &d.TextHeading}, {&t.TextDim, &d.TextDim}, {&t.TextOnColor, &d.TextOnColor},
		{&t.TextMenu, &d.TextMenu}, {&t.RailOK, &d.RailOK}, {&t.RailWarn, &d.RailWarn},
		{&t.RailFailed, &d.RailFailed}, {&t.RailIdle, &d.RailIdle}, {&t.RailCursor, &d.RailCursor},
		{&t.Pulse, &d.Pulse}, {&t.Frame, &d.Frame},
	} {
		if *f.v == "" {
			*f.v = *f.d
		}
	}
	if t.Frames == nil {
		t.Frames = map[string]string{}
	}
	for section, c := range d.Frames {
		if t.Frames[section] == "" {
			t.Frames[section] = c
		}
	}
}

// legacyColors are the colors config.yaml had up to version 2; they move
// to theme.yaml.
type legacyColors struct {
	Good     string `yaml:"good"`
	Busy     string `yaml:"busy"`
	Warn     string `yaml:"warn"`
	Bad      string `yaml:"bad"`
	Info     string `yaml:"info"`
	Flash    string `yaml:"flash"`
	Selected string `yaml:"selected"`
	Linked   string `yaml:"linked"`
	Menu     string `yaml:"menu"`
}

func (l legacyColors) theme() Theme {
	return Theme{
		Badge: l.Info, KeyGlobal: l.Info, Menu: l.Menu,
		RowHighlighted: l.Selected, RowReferenced: l.Linked, Info: l.Info,
		StatusOK: l.Good, StatusBusy: l.Busy, StatusWarn: l.Warn, StatusFailed: l.Bad, Flash: l.Flash,
	}
}
