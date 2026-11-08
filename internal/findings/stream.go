package findings

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Renderer turns streamed output into Markdown as it arrives, including the
// line still being written, so prose appears token by token. Its output only
// ever grows: each Write returns text to append to what came before.
//
// While a line is unfinished, Renderer shows what has arrived of it. The
// prompt asks for fields in the order they are rendered, so the preview of a
// line is a prefix of its final Markdown. If a model orders fields
// differently, the preview stops growing, and once the line completes its
// final Markdown is written after it.
type Renderer struct {
	partial string
	// shown is what has been written for the unfinished line.
	shown string
}

// Write adds a chunk of output and returns the Markdown to append.
func (r *Renderer) Write(chunk string) string {
	r.partial += chunk

	var b strings.Builder
	for {
		line, ok := nextLine(&r.partial)
		if !ok {
			break
		}
		r.complete(&b, line)
	}

	if pending := pendingMarkdown(r.partial); strings.HasPrefix(pending, r.shown) {
		b.WriteString(pending[len(r.shown):])
		r.shown = pending
	}

	return b.String()
}

// Close returns the Markdown that completes the output. Call it once the
// output ends.
func (r *Renderer) Close() string {
	var b strings.Builder
	line := r.partial
	r.partial = ""
	r.complete(&b, line)
	return b.String()
}

// complete writes what is missing of line's final Markdown.
func (r *Renderer) complete(b *strings.Builder, line string) {
	var final string
	if e, ok := parseLine(line); ok {
		final = Markdown([]Event{e})
	}

	if strings.HasPrefix(final, r.shown) {
		b.WriteString(final[len(r.shown):])
	} else {
		// The preview went a different way than the final line; start over
		// below it.
		b.WriteString("\n\n" + final)
	}
	r.shown = ""
}

// nextLine removes the first complete line from partial and returns it.
func nextLine(partial *string) (string, bool) {
	i := strings.IndexByte(*partial, '\n')
	if i < 0 {
		return "", false
	}
	line := (*partial)[:i]
	*partial = (*partial)[i+1:]
	return line, true
}

// pendingMarkdown renders as much of the unfinished line as can be shown
// without anything later having to change it.
func pendingMarkdown(partial string) string {
	s := strings.TrimSpace(partial)
	switch {
	case s == "", strings.HasPrefix(s, "`"):
		// Blank lines and code fences render as nothing.
		return ""
	case !strings.HasPrefix(s, "{"):
		return strings.TrimRight(trimIncompleteRune(partial), "\r")
	}

	l, open, ok := parsePartial(s)
	if !ok {
		return ""
	}

	switch l.Type {
	case "intro", "verdict":
		text := strings.TrimSpace(l.Text)
		if text == "" {
			return ""
		}
		e := Event{Intro: text}
		if l.Type == "verdict" {
			e = Event{Verdict: text}
		}
		md := Markdown([]Event{e})
		if open == "text" {
			md = strings.TrimSuffix(md, "\n\n")
		}
		return md
	}

	switch open {
	case "", "title", "body", "suggestion", "remark":
	default:
		// A field that renders above the streaming prose isn't complete
		// yet; showing it now could change what is already written.
		return ""
	}

	f := normaliseFinding(l.Finding)
	if f.Title == "" {
		return ""
	}
	if open == "suggestion" {
		// The fence around a suggestion depends on the backticks inside it,
		// so the block is shown once the suggestion is complete.
		f.Suggestion, open = "", ""
	}

	var b strings.Builder
	writeFinding(&b, f, open)
	return b.String()
}

// parsePartial parses the start of a one-line JSON object whose values are
// strings or numbers. It returns the fields that have arrived, with a string
// value still streaming cut at its last complete character, and the key of
// that value ("" if none).
func parsePartial(s string) (line, string, bool) {
	var l line
	if json.Unmarshal([]byte(s), &l) == nil {
		return l, "", true
	}

	const (
		keyOrEnd = iota
		inKey
		colon
		value
		inString
		inLiteral
		afterValue
	)

	state := keyOrEnd
	// s[:safe] + "}" is a valid object holding every complete field.
	safe := 1
	var key string
	keyStart, valueStart := 0, 0
	escStart, hexLeft := -1, 0
	keyEscaped := false

	for i := 1; i < len(s); i++ {
		c := s[i]
		space := c == ' ' || c == '\t' || c == '\r'

		switch state {
		case keyOrEnd:
			switch {
			case c == '"':
				state, keyStart = inKey, i+1
			case !space:
				return line{}, "", false
			}

		case inKey:
			switch {
			case keyEscaped:
				keyEscaped = false
			case c == '\\':
				keyEscaped = true
			case c == '"':
				key, state = s[keyStart:i], colon
			}

		case colon:
			switch {
			case c == ':':
				state = value
			case !space:
				return line{}, "", false
			}

		case value:
			switch {
			case c == '"':
				state, valueStart, escStart = inString, i+1, -1
			case c == '{', c == '[':
				return line{}, "", false
			case !space:
				state = inLiteral
			}

		case inString:
			switch {
			case hexLeft > 0:
				if hexLeft--; hexLeft == 0 {
					escStart = -1
				}
			case escStart >= 0:
				if c == 'u' {
					hexLeft = 4
				} else {
					escStart = -1
				}
			case c == '\\':
				escStart = i
			case c == '"':
				state, safe = afterValue, i+1
			}

		case inLiteral:
			switch {
			case c == ',':
				state, safe = keyOrEnd, i
			case space:
				state, safe = afterValue, i
			}

		case afterValue:
			switch {
			case c == ',':
				state = keyOrEnd
			case !space:
				return line{}, "", false
			}
		}
	}

	repaired := s[:safe] + "}"
	open := ""
	if state == inString {
		end := len(s)
		if escStart >= 0 {
			end = escStart
		}
		repaired = s[:valueStart] + trimIncompleteRune(s[valueStart:end]) + `"}`
		open = key
	}

	l = line{}
	if json.Unmarshal([]byte(repaired), &l) != nil {
		return line{}, "", false
	}
	return l, open, true
}

// trimIncompleteRune drops a UTF-8 sequence cut off at the end of s.
func trimIncompleteRune(s string) string {
	for i := 0; i < utf8.UTFMax-1 && len(s) > 0 && !utf8.ValidString(s); i++ {
		s = s[:len(s)-1]
	}
	return s
}
