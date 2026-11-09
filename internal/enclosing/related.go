package enclosing

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Repo is the version of a repository a review belongs to.
// git.ReviewDiff implements it.
type Repo interface {
	// NewSideFile reads a file as it is on the new side of the diff.
	NewSideFile(ctx context.Context, path string) ([]byte, error)
	// RepoFile reads a file the diff doesn't change.
	RepoFile(ctx context.Context, path string) ([]byte, error)
	// Grep returns the files that contain word as a whole word among the
	// files matching the glob patterns.
	Grep(ctx context.Context, word string, globs ...string) ([]string, error)
}

// Piece is a range of lines from one file, 1-based and inclusive.
type Piece struct {
	Path       string
	Start, End int
	// Label says why the piece is shown, e.g. "definition of Write".
	Label string
	Lines []string
}

// RelatedOptions limits and targets Related.
type RelatedOptions struct {
	// MaxLines caps the lines of all pieces together. Definitions come
	// first, then callers; a piece that doesn't fit is left out.
	MaxLines int
	// Focus, if set, targets these lines instead of the diff's changes:
	// definitions of what the declarations around them use, and callers of
	// those declarations. The check uses it for the lines its findings
	// point to.
	Focus map[string][]int
	// Skip lists files not to take pieces from, e.g. because the model is
	// already shown them whole.
	Skip map[string]bool
}

// Limits that keep the pieces relevant: name-based lookup can't tell
// same-named functions apart, so common names are left out rather than
// guessed.
const (
	// maxDefinitionLines is the longest definition shown whole; longer
	// ones are cut to their first maxDefinitionLines lines.
	maxDefinitionLines = 60
	// maxCallerLines is the longest calling declaration shown whole;
	// longer ones show callerWindow lines either side of the call.
	maxCallerLines = 40
	callerWindow   = 10
	// maxDefinitions is how many same-named declarations may be shown for
	// one call; with more, the call is too ambiguous to resolve by name.
	maxDefinitions = 2
	// maxCallSites skips a changed function with more call sites than
	// this; maxCallers is how many of the rest are shown.
	maxCallSites = 12
	maxCallers   = 4
)

