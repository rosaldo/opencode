package chat

import "testing"

func TestParseInput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Input
	}{
		{"empty", "", Input{Kind: InputText}},
		{"plain text", "hello world", Input{Kind: InputText}},
		{"slash alone", "/", Input{Kind: InputSlash, Name: ""}},
		{"slash command", "/new", Input{Kind: InputSlash, Name: "new"}},
		{"slash with args", "/rename my session", Input{Kind: InputSlash, Name: "rename", Args: "my session"}},
		{"slash trailing space", "/models ", Input{Kind: InputSlash, Name: "models", Args: ""}},
		{"slash not at start", "run /new now", Input{Kind: InputText}},
		{"bang alone", "!", Input{Kind: InputShell, Command: ""}},
		{"bang command", "!ls -la", Input{Kind: InputShell, Command: "ls -la"}},
		{"bang trims", "!  echo hi  ", Input{Kind: InputShell, Command: "echo hi"}},
		{"bang not at start", "echo !warning", Input{Kind: InputText}},
		{"mid slash is text", "path/to/file", Input{Kind: InputText}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseInput(tc.in)
			if got != tc.want {
				t.Errorf("ParseInput(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSlashCommandNames(t *testing.T) {
	got := SlashCommandNames([]string{"/new", "/rename", "models"})
	want := []string{"new", "rename", "models"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
