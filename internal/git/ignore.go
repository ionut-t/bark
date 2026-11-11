package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// IgnoreFileName is the file at a repository's root that lists paths Bark
// leaves out of every diff it sends to a model, on top of the defaults.
const IgnoreFileName = ".barkignore"

// defaultIgnore are generated and dependency files that add token cost
// without useful review signal: lock files and checksums (the manifest that
// changed them is still reviewed), generated protobuf code and minified
// assets. A .barkignore line "!<pattern>" turns one of them off.
var defaultIgnore = []string{
	"*.sum", "*.lock", "*.pb.go", "*.pb.gw.go",
	"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "bun.lockb",
	"*.min.js", "*.min.css",
}

// protectedPaths are never ignored: the files that decide how Bark reviews
// a change. In CI, a pull request's own .barkignore and .bark/config.toml
// apply to its review, so if they could ignore themselves, a pull request
// could hide files or lower the bar without the change being reviewed.
// The second path is config.RepoFilePath relative to the root.
var protectedPaths = []string{IgnoreFileName, ".bark/config.toml"}

// ignoreRules are the paths left out of diffs. Each glob uses git's
// pathspec glob syntax, anchored at the repository root, and has a regexp
// that matches the same paths, for diffs git doesn't produce (pull requests
// come from gh).
type ignoreRules struct {
	globs []string
	res   []*regexp.Regexp
}

// matches reports whether the repository-relative path is ignored.
func (r ignoreRules) matches(path string) bool {
	return slices.ContainsFunc(r.res, func(re *regexp.Regexp) bool { return re.MatchString(path) })
}

// pathspecs returns git pathspecs that leave the ignored paths out. They
// work from any directory of the repository.
func (r ignoreRules) pathspecs() []string {
	specs := make([]string, len(r.globs))
	for i, g := range r.globs {
		specs[i] = ":(top,exclude,glob)" + g
	}
	return specs
}

// add appends the globs for one gitignore-style pattern.
func (r *ignoreRules) add(pattern string) error {
	for _, g := range patternGlobs(pattern) {
		re, err := globRegexp(g)
		if err != nil {
			return err
		}
		r.globs = append(r.globs, g)
		r.res = append(r.res, re)
	}
	return nil
}

// parseIgnore returns the defaults plus the patterns in a .barkignore file.
// The syntax follows .gitignore: a pattern without a slash matches at any
// depth, a leading or middle slash anchors it at the root, a trailing slash
// matches a directory, "*" and "?" stay within one directory and "**"
// crosses them, and "#" starts a comment. "!" can only turn off one of the
// defaults: git can't add back what an earlier pattern left out.
//
// The error lists every line that can't be used; the rules returned still
// hold the defaults and every valid line.
func parseIgnore(content string) (ignoreRules, error) {
	type entry struct {
		pattern string
		line    int
	}
	entries := make([]entry, len(defaultIgnore))
	for i, p := range defaultIgnore {
		entries[i] = entry{pattern: p}
	}

	type lineError struct {
		line int
		err  error
	}
	var bad []lineError
	for i, raw := range strings.Split(content, "\n") {
		n := i + 1
		p := strings.TrimRight(raw, " \t\r")
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		if negated, ok := strings.CutPrefix(p, "!"); ok {
			j := slices.IndexFunc(entries, func(e entry) bool { return e.line == 0 && e.pattern == negated })
			if j < 0 {
				bad = append(bad, lineError{n, fmt.Errorf("line %d: can't use %q: only the default patterns (%s) can be turned off with '!'",
					n, p, strings.Join(defaultIgnore, ", "))})
				continue
			}
			entries = slices.Delete(entries, j, j+1)
			continue
		}
		entries = append(entries, entry{pattern: p, line: n})
	}

	var rules ignoreRules
	for _, e := range entries {
		var one ignoreRules
		if err := one.add(e.pattern); err != nil {
			bad = append(bad, lineError{e.line, fmt.Errorf("line %d: can't use %q: %w", e.line, e.pattern, err)})
			continue
		}
		if i := slices.IndexFunc(protectedPaths, one.matches); i >= 0 {
			bad = append(bad, lineError{e.line, fmt.Errorf("line %d: can't use %q: it matches %s, which is always reviewed",
				e.line, e.pattern, protectedPaths[i])})
			continue
		}
		rules.globs = append(rules.globs, one.globs...)
		rules.res = append(rules.res, one.res...)
	}

	slices.SortStableFunc(bad, func(a, b lineError) int { return a.line - b.line })
	errs := make([]error, len(bad))
	for i, b := range bad {
		errs[i] = b.err
	}
	return rules, errors.Join(errs...)
}

// patternGlobs turns a gitignore-style pattern into root-anchored globs.
// A pattern that isn't marked as a directory also matches everything inside
// a directory of that name, as in .gitignore.
func patternGlobs(p string) []string {
	dir := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if strings.Contains(p, "/") {
		p = strings.TrimPrefix(p, "/")
	} else {
		p = "**/" + p
	}
	if dir {
		return []string{p + "/**"}
	}
	return []string{p, p + "/**"}
}

// globRegexp compiles a pathspec glob into a regexp matching the same
// repository-relative paths.
func globRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		atSegmentStart := i == 0 || glob[i-1] == '/'
		switch c := glob[i]; {
		case atSegmentStart && strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case atSegmentStart && glob[i:] == "**":
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '\\' && i+1 < len(glob):
			i++
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		case c == '[':
			class, n, err := bracket(glob[i:])
			if err != nil {
				return nil, err
			}
			b.WriteString(class)
			i += n - 1
		default:
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// bracket converts the character class at the start of s and returns it
// and how many bytes of s it used.
func bracket(s string) (string, int, error) {
	i := 1
	negate := i < len(s) && (s[i] == '!' || s[i] == '^')
	if negate {
		i++
	}
	start := i
	// A ']' right after the opening bracket is a literal.
	if i < len(s) && s[i] == ']' {
		i++
	}
	for i < len(s) && s[i] != ']' {
		i++
	}
	if i >= len(s) {
		return "", 0, errors.New("unclosed [")
	}

	var b strings.Builder
	b.WriteString("[")
	if negate {
		b.WriteString("^/")
	}
	for _, r := range s[start:i] {
		if r == '\\' || r == '[' || r == ']' || r == '^' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteString("]")
	return b.String(), i + 1, nil
}

// loadIgnore returns the rules for the current repository. A .barkignore
// that can't be read or has bad lines still gives the defaults plus its
// valid lines; CheckIgnoreFile reports the problems.
func loadIgnore(ctx context.Context) ignoreRules {
	rules, _ := readIgnoreFile(ctx)
	return rules
}

// excludes returns the pathspecs that leave ignored files out of a git diff.
func excludes(ctx context.Context) []string {
	return loadIgnore(ctx).pathspecs()
}

// CheckIgnoreFile reports problems with the repository's .barkignore:
// a file that can't be read, or lines that can't be used. No repository
// or no file is not a problem.
func CheckIgnoreFile(ctx context.Context) error {
	_, err := readIgnoreFile(ctx)
	return err
}

func readIgnoreFile(ctx context.Context) (ignoreRules, error) {
	defaults, _ := parseIgnore("")
	root, err := RepoRoot(ctx)
	if err != nil {
		return defaults, nil
	}

	path := filepath.Join(root, IgnoreFileName)
	content, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}

	if err != nil {
		return defaults, err
	}

	rules, err := parseIgnore(string(content))
	if err != nil {
		return rules, fmt.Errorf("%s:\n%w", path, err)
	}

	return rules, nil
}