// Related finds code the diff's changes depend on and code that depends on
// them: the definitions of the functions and types the added lines use,
// and the declarations that call the changed functions. Lookup is by name
// with tree-sitter and grep, without type information, so it can pick a
// same-named declaration from another type or package.
//
// Pieces from files the diff changes are read from the new side, so their
// line numbers match the diff's. Files that can't be read, and languages
// without a spec, are skipped.
func Related(ctx context.Context, diff string, repo Repo, opt RelatedOptions) []Piece {
	r := &related{ctx: ctx, repo: repo, diffFiles: changedFiles(diff), files: map[string]*parsedFile{}, greps: map[string][]string{}}

	targets := addedLines(diff)
	modified := ModifiedLinesFromDiff(diff)
	for p, ls := range modified {
		for _, l := range ls {
			if targets[p] == nil {
				targets[p] = map[int]bool{}
			}
			targets[p][l] = true
		}
	}
	shown := targets // lines the model sees in the diff
	if opt.Focus != nil {
		targets = map[string]map[int]bool{}
		for p, ls := range opt.Focus {
			targets[p] = map[int]bool{}
			for _, l := range ls {
				targets[p][l] = true
			}
		}
	}

	paths := make([]string, 0, len(targets))
	for p := range targets {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	have := map[string]bool{}
	skip := func(p string, start, end int) bool {
		if opt.Skip[p] || have[fmt.Sprintf("%s:%d", p, start)] {
			return true
		}
		for l := range shown[p] {
			if start <= l && l <= end {
				return true // the diff shows the changes in it
			}
		}
		return false
	}
	add := func(out *[]Piece, pc Piece) {
		have[fmt.Sprintf("%s:%d", pc.Path, pc.Start)] = true
		*out = append(*out, pc)
	}

	// Declarations around the targets: with Focus, every line of them is
	// searched for calls; otherwise only the targeted (added) lines are.
	var changed []decl
	seenChanged := map[string]bool{}
	type callsIn struct {
		f     *parsedFile
		calls []call
	}
	var uses []callsIn
	for _, p := range paths {
		f := r.file(p, true)
		if f == nil {
			continue
		}
		callLines := targets[p]
		if opt.Focus != nil {
			callLines = map[int]bool{}
		}
		for l := range targets[p] {
			d, ok := f.enclosing(l)
			if !ok {
				continue
			}
			if opt.Focus != nil {
				for n := d.start; n <= d.end; n++ {
					callLines[n] = true
				}
			}
			if d.callable && !seenChanged[p+":"+d.name] {
				seenChanged[p+":"+d.name] = true
				changed = append(changed, d)
			}
		}
		uses = append(uses, callsIn{f, f.calls(callLines)})
	}

	// Search for every name at once, then read the files found in parallel.
	words := map[string][]string{} // globs -> names
	globsOf := map[string][]string{}
	for _, u := range uses {
		key := strings.Join(u.f.spec.globs, "\x00")
		globsOf[key] = u.f.spec.globs
		for _, c := range u.calls {
			if !u.f.spec.builtins[c.name] {
				words[key] = append(words[key], c.name)
			}
		}
	}
	for _, d := range changed {
		key := strings.Join(r.files[d.path].spec.globs, "\x00")
		words[key] = append(words[key], d.name)
	}
	for key, ws := range words {
		r.grepAll(ws, globsOf[key])
	}
	r.prefetch()

	var defs, callers []Piece
	for _, u := range uses {
		for _, c := range u.calls {
			for _, d := range r.definitions(c, u.f) {
				if skip(d.path, d.start, d.end) {
					continue
				}
				pc := Piece{Path: d.path, Start: d.start, End: d.end, Label: "definition of " + d.name}
				if n := d.end - d.start + 1; n > maxDefinitionLines {
					pc.End = d.start + maxDefinitionLines - 1
					pc.Label += fmt.Sprintf(" (first %d of %d lines)", maxDefinitionLines, n)
				}
				add(&defs, pc)
			}
		}
	}

	sort.Slice(changed, func(i, j int) bool {
		if changed[i].path != changed[j].path {
			return changed[i].path < changed[j].path
		}
		return changed[i].start < changed[j].start
	})
	for _, d := range changed {
		for _, pc := range r.callers(d) {
			if !skip(pc.Path, pc.Start, pc.End) {
				add(&callers, pc)
			}
		}
	}

	var out []Piece
	budget := opt.MaxLines
	for _, pc := range append(defs, callers...) {
		n := pc.End - pc.Start + 1
		if n > budget {
			continue
		}
		budget -= n
		lines := r.files[pc.Path].lines
		pc.Lines = lines[pc.Start-1 : min(pc.End, len(lines))]
		out = append(out, pc)
	}
	return out
}

// FormatRelated renders pieces as a prompt section, with line numbers.
func FormatRelated(pieces []Piece) string {
	if len(pieces) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Related code\n\n")
	b.WriteString("Read-only context from elsewhere in the repository, with line numbers: definitions of functions and types the change uses, and code that calls the changed functions. It is not under review. Use it to check how the changed code is called and what the code it calls does; it was found by name, so a piece may belong to a different function of the same name.\n")
	for _, pc := range pieces {
		fmt.Fprintf(&b, "\n### %s:%d-%d (%s)\n\n", pc.Path, pc.Start, pc.End, pc.Label)
		for i, l := range pc.Lines {
			fmt.Fprintf(&b, "%d\t%s\n", pc.Start+i, l)
		}
	}
	return b.String()
}

type related struct {
	ctx       context.Context
	repo      Repo
	diffFiles map[string]bool
	files     map[string]*parsedFile
	// greps caches grep results by name and globs; a diff calls the same
	// names from many places, and each search is a git process.
	greps map[string][]string
}

// maxGrepFiles skips a name found in more files than this: too common to
// resolve by name, and each file has to be read and parsed.
const maxGrepFiles = 50

func (r *related) grep(word string, globs []string) []string {
	key := word + "\x00" + strings.Join(globs, "\x00")
	if paths, ok := r.greps[key]; ok {
		return paths
	}
	paths, err := r.repo.Grep(r.ctx, word, globs...)
	if err != nil || len(paths) > maxGrepFiles {
		paths = nil
	}
	r.greps[key] = paths
	return paths
}

// wordGrepper is a Repo that can search for many words in one go.
type wordGrepper interface {
	GrepWords(ctx context.Context, words []string, globs ...string) (map[string][]string, error)
}

// grepAll fills the grep cache for words in one search, if the repo can.
func (r *related) grepAll(words []string, globs []string) {
	g, ok := r.repo.(wordGrepper)
	if !ok {
		return
	}
	var todo []string
	seen := map[string]bool{}
	suffix := "\x00" + strings.Join(globs, "\x00")
	for _, w := range words {
		if _, cached := r.greps[w+suffix]; !cached && !seen[w] && isName(w) {
			seen[w] = true
			todo = append(todo, w)
		}
	}
	if len(todo) == 0 {
		return
	}
	found, err := g.GrepWords(r.ctx, todo, globs...)
	if err != nil {
		return // each name is searched on its own instead
	}
	for _, w := range todo {
		paths := found[w]
		if len(paths) > maxGrepFiles {
			paths = nil
		}
		slices.Sort(paths)
		r.greps[w+suffix] = paths
	}
}

// prefetch reads and parses, in parallel, every file the cached searches
// found that isn't read yet.
func (r *related) prefetch() {
	var todo []string
	seen := map[string]bool{}
	for _, paths := range r.greps {
		for _, p := range paths {
			if _, ok := r.files[p]; !ok && !seen[p] {
				seen[p] = true
				todo = append(todo, p)
			}
		}
	}
	parsed := make([]*parsedFile, len(todo))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, p := range todo {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			parsed[i] = r.load(p)
		})
	}
	wg.Wait()
	for i, p := range todo {
		r.files[p] = parsed[i]
	}
}

