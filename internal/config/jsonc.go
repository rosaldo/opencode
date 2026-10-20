package config

import "strings"

// JSONC (JSON with comments) support. opencode v2 accepts opencode.jsonc files
// that may contain // line comments, /* block */ comments and trailing commas.
// Rather than pull in a parser dependency, stripJsonc removes those before the
// text is handed to encoding/json.

// stripJsonc returns content with comments and trailing commas removed so it can
// be parsed as strict JSON. It runs in two passes — comments first, then
// trailing commas — so a comma followed by a comment is still recognised as
// trailing. Both passes respect JSON string literals, so markers inside strings
// (including escaped quotes) are left untouched.
func stripJsonc(content string) string {
	return stripTrailingCommas(stripComments(content))
}

// stripComments removes // line comments and /* block */ comments, leaving
// string literals intact.
func stripComments(content string) string {
	var b strings.Builder
	b.Grow(len(content))

	inString := false
	escaped := false

	for i := 0; i < len(content); i++ {
		c := content[i]

		if inString {
			b.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
			b.WriteByte(c)
		case '/':
			if i+1 < len(content) && content[i+1] == '/' {
				for i < len(content) && content[i] != '\n' {
					i++
				}
				if i < len(content) {
					b.WriteByte('\n')
				}
			} else if i+1 < len(content) && content[i+1] == '*' {
				i += 2
				for i+1 < len(content) && !(content[i] == '*' && content[i+1] == '/') {
					i++
				}
				i++ // land on the closing '/'
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

// stripTrailingCommas removes a "," when the next non-space character closes an
// object or array. String literals are respected.
func stripTrailingCommas(content string) string {
	var b strings.Builder
	b.Grow(len(content))

	inString := false
	escaped := false

	for i := 0; i < len(content); i++ {
		c := content[i]

		if inString {
			b.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		if c == '"' {
			inString = true
			b.WriteByte(c)
			continue
		}

		if c == ',' {
			j := i + 1
			for j < len(content) && (content[j] == ' ' || content[j] == '\t' || content[j] == '\n' || content[j] == '\r') {
				j++
			}
			if j < len(content) && (content[j] == '}' || content[j] == ']') {
				continue
			}
		}
		b.WriteByte(c)
	}

	return b.String()
}
