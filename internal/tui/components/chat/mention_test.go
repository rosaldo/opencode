package chat

import "testing"

func TestSplitMention(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantClean string
		wantName  string
	}{
		{"leading", "@reviewer check the auth flow", "check the auth flow", "reviewer"},
		{"mid sentence", "please @docs-writer update the readme", "please update the readme", "docs-writer"},
		{"only mention", "@reviewer", "@reviewer", "reviewer"},
		{"dotted name", "@team.security audit", "audit", "team.security"},
		{"underscore", "@code_review look", "look", "code_review"},
		{"no mention", "just a normal message", "just a normal message", ""},
		{"email not a mention", "mail me at a@b.com", "mail me at a@b.com", ""},
		{"first of two wins", "@one and @two", "and @two", "one"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clean, name := splitMention(tc.in)
			if clean != tc.wantClean || name != tc.wantName {
				t.Fatalf("splitMention(%q) = (%q, %q), want (%q, %q)", tc.in, clean, name, tc.wantClean, tc.wantName)
			}
		})
	}
}
