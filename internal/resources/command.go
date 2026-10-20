package resources

// Command is something the selected row can do: show logs, open a shell,
// describe it. Kind-specific commands live in the kind's commands folder
// (resources/pods/commands/shell.go), commands for every kind in
// resources/commands. The config decides which commands a section shows and
// on which keys; the others exist but stay unbound.
type Command struct {
	// Name is how the config refers to it, e.g. "shell".
	Name string
	// Run does it for the selected row; it may act directly and return nil.
	Run func(t Target) Step
	// Enabled, when set, decides whether the command applies to a specific
	// row; nil means always enabled (the command itself returns nil for
	// rows it cannot serve). Used to hide commands from the hints.
	Enabled func(t Target) bool
}

// Commander is a kind whose rows have commands of their own (logs, shell,
// …), next to those every kind's rows have (resources/commands: describe,
// yaml, copy). A kind embeds Own and is given them in its New: its commands
// folder imports the kind, so the kind cannot import them.
type Commander interface {
	Commands() []Command
}

// Own are a kind's own commands; embedding it makes the kind a Commander.
type Own []Command

func (o Own) Commands() []Command { return o }

// Target is the row a command runs on.
type Target struct {
	Host Host
	Kind Resource // the kind showing the row, e.g. *pods.Resource
	Key  string   // the row's key in that kind
	Ref  Ref      // the object it shows
	Sub  string   // sub-element of the row (e.g. container name)
}
