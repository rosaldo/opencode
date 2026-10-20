package models

import "testing"

func TestResolveModelID(t *testing.T) {
	tests := []struct {
		ref  string
		want ModelID
		ok   bool
	}{
		{"claude-4-sonnet", Claude4Sonnet, true},
		{"anthropic/claude-4-sonnet", Claude4Sonnet, true},
		{"anthropic/does-not-exist", "", false},
		{"does-not-exist", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := ResolveModelID(tt.ref)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ResolveModelID(%q) = (%q, %v), want (%q, %v)", tt.ref, got, ok, tt.want, tt.ok)
		}
	}
}
