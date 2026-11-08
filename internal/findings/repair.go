package findings

import (
	"fmt"
	"regexp"
	"strings"
)

// keys are the fields a line of output can have. repair uses them to tell
// the quote that ends a value from a quote inside it.
var keys = []string{
	"type", "text",
	"file", "line", "end_line", "severity", "category",
	"title", "body", "original", "suggestion", "remark",
}

// nextField matches what follows the closing quote of a value: the next
// field, or the end of the object.
var nextField = regexp.MustCompile(`^\s*(?:,\s*"(?:` + strings.Join(keys, "|") + `)"\s*:|}\s*$)`)

// repair fixes the mistakes models make when they put code in a JSON string:
// quotes left unescaped, backslashes that don't start a valid escape (as in
// a regexp's \.), and raw tabs. A quote inside a value ends it only if the
// next field or the end of the object follows; otherwise it is escaped.
// repair is only for lines that already failed to parse, and its result
// still has to parse.
func repair(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)

	inString, isKey := false, false
	// expectKey is true where a key can start: after { or ,.
	expectKey := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if !inString {
			switch c {
			case '"':
				inString, isKey = true, expectKey
			case '{', ',':
				expectKey = true
			case ':':
				expectKey = false
			}
			b.WriteByte(c)
			continue
		}

		switch {
		case c == '\\':
			if i+1 < len(s) && validEscape(s[i+1:]) {
				n := 2
				if s[i+1] == 'u' {
					n = 6
				}
				b.WriteString(s[i : i+n])
				i += n - 1
			} else {
				b.WriteString(`\\`)
			}
		case c == '"':
			if isKey || nextField.MatchString(s[i+1:]) {
				inString, expectKey = false, false
				b.WriteByte(c)
			} else {
				b.WriteString(`\"`)
			}
		case c < 0x20:
			fmt.Fprintf(&b, `\u%04x`, c)
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

// validEscape reports whether s, the text after a backslash, starts with a
// valid JSON escape.
func validEscape(s string) bool {
	switch s[0] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return true
	case 'u':
		if len(s) < 5 {
			return false
		}
		for _, h := range s[1:5] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", h) {
				return false
			}
		}
		return true
	}
	return false
}