// file reads and parses p, or returns nil. Files the diff changes are read
// from its new side.
func (r *related) file(p string, inDiff bool) *parsedFile {
	if f, ok := r.files[p]; ok {
		return f
	}
	if inDiff {
		r.diffFiles[p] = true
	}
	f := r.load(p)
	r.files[p] = f
	return f
}

// load reads and parses p without touching the cache, so it can run in
// parallel.
func (r *related) load(p string) *parsedFile {
	read := r.repo.RepoFile
	if r.diffFiles[p] {
		read = r.repo.NewSideFile
	}
	src, err := read(r.ctx, p)
	if err != nil || strings.IndexByte(string(src), 0) >= 0 {
		return nil
	}
	return parse(p, src)
}

// definitions finds the declarations c may refer to. A qualifier that
// names a package directory, or a call from the same directory, picks
// among same-named declarations; a name with too many candidates is
// skipped.
func (r *related) definitions(c call, from *parsedFile) []decl {
	if from.spec.builtins[c.name] {
		return nil
	}
	var cands []decl
	paths := r.grep(c.name, from.spec.globs)
	for _, p := range paths {
		if isTestFile(p) {
			continue
		}
		f := r.file(p, false)
		if f == nil {
			continue
		}
		for _, d := range f.decls {
			if d.name == c.name {
				cands = append(cands, d)
			}
		}
	}
	if len(cands) <= 1 {
		return cands
	}
	var picked []decl
	for _, d := range cands {
		dir := path.Dir(d.path)
		if (c.qualifier != "" && path.Base(dir) == c.qualifier) || (c.qualifier == "" && dir == path.Dir(from.path)) {
			picked = append(picked, d)
		}
	}
	switch {
	case len(picked) > 0 && len(picked) <= maxDefinitions:
		return picked
	case len(cands) <= maxDefinitions:
		return cands
	}
	return nil
}

// callers returns the declarations that call d, production code first;
// test code only when nothing else calls it.
func (r *related) callers(d decl) []Piece {
	from := r.files[d.path]
	paths := r.grep(d.name, from.spec.globs)
	callRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(d.name) + `\s*(\[[^\]]*\]\s*)?\(`)
	var prod, test []Piece
	sites := 0
	for _, p := range paths {
		f := r.file(p, false)
		if f == nil {
			continue
		}
		seen := map[int]bool{}
		for i, line := range f.lines {
			n := i + 1
			if !callRe.MatchString(line) || (p == d.path && d.start <= n && n <= d.end) {
				continue
			}
			enc, ok := f.enclosing(n)
			if !ok || (enc.name == d.name && enc.start == n) || seen[enc.start] {
				continue
			}
			seen[enc.start] = true
			sites++
			pc := Piece{Path: p, Start: enc.start, End: enc.end, Label: fmt.Sprintf("%s calls %s", enc.name, d.name)}
			if enc.end-enc.start+1 > maxCallerLines {
				pc.Start, pc.End = max(enc.start, n-callerWindow), min(enc.end, n+callerWindow)
				pc.Label += fmt.Sprintf("; lines around the call, %s is lines %d-%d", enc.name, enc.start, enc.end)
			}
			if isTestFile(p) {
				test = append(test, pc)
			} else {
				prod = append(prod, pc)
			}
		}
	}
	if sites > maxCallSites {
		return nil
	}
	if len(prod) == 0 && len(test) > 0 {
		return test[:1]
	}
	return prod[:min(len(prod), maxCallers)]
}

