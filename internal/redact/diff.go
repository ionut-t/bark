package redact

import (
	"strconv"
	"strings"
)

// Secret is a secret a diff adds.
type Secret struct {
	// File is the file's path on the new side.
	File string
	// Line is the line the secret starts on in the new version of File.
	Line int
	// Kind names the secret's shape (see Match.Kind).
	Kind string
}

// Added returns the secrets on the lines a unified diff adds, once for each
// line and kind, in the diff's order. Secrets on removed or context lines
// are left out: the change doesn't add them.
func Added(diff string) []Secret {
	_, matches := Mask(diff)
	if len(matches) == 0 {
		return nil
	}
	added := addedLines(diff)
	var out []Secret
	seen := map[Secret]bool{}
	for _, m := range matches {
		at, ok := added[m.Line]
		if !ok {
			continue
		}
		s := Secret{File: at.file, Line: at.line, Kind: m.Kind}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

type newSide struct {
	file string
	line int
}

// addedLines maps the index of each line diff adds to its file and its
// line number in the new version of the file.
func addedLines(diff string) map[int]newSide {
	out := map[int]newSide{}
	var file string
	// next is the new-side number of the next added or context line;
	// left is how many new-side lines the hunk still has.
	var next, left int
	for i, l := range strings.Split(diff, "\n") {
		if m := hunkLine.FindStringSubmatch(l); m != nil {
			next, _ = strconv.Atoi(m[1])
			left = 1
			if m[2] != "" {
				left, _ = strconv.Atoi(m[2])
			}
			continue
		}
		if strings.HasPrefix(l, "diff --git ") {
			// A new file, even if the hunk's counts were wrong.
			left = 0
		}
		if left == 0 {
			if f, ok := fileOf(l); ok && !strings.HasPrefix(l, "### ") {
				file = f
			}
			continue
		}
		switch {
		case strings.HasPrefix(l, "+"):
			out[i] = newSide{file: file, line: next}
			next++
			left--
		case strings.HasPrefix(l, " "), l == "":
			next++
			left--
		}
	}
	return out
}
