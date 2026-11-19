package git

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// fileTier says how early a file's diff goes when a diff is cut to fit its
// limit: lower tiers are kept first.
type fileTier int

const (
	tierCode fileTier = iota
	tierTest
	tierDocs
	tierFixture
	tierGenerated
)

// fixtureDirs, docDirs and testDirs are directory names whose files belong
// to a tier, whatever their names.
var (
	fixtureDirs = []string{"testdata", "fixtures", "__fixtures__", "__snapshots__"}
	docDirs     = []string{"docs", "doc"}
	testDirs    = []string{"test", "tests", "__tests__"}
)

// pathTier ranks a repository-relative path by its name alone. A file in
// more than one tier gets the latest: a test fixture is a fixture, a README
// among tests is a doc.
func pathTier(p string) fileTier {
	p = strings.ToLower(p)
	dirs := strings.Split(p, "/")
	base := dirs[len(dirs)-1]
	dirs = dirs[:len(dirs)-1]
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	inDir := func(names []string) bool {
		return slices.ContainsFunc(dirs, func(d string) bool { return slices.Contains(names, d) })
	}

	switch {
	case inDir(fixtureDirs), ext == ".snap", ext == ".golden":
		return tierFixture
	case inDir(docDirs),
		slices.Contains([]string{".md", ".mdx", ".rst", ".adoc"}, ext),
		slices.Contains([]string{"license", "licence", "copying", "changelog"}, stem):
		return tierDocs
	case inDir(testDirs),
		strings.HasSuffix(stem, "_test"),
		strings.HasSuffix(stem, ".test"),
		strings.HasSuffix(stem, ".spec"),
		ext == ".py" && strings.HasPrefix(stem, "test_"):
		return tierTest
	}
	return tierCode
}

// generatedMarker matches a comment that marks a file as generated: Go's
// "Code generated ... DO NOT EDIT." and the "@generated" other tools use.
// Only at the start of a comment, so code that mentions them doesn't count.
var generatedMarker = regexp.MustCompile(`^\s*(//|#|/\*|\*|--|;|<!--)\s*(Code generated .*DO NOT EDIT|@generated\b)`)

// generated reports whether a file's section of a diff shows it's
// generated: a marker on an added or unchanged line of its new side.
func generated(section string) bool {
	if !strings.Contains(section, "generated") {
		return false
	}
	hunks := false
	for line := range strings.Lines(section) {
		if strings.HasPrefix(line, "@@") {
			hunks = true
			continue
		}
		if hunks && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ")) &&
			generatedMarker.MatchString(line[1:]) {
			return true
		}
	}
	return false
}

// sectionPath returns the path of the file a section of a diff changes:
// the new path, or the old one for a deleted file. It is "" if the section
// doesn't say.
func sectionPath(section string) string {
	var header, renamed, old string
	for line := range strings.Lines(section) {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "@@"):
			return pathOr(renamed, old, header)
		case strings.HasPrefix(line, fileHeader):
			header = headerPath(strings.TrimPrefix(line, fileHeader))
		case strings.HasPrefix(line, "rename to "):
			renamed = unquotePath(strings.TrimPrefix(line, "rename to "), "")
		case strings.HasPrefix(line, "--- "):
			old = unquotePath(strings.TrimPrefix(line, "--- "), "a/")
		case strings.HasPrefix(line, "+++ "):
			if p := unquotePath(strings.TrimPrefix(line, "+++ "), "b/"); p != "" {
				return p
			}
		}
	}
	return pathOr(renamed, old, header)
}

// pathOr returns the first of paths that isn't "".
func pathOr(paths ...string) string {
	for _, p := range paths {
		if p != "" {
			return p
		}
	}
	return ""
}

// unquotePath reads a path as git prints it after "--- ", "+++ " or
// "rename to ": quoted if it has unusual characters, followed by a tab if
// it has a space, and with prefix. It is "" for /dev/null.
func unquotePath(p, prefix string) string {
	p = strings.TrimSuffix(p, "\t")
	if p == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(p, `"`) {
		unquoted, err := strconv.Unquote(p)
		if err != nil {
			return ""
		}
		p = unquoted
	}
	return strings.TrimPrefix(p, prefix)
}

// headerPath reads the new path from what follows "diff --git ", for a
// section with no "+++ " line (a binary file, a rename or mode change with
// no content change). It is "" if it can't tell.
func headerPath(rest string) string {
	if strings.HasPrefix(rest, `"`) {
		// Both paths quoted, or only the old one: the new path is the last.
		if i := strings.LastIndex(rest, ` "b/`); i >= 0 {
			return unquotePath(rest[i+1:], "b/")
		}
		if i := strings.LastIndex(rest, " b/"); i >= 0 {
			return rest[i+3:]
		}
		return ""
	}
	// The same path twice ("a/<p> b/<p>") is split in the middle, so a path
	// with " b/" in it is read right.
	if n := len(rest); n%2 == 1 {
		mid := n / 2
		if strings.HasPrefix(rest, "a/") && strings.HasPrefix(rest[mid:], " b/") && rest[2:mid] == rest[mid+3:] {
			return rest[mid+3:]
		}
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return unquotePath(rest[i+1:], "b/")
	}
	return ""
}

// diffFile is one file's section of a diff.
type diffFile struct {
	text string
	path string
	tier fileTier
}

// splitFiles splits diff into what comes before its first file (a commit's
// message, for git show) and each file's section.
func splitFiles(diff string) (head string, files []diffFile) {
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		text := cur.String()
		cur.Reset()
		if !strings.HasPrefix(text, fileHeader) {
			head = text
			return
		}
		f := diffFile{text: text, path: sectionPath(text)}
		f.tier = pathTier(f.path)
		if generated(text) {
			f.tier = tierGenerated
		}
		files = append(files, f)
	}
	for line := range strings.Lines(diff) {
		if strings.HasPrefix(line, fileHeader) {
			flush()
		}
		cur.WriteString(line)
	}
	flush()
	return head, files
}

// rankFiles returns diff with its files in the order they are kept when it
// is cut: by tier, and in diff order within a tier. What comes before the
// first file stays first.
func rankFiles(diff string) (string, []diffFile) {
	head, files := splitFiles(diff)
	slices.SortStableFunc(files, func(a, b diffFile) int { return int(a.tier) - int(b.tier) })
	var b strings.Builder
	b.Grow(len(diff))
	b.WriteString(head)
	for _, f := range files {
		b.WriteString(f.text)
	}
	return b.String(), files
}
