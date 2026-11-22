// Package houserules reads a repository's own guidance for contributors and
// coding agents (AGENTS.md, CLAUDE.md, or the files its config lists) so a
// review can hold a change to it.
package houserules

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ionut-t/bark/v2/internal/tokens"
)

// DefaultFiles are the files read when the config lists none.
var DefaultFiles = []string{"AGENTS.md", "CLAUDE.md"}

// DefaultMaxTokens caps the house rules in a prompt when Options doesn't.
const DefaultMaxTokens = 2000

// ReadFunc reads a repository-relative path at the version under review.
type ReadFunc func(ctx context.Context, path string) ([]byte, error)

// Options says which files to read.
type Options struct {
	// Files are the names and paths to read, highest priority first. A name
	// with no slash is looked for in every folder above a changed file,
	// closest first, then at the root; a path with a slash ("docs/STYLE.md",
	// "./AGENTS.md") is read from the root only. Empty means DefaultFiles.
	Files []string
	// Changed are the repository-relative paths the diff touches.
	Changed []string
	// MaxTokens caps the text; 0 means DefaultMaxTokens.
	MaxTokens int
}

// Source is one file the rules were read from.
type Source struct {
	Path string
	// Target is the file Path points to, read in its place (see follow);
	// "" if Path isn't a pointer.
	Target string
	// Tokens is about how many tokens of the file are in Text; 0 if it was
	// left out.
	Tokens int
	// FullTokens is about how many tokens the whole file is.
	FullTokens int
}

// Cut reports whether only part of the file is in the text.
func (s Source) Cut() bool { return s.Tokens > 0 && s.Tokens < s.FullTokens }

// Rules is what Load read.
type Rules struct {
	// Text is the files, each in a <file path="..."> block; "" if none.
	Text    string
	Sources []Source
	// Missing are the listed files that weren't found. Missing default
	// files aren't recorded: most repositories don't have them.
	Missing   []string
	maxTokens int
}

// Summary describes the sources for the BARK_DEBUG log, e.g.
// "AGENTS.md (640), CLAUDE.md (cut 300/900), docs/STYLE.md (left out, 400)".
func (r Rules) Summary() string {
	parts := make([]string, 0, len(r.Sources))
	for _, s := range r.Sources {
		switch {
		case s.Tokens == 0:
			parts = append(parts, fmt.Sprintf("%s (left out, %d)", s.Path, s.FullTokens))
		case s.Cut():
			parts = append(parts, fmt.Sprintf("%s (cut %d/%d)", s.Path, s.Tokens, s.FullTokens))
		default:
			parts = append(parts, fmt.Sprintf("%s (%d)", s.Path, s.Tokens))
		}
	}
	return strings.Join(parts, ", ")
}

// LogField describes the house rules for the BARK_DEBUG log: "off" when
// they're turned off, "none found", or the Summary.
func LogField(enabled bool, r Rules) string {
	switch {
	case !enabled:
		return "off"
	case len(r.Sources) == 0:
		return "none found"
	}
	return r.Summary()
}

// Warnings says which listed files weren't found and which were cut or
// left out to fit.
func (r Rules) Warnings() []string {
	var out []string
	if len(r.Missing) > 0 {
		out = append(out, fmt.Sprintf("house rules: %s not found", strings.Join(r.Missing, ", ")))
	}
	var cut []string
	for _, s := range r.Sources {
		switch {
		case s.Tokens == 0:
			cut = append(cut, s.Path+" left out")
		case s.Cut():
			cut = append(cut, s.Path+" cut")
		}
	}
	if len(cut) > 0 {
		out = append(out, fmt.Sprintf("house rules: %s to fit %d tokens", strings.Join(cut, ", "), r.maxTokens))
	}
	return out
}

// ValidateFile checks a configured file: a relative path inside the
// repository, naming a file.
func ValidateFile(f string) error {
	clean := path.Clean(strings.ReplaceAll(f, "\\", "/"))
	switch {
	case strings.TrimSpace(f) == "":
		return errors.New("an empty file name")
	case path.IsAbs(clean):
		return fmt.Errorf("%q is absolute; use a path relative to the repository root", f)
	case clean == "." || strings.HasSuffix(f, "/"):
		return fmt.Errorf("%q isn't a file", f)
	case clean == ".." || strings.HasPrefix(clean, "../"):
		return fmt.Errorf("%q is outside the repository", f)
	}
	return nil
}