func isTestFile(p string) bool {
	base := path.Base(p)
	return strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(strings.TrimSuffix(base, path.Ext(base)), "_test") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.Contains("/"+p, "/__tests__/")
}

// changedFiles returns the new-side paths the diff touches.
func changedFiles(diff string) map[string]bool {
	out := map[string]bool{}
	for l := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(l, "+++ ") {
			if p := newFilePath(l); p != "" {
				out[p] = true
			}
		}
	}
	return out
}

// addedLines returns the new-side numbers of the added lines per file,
// including files the diff adds.
func addedLines(diff string) map[string]map[int]bool {
	out := map[string]map[int]bool{}
	var file string
	line := -1
	for l := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			file, line = "", -1
		case line == -1 && strings.HasPrefix(l, "+++ "):
			file = newFilePath(l)
		case strings.HasPrefix(l, "@@ "):
			line = -1
			if parts := strings.Split(l, " "); len(parts) >= 3 {
				start, _, _ := strings.Cut(strings.TrimPrefix(parts[2], "+"), ",")
				var n int
				if _, err := fmt.Sscan(start, &n); err == nil {
					line = n - 1
				}
			}
		case file == "" || line < 0:
		case strings.HasPrefix(l, "+"):
			line++
			if out[file] == nil {
				out[file] = map[int]bool{}
			}
			out[file][line] = true
		case strings.HasPrefix(l, " "):
			line++
		}
	}
	return out
}

// spec describes what Related looks for in one language.
type spec struct {
	// globs are the files a name is searched in.
	globs []string
	// decls maps a declaration node type to how to read it.
	decls map[string]declKind
	// calls maps a node that uses a function or type to the field holding
	// what it uses.
	calls map[string]string
	// builtins are names never looked up.
	builtins map[string]bool
}

type declKind struct {
	callable bool
	// outer, if set, is a parent node type that should be shown instead
	// (Go's "type" keyword, a JavaScript const declaration).
	outer string
}

// nameFields are the fields that hold the name in a qualified reference
// (pkg.Func, obj.method, mod::func), and qualifierFields what comes before
// it.
var (
	nameFields      = []string{"field", "property", "attribute", "name"}
	qualifierFields = []string{"operand", "object", "package", "path", "value"}
)

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

var jsSpec = spec{
	globs: []string{"*.ts", "*.tsx", "*.js", "*.jsx", "*.mjs", "*.cjs"},
	decls: map[string]declKind{
		"function_declaration":   {callable: true},
		"method_definition":      {callable: true},
		"variable_declarator":    {callable: true, outer: "lexical_declaration"},
		"class_declaration":      {},
		"interface_declaration":  {},
		"type_alias_declaration": {},
		"enum_declaration":       {},
	},
	calls:    map[string]string{"call_expression": "function", "new_expression": "constructor"},
	builtins: set("require", "log", "push", "map", "filter", "then", "catch", "toString", "parseInt", "String", "Number", "Error", "Promise", "Array", "Object", "Map", "Set"),
}

var specs = map[string]spec{
	"go": {
		globs: []string{"*.go"},
		decls: map[string]declKind{
			"function_declaration": {callable: true},
			"method_declaration":   {callable: true},
			"type_spec":            {outer: "type_declaration"},
		},
		calls: map[string]string{"call_expression": "function", "composite_literal": "type"},
		builtins: set("append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make",
			"max", "min", "new", "panic", "print", "println", "real", "recover",
			"bool", "byte", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64",
			"rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "any",
			"Error", "String"),
	},
	"python": {
		globs: []string{"*.py"},
		decls: map[string]declKind{
			"function_definition": {callable: true},
			"class_definition":    {},
		},
		calls:    map[string]string{"call": "function"},
		builtins: set("print", "len", "str", "int", "float", "list", "dict", "set", "tuple", "range", "isinstance", "super", "open", "append", "format", "get", "join"),
	},
	"rust": {
		globs: []string{"*.rs"},
		decls: map[string]declKind{
			"function_item": {callable: true},
			"struct_item":   {},
			"enum_item":     {},
			"trait_item":    {},
			"type_item":     {},
		},
		calls:    map[string]string{"call_expression": "function", "struct_expression": "name"},
		builtins: set("new", "clone", "unwrap", "expect", "into", "from", "to_string", "iter", "map", "collect", "Some", "Ok", "Err", "Box", "Vec", "String"),
	},
	"typescript": jsSpec,
	"tsx":        jsSpec,
	"javascript": jsSpec,
}

