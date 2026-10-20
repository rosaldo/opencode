package config

import (
	"encoding/json"
	"testing"
)

func TestStripJsonc(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "line comment",
			in:   "{\n  // hi\n  \"a\": 1\n}",
			want: "{\n  \n  \"a\": 1\n}",
		},
		{
			name: "block comment",
			in:   "{/* c */\"a\": 1}",
			want: "{\"a\": 1}",
		},
		{
			name: "trailing comma object",
			in:   "{\"a\": 1,}",
			want: "{\"a\": 1}",
		},
		{
			name: "trailing comma array",
			in:   "[1, 2, ]",
			want: "[1, 2 ]",
		},
		{
			name: "comment markers inside string kept",
			in:   "{\"a\": \"http://x // y /* z */\"}",
			want: "{\"a\": \"http://x // y /* z */\"}",
		},
		{
			name: "escaped quote inside string",
			in:   "{\"a\": \"he said \\\"//not a comment\\\"\"}",
			want: "{\"a\": \"he said \\\"//not a comment\\\"\"}",
		},
		{
			name: "comment between value and trailing comma",
			in:   "{\"a\": 1, /* c */}",
			want: "{\"a\": 1 }",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripJsonc(tc.in); got != tc.want {
				t.Errorf("stripJsonc(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripJsoncParses(t *testing.T) {
	in := `{
		// a comment
		"agents": {
			"a": { "mode": "subagent", }, /* inline */
		},
	}`
	// stripJsonc output must be valid JSON when comments/trailing commas are the
	// only extras.
	var out map[string]any
	if err := json.Unmarshal([]byte(stripJsonc(in)), &out); err != nil {
		t.Fatalf("stripJsonc output is not valid JSON: %v", err)
	}
}
