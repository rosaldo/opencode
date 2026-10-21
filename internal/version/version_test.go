package version

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"v1.0.2", "1.0.2"},
		{"1.0.2", "1.0.2"},
		{"v1.2.3+incompatible", "1.2.3"},
		{"v0.1.0+dirty", "0.1.0"},
		{"1.0.0+meta", "1.0.0"},
		{"v1.0.0-beta.1", "1.0.0-beta.1"},
		{"", ""},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		if got := normalize(c.in); got != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The version the test binary reports must already be in normalized form.
func TestVersionIsNormalized(t *testing.T) {
	if len(Version) > 0 && (Version[0] == 'v' || containsByte(Version, '+')) {
		t.Fatalf("Version %q is not normalized", Version)
	}
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}