type decl struct {
	path       string
	name       string
	callable   bool
	start, end int // 1-based, inclusive
}

type call struct {
	qualifier, name string
}

type parsedFile struct {
	path  string
	spec  spec
	lines []string
	decls []decl
	// uses are the calls and type uses in the file, by line.
	uses map[int][]call
}

func parse(p string, src []byte) *parsedFile {
	entry := grammars.DetectLanguage(p)
	if entry == nil {
		return nil
	}
	sp, ok := specs[entry.Name]
	lang := entry.Language()
	if !ok || lang == nil {
		return nil
	}
	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil || tree.RootNode() == nil {
		return nil
	}

	f := &parsedFile{path: p, spec: sp, lines: strings.Split(strings.TrimSuffix(string(src), "\n"), "\n"), uses: map[int][]call{}}
	var visit func(n *gotreesitter.Node)
	visit = func(n *gotreesitter.Node) {
		t := n.Type(lang)
		if k, ok := sp.decls[t]; ok {
			if d, ok := readDecl(n, k, t, lang, src); ok {
				d.path = p
				f.decls = append(f.decls, d)
			}
		}
		if field, ok := sp.calls[t]; ok {
			if c, ok := readCall(n.ChildByFieldName(field, lang), lang, src); ok {
				row := int(n.StartPoint().Row) + 1
				f.uses[row] = append(f.uses[row], c)
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			visit(n.Child(i))
		}
	}
	visit(tree.RootNode())
	return f
}

func readDecl(n *gotreesitter.Node, k declKind, t string, lang *gotreesitter.Language, src []byte) (decl, bool) {
	name := n.ChildByFieldName("name", lang)
	if name == nil {
		return decl{}, false
	}
	if t == "variable_declarator" {
		// Only functions assigned to a name: const f = () => {...}.
		v := n.ChildByFieldName("value", lang)
		if v == nil || (v.Type(lang) != "arrow_function" && v.Type(lang) != "function_expression" && v.Type(lang) != "function") {
			return decl{}, false
		}
	}
	shown := n
	if p := n.Parent(); k.outer != "" && p != nil && p.Type(lang) == k.outer && p.NamedChildCount() == 1 {
		shown = p
	}
	return decl{
		name:     name.Text(src),
		callable: k.callable,
		start:    int(shown.StartPoint().Row) + 1,
		end:      int(shown.EndPoint().Row) + 1,
	}, true
}

// readCall reads the name a call or type use refers to, and what qualifies
// it (a package, receiver or module), if anything.
func readCall(n *gotreesitter.Node, lang *gotreesitter.Language, src []byte) (call, bool) {
	if n == nil {
		return call{}, false
	}
	if n.NamedChildCount() == 0 {
		return call{name: n.Text(src)}, isName(n.Text(src))
	}
	for _, nf := range nameFields {
		name := n.ChildByFieldName(nf, lang)
		if name == nil {
			continue
		}
		c := call{name: name.Text(src)}
		for _, qf := range qualifierFields {
			if q := n.ChildByFieldName(qf, lang); q != nil {
				c.qualifier = q.Text(src)
				break
			}
		}
		return c, isName(c.name)
	}
	return call{}, false
}

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]+$`)

// isName reports whether s is an identifier worth looking up: at least two
// characters, so a one-letter variable isn't searched for.
func isName(s string) bool { return nameRe.MatchString(s) }

// enclosing returns the innermost declaration around line.
func (f *parsedFile) enclosing(line int) (decl, bool) {
	var best decl
	found := false
	for _, d := range f.decls {
		if d.start <= line && line <= d.end && (!found || d.end-d.start < best.end-best.start) {
			best, found = d, true
		}
	}
	return best, found
}

// calls returns the calls on the given lines, in order, without repeats.
func (f *parsedFile) calls(lines map[int]bool) []call {
	rows := make([]int, 0, len(lines))
	for l := range lines {
		rows = append(rows, l)
	}
	sort.Ints(rows)
	var out []call
	seen := map[call]bool{}
	for _, l := range rows {
		for _, c := range f.uses[l] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}