// Load reads the house rules for a change. Files that can't be read are
// skipped; the rules never stop a review.
func Load(ctx context.Context, read ReadFunc, opts Options) Rules {
	files := opts.Files
	listed := len(files) > 0
	if !listed {
		files = DefaultFiles
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	type group struct {
		name  string
		paths []string
	}
	var groups []group
	var all []string
	for _, f := range files {
		if ValidateFile(f) != nil {
			continue
		}
		g := group{name: f, paths: candidates(f, opts.Changed)}
		groups = append(groups, g)
		all = append(all, g.paths...)
	}
	contents := readAll(ctx, read, all)

	rules := Rules{maxTokens: maxTokens}
	seen := map[string]bool{}
	var b strings.Builder
	budget := tokens.Bytes(uint32(maxTokens))
	full := false
	for _, g := range groups {
		found := false
		for _, p := range g.paths {
			content, ok := contents[p]
			if !ok {
				continue
			}
			found = true
			content, target := follow(ctx, read, p, content)
			key := strings.TrimSpace(content)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true

			body := strings.TrimRight(content, "\n") + "\n"
			src := Source{Path: p, Target: target, FullTokens: tokenCount(body)}
			if !full {
				open := fmt.Sprintf("<file path=%q>\n", p)
				const closing = "</file>\n\n"
				if len(open)+len(body)+len(closing) <= budget {
					b.WriteString(open)
					b.WriteString(body)
					b.WriteString(closing)
					budget -= len(open) + len(body) + len(closing)
					src.Tokens = src.FullTokens
				} else {
					full = true
					const marker = "[cut]\n"
					if kept := cutAt(body, budget-len(open)-len(marker)-len(closing)); kept != "" {
						b.WriteString(open)
						b.WriteString(kept)
						b.WriteString(marker)
						b.WriteString(closing)
						// Always below FullTokens: kept plus the marker fits
						// where body didn't, so body is at least 7 bytes
						// (about 3 tokens) longer.
						src.Tokens = tokenCount(kept)
					}
				}
			}
			rules.Sources = append(rules.Sources, src)
		}
		if listed && !found {
			rules.Missing = append(rules.Missing, g.name)
		}
	}
	rules.Text = strings.TrimRight(b.String(), "\n")
	if rules.Text != "" {
		rules.Text += "\n"
	}
	return rules
}

// tokenCount estimates the tokens in s, at least 1 for any text, so a
// short file isn't taken for one that was left out.
func tokenCount(s string) int {
	return max(1, tokens.Estimate(len(s)))
}

// candidates are the paths to look for f at, highest priority first.
func candidates(f string, changed []string) []string {
	f = strings.ReplaceAll(f, "\\", "/")
	if strings.Contains(f, "/") {
		return []string{path.Clean(f)}
	}
	// Folders above changed files, deepest first, then by name.
	dirs := map[string]bool{}
	for _, c := range changed {
		for d := path.Dir(c); d != "." && d != "/" && d != ""; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	sorted := make([]string, 0, len(dirs))
	for d := range dirs {
		sorted = append(sorted, d)
	}
	slices.SortFunc(sorted, func(a, b string) int {
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return db - da
		}
		return strings.Compare(a, b)
	})
	out := make([]string, 0, len(sorted)+1)
	for _, d := range sorted {
		out = append(out, d+"/"+f)
	}
	return append(out, f)
}

// readParallel caps the reads in flight; each may be a git process.
const readParallel = 8

// readAll reads paths in parallel; the ones that fail are left out.
func readAll(ctx context.Context, read ReadFunc, paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, readParallel)
	for _, p := range paths {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			data, err := read(ctx, p)
			if err != nil {
				return
			}
			mu.Lock()
			out[p] = string(data)
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

// follow returns the file a pointer file points to, and its path: one that
// is only a path, as git show prints a symlink, or only an "@path" import,
// as a CLAUDE.md that defers to AGENTS.md often is. Anything else, or a
// pointer that can't be read, is returned as it is, with no path.
func follow(ctx context.Context, read ReadFunc, p, content string) (string, string) {
	target := strings.TrimSpace(content)
	if target == "" || strings.ContainsAny(target, " \t\n") {
		return content, ""
	}
	target = strings.TrimPrefix(target, "@")
	if !strings.HasSuffix(strings.ToLower(target), ".md") {
		return content, ""
	}
	resolved := path.Join(path.Dir(p), target)
	if ValidateFile(resolved) != nil || resolved == p {
		return content, ""
	}
	data, err := read(ctx, resolved)
	if err != nil {
		return content, ""
	}
	return string(data), resolved
}

// IsRulesFile reports whether the repository-relative path p is where Load
// looks for one of files (DefaultFiles if empty): a name with no slash in
// any folder, a path with one at the root only.
func IsRulesFile(p string, files []string) bool {
	if len(files) == 0 {
		files = DefaultFiles
	}
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	for _, f := range files {
		if ValidateFile(f) != nil {
			continue
		}
		f = strings.ReplaceAll(f, "\\", "/")
		if strings.Contains(f, "/") {
			if path.Clean(f) == p {
				return true
			}
		} else if path.Base(p) == f {
			return true
		}
	}
	return false
}

// cutAt returns the longest start of body within n bytes that ends at a
// paragraph, or failing that a line; "" if none fits.
func cutAt(body string, n int) string {
	if n <= 0 {
		return ""
	}
	head := body[:min(n, len(body))]
	if i := strings.LastIndex(head, "\n\n"); i >= 0 {
		return strings.TrimRight(head[:i+1], "\n") + "\n"
	}
	if i := strings.LastIndex(head, "\n"); i >= 0 {
		return head[:i+1]
	}
	return ""
}

// ChangedPaths returns the paths a unified git diff touches, old and new,
// in the order they appear.
func ChangedPaths(diff string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	inHeader := false
	for l := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			inHeader = true
		case strings.HasPrefix(l, "@@"):
			inHeader = false
		case !inHeader:
		case strings.HasPrefix(l, "--- "):
			add(diffPath(strings.TrimPrefix(l, "--- "), "a/"))
		case strings.HasPrefix(l, "+++ "):
			add(diffPath(strings.TrimPrefix(l, "+++ "), "b/"))
		case strings.HasPrefix(l, "rename from "):
			add(diffPath(strings.TrimPrefix(l, "rename from "), ""))
		case strings.HasPrefix(l, "rename to "):
			add(diffPath(strings.TrimPrefix(l, "rename to "), ""))
		}
	}
	return out
}

// diffPath reads a path from a diff header line, unquoting it and removing
// prefix; "" for /dev/null.
func diffPath(p, prefix string) string {
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
