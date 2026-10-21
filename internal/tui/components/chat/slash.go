package chat

import "strings"

// InputKind classifies what the prompt input represents.
type InputKind int

const (
	// InputText is an ordinary prompt sent to the agent.
	InputText InputKind = iota
	// InputSlash is a slash command, e.g. "/new" or "/models foo".
	InputSlash
	// InputShell is a shell passthrough, e.g. "!ls -la".
	InputShell
)

// Input is the parsed result of a prompt line.
type Input struct {
	Kind InputKind
	// Name is the command name without the leading slash (InputSlash only).
	Name string
	// Args is everything after the command name, trimmed (InputSlash only).
	Args string
	// Command is the shell command without the leading bang (InputShell only).
	Command string
}

// ParseInput classifies a raw editor value. A leading "/" or "!" only triggers a
// command when it is the very first character (the trigger position), so ordinary
// prose that merely contains them is sent unchanged.
func ParseInput(text string) Input {
	if text == "" {
		return Input{Kind: InputText}
	}

	switch text[0] {
	case '/':
		body := text[1:]
		name, args, _ := strings.Cut(body, " ")
		return Input{
			Kind: InputSlash,
			Name: strings.TrimSpace(name),
			Args: strings.TrimSpace(args),
		}
	case '!':
		return Input{
			Kind:    InputShell,
			Command: strings.TrimSpace(text[1:]),
		}
	default:
		return Input{Kind: InputText}
	}
}

// SlashCommandNames returns the command names advertised by the parser for a
// given registry, used to keep the dialog listing and the parser in sync.
func SlashCommandNames(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, strings.TrimPrefix(id, "/"))
	}
	return out
}
