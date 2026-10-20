package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/opencode-ai/opencode/internal/logging"
)

// interpolate expands the opencode v2 placeholder forms in s, using baseDir to
// resolve relative file paths:
//
//	{env:VAR}   -> the value of the environment variable (empty if unset)
//	{file:path} -> the contents of the file (empty if unreadable); path may be
//	               absolute, start with "~/" (home), or be relative to baseDir
//
// Multiple placeholders may appear in one string. Unknown "{...}" text is left
// as-is. It is applied to every string in the config (Spec 11a).
func interpolate(s, baseDir string) string {
	if !strings.ContainsRune(s, '{') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		end += i
		token := s[i+1 : end]
		switch {
		case strings.HasPrefix(token, "env:"):
			b.WriteString(os.Getenv(strings.TrimPrefix(token, "env:")))
			i = end + 1
		case strings.HasPrefix(token, "file:"):
			b.WriteString(readInterpolatedFile(strings.TrimPrefix(token, "file:"), baseDir))
			i = end + 1
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// readInterpolatedFile reads a "{file:...}" reference. An absolute path or one
// starting with "~/" is used directly; anything else is relative to baseDir.
func readInterpolatedFile(path, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	resolved := path
	switch {
	case strings.HasPrefix(path, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			resolved = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	case !filepath.IsAbs(path):
		resolved = filepath.Join(baseDir, path)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		logging.Warn("failed to read {file:} reference", "file", resolved, "error", err)
		return ""
	}
	return string(data)
}
