package commands

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fu/kfs/internal/resources"
)

// Copy puts the row's object name on the clipboard: through the local
// clipboard tool (pbcopy, wl-copy, xclip, xsel), or, over SSH or without
// one, an OSC 52 escape that the terminal itself copies from.
var Copy = resources.Command{
	Name: "copy",
	Run: func(t resources.Target) resources.Step {
		if t.Ref.Name == "" {
			return nil
		}
		if err := clipboard(t.Ref.Name); err != nil {
			t.Host.Notice("copy: " + err.Error())
			return nil
		}
		t.Host.Notice("copied " + t.Ref.Name)
		return nil
	},
}

// clipboards are the local tools tried in order, with their arguments.
var clipboards = [][]string{
	{"pbcopy"},
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
	{"xsel", "--clipboard", "--input"},
}

func clipboard(s string) error {
	if os.Getenv("SSH_TTY") == "" {
		for _, c := range clipboards {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(s)
			if cmd.Run() == nil {
				return nil
			}
		}
	}
	// OSC 52 on stderr: the same terminal, but not the stream Bubble Tea draws to.
	_, err := fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(s)))
	return err
}
