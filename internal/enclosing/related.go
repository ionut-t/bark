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
// them: the definitions of the types the added lines use (in annotations,
// through the parameters and fields they read, and one step further,
// through return types and bases), of the functions they call, and the
// declarations that call the changed functions. Lookup is by name with
// tree-sitter and grep, without type information, so it can pick a
// same-named declaration from another type or package; TypeScript imports
// are followed to the file they name (see tsmodules.go).
//
// Pieces from files the diff changes are read from the new side, so their
// line numbers match the diff's. Files that can't be read, and languages
// without a spec, are skipped.
func Related(ctx context.Context, diff string, repo Repo, opt RelatedOptions) []Piece {
	r := &related{
		ctx: ctx, repo: repo, diffFiles: changedFiles(diff), files: map[string]*parsedFile{}, greps: map[string][]string{}, modules: map[string]string{},
		imported: map[string][]decl{}, modulePaths: map[string]string{}, tsconfigs: map[string]tsPaths{},
		ignored: func(string) bool { return false },
	}
	if ic, ok := repo.(ignoreChecker); ok {
		r.ignored = ic.IgnoredFiles(ctx)
	}

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
	// searched for calls and types; otherwise only the targeted (added)
	// lines are.
	var changed []decl
	seenChanged := map[string]bool{}
	var types, calls []ref
	var pending []selfUse // fields read through a Go or Rust receiver
	accessed := map[string]bool{}
	for _, p := range paths {
		f := r.file(p, true)
		if f == nil {
			continue
		}
		lines := sortedLines(targets[p])
		callLines := targets[p]
		if opt.Focus != nil {
			callLines = map[int]bool{}
		}
		for _, l := range lines {
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
		// The types of the parameters the lines use.
		var paramTypes []ref
		seenParam := map[string]bool{}
		for _, l := range sortedLines(callLines) {
			fn, ok := f.enclosingCallable(l)
			for _, id := range f.idents[l] {
				if key := fmt.Sprintf("%d:%s", fn.start, id); ok && !seenParam[key] && fn.params[id] != nil {
					seenParam[key] = true
					for _, c := range fn.params[id] {
						paramTypes = append(paramTypes, ref{c, f})
					}
				}
			}
		}
		for _, l := range sortedLines(callLines) {
			for _, c := range f.types[l] {
				types = append(types, ref{c, f})
			}
			types = append(types, r.importedClasses(f, l)...)
			for _, c := range f.uses[l] {
				calls = append(calls, ref{c, f})
				if t, ok := typeQualifier(c); ok {
					types = append(types, ref{t, f})
				}
			}
			for _, name := range f.props[l] {
				accessed[name] = true
			}
			for _, field := range f.selfs[l] {
				if cls, ok := f.enclosingType(l); ok {
					for _, m := range cls.members[field] {
						for _, c := range m.types {
							types = append(types, ref{c, f})
						}
					}
				} else if fn, ok := f.enclosingCallable(l); ok && fn.owner != "" {
					types = append(types, ref{call{name: fn.owner, kind: refType}, f})
					pending = append(pending, selfUse{f, fn.owner, field})
				}
			}
		}
		types = append(types, paramTypes...)
	}

	// What the targets use, searched for at once, then what the
	// definitions found use in their signatures, one step further.
	first := append(slices.Clone(types), calls...)
	found := r.lookupAll(first, changed)
	var typeDefs, callDefs, deeper []Piece
	var next []ref
	var nextIsType []bool
	var defs []decl
	for _, ds := range found {
		for _, d := range ds {
			defs = append(defs, d)
			for _, c := range d.sig {
				next = append(next, ref{c, r.files[d.path]})
				nextIsType = append(nextIsType, false)
			}
		}
	}
	for _, u := range pending {
		for _, cls := range u.owners(defs) {
			for _, m := range cls.members[u.field] {
				for _, c := range m.types {
					// Qualified by the imports of the type's file.
					next = append(next, ref{c, r.files[cls.path]})
					nextIsType = append(nextIsType, true)
				}
			}
		}
	}
	addDefs := func(out *[]Piece, ds []decl) {
		for _, d := range ds {
			if skip(d.path, d.start, d.end) {
				continue
			}
			for _, pc := range definitionPieces(d, accessed) {
				if !skip(pc.Path, pc.Start, pc.End) {
					add(out, pc)
				}
			}
		}
	}
	for i, ds := range found {
		if i < len(types) {
			addDefs(&typeDefs, ds)
		} else {
			addDefs(&callDefs, ds)
		}
	}
	for i, ds := range r.lookupAll(next, nil) {
		if nextIsType[i] {
			addDefs(&typeDefs, ds)
		} else {
			addDefs(&deeper, ds)
		}
	}

	var callers []Piece
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

	// Types the change uses first, then the functions it calls, the types
	// those use, and the code that calls the change.
	var out []Piece
	budget := opt.MaxLines
	for _, pc := range slices.Concat(typeDefs, callDefs, deeper, callers) {
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

// ref is a name a file uses.
type ref struct {
	c    call
	from *parsedFile
}

// selfUse is a field a method reads through its Go receiver or Rust self.
type selfUse struct {
	f            *parsedFile
	owner, field string
}

// owners returns the declarations of the type u's method belongs to: in
// its own file, or among the definitions found.
func (u selfUse) owners(found []decl) []decl {
	var out []decl
	seen := map[string]bool{}
	for _, d := range append(slices.Clone(u.f.decls), found...) {
		if key := fmt.Sprintf("%s:%d", d.path, d.start); d.isType && d.name == u.owner && !seen[key] {
			seen[key] = true
			out = append(out, d)
		}
	}
	return out
}

// definitionPieces returns the pieces showing d: all of it, or, when it is
// longer than maxDefinitionLines, the members of it the change uses (the
// field it reads in a long interface, and the field's validators), or else
// its first maxDefinitionLines lines.
func definitionPieces(d decl, accessed map[string]bool) []Piece {
	pc := Piece{Path: d.path, Start: d.start, End: d.end, Label: "definition of " + d.name}
	n := d.end - d.start + 1
	if n <= maxDefinitionLines {
		return []Piece{pc}
	}
	var out []Piece
	for name, ms := range d.members {
		if !accessed[name] {
			continue
		}
		for _, m := range ms {
			out = append(out, Piece{Path: d.path, Start: m.start, End: min(m.end, m.start+maxDefinitionLines-1), Label: fmt.Sprintf("%s in %s, lines %d-%d", name, d.name, d.start, d.end)})
		}
	}
	if len(out) > 0 {
		sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
		return out
	}
	pc.End = d.start + maxDefinitionLines - 1
	pc.Label += fmt.Sprintf(" (first %d of %d lines)", maxDefinitionLines, n)
	return []Piece{pc}
}

// lookupAll finds the declarations each ref may refer to. Names not found
// through imports are searched for at once, with the names of the changed
// declarations, whose callers are searched for next.
func (r *related) lookupAll(refs []ref, changed []decl) [][]decl {
	words := map[string][]string{} // globs -> names
	globsOf := map[string][]string{}
	want := func(name string, sp spec) {
		key := strings.Join(sp.globs, "\x00")
		globsOf[key] = sp.globs
		words[key] = append(words[key], name)
	}
	skip := func(rf ref) bool {
		return rf.from == nil || rf.from.spec.builtins[rf.c.name] || !isName(rf.from.unalias(rf.c).name)
	}
	for _, rf := range refs {
		if skip(rf) {
			continue
		}
		if _, ok := r.resolveImport(rf.c, rf.from); !ok {
			want(rf.from.unalias(rf.c).name, rf.from.spec)
		}
	}
	for _, d := range changed {
		want(d.name, r.files[d.path].spec)
	}
	for key, ws := range words {
		r.grepAll(ws, globsOf[key])
	}
	r.prefetch()

	out := make([][]decl, len(refs))
	for i, rf := range refs {
		if skip(rf) {
			continue
		}
		if ds, ok := r.resolveImport(rf.c, rf.from); ok {
			out[i] = ds
		} else {
			out[i] = r.definitions(rf.from.unalias(rf.c), rf.from)
		}
	}
	return out
}

func sortedLines(lines map[int]bool) []int {
	out := make([]int, 0, len(lines))
	for l := range lines {
		out = append(out, l)
	}
	sort.Ints(out)
	return out
}

// FormatRelated renders pieces as a prompt section, with line numbers.
func FormatRelated(pieces []Piece) string {
	if len(pieces) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Related code\n\n")
	b.WriteString("Read-only context from elsewhere in the repository, with line numbers: definitions of the types and functions the change uses (for a long type, the members it uses), and code that calls the changed functions. It is not under review. Use it to check how the changed code is called, what the code it calls does, and what the types it uses allow, such as whether a field may be missing or null; it was found by name, so a piece may belong to a different declaration of the same name, and a type defined outside the repository isn't shown.\n")
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
	// modules caches go.mod module paths by directory (see goModule).
	modules map[string]string
	// imported caches resolveImport by file and reference (TypeScript and
	// Python), modulePaths resolveModule by directory and module (and
	// resolvePyModule, under keys that start with "py\x00"), and tsconfigs
	// the module resolution by directory.
	imported    map[string][]decl
	modulePaths map[string]string
	tsconfigs   map[string]tsPaths
	// ignored reports whether .barkignore leaves a file out; see
	// ignoreChecker.
	ignored func(path string) bool
}

// maxGrepFiles skips a name found in more files than this: too common to
// resolve by name, and each file has to be read and parsed. Files
// .barkignore leaves out are counted apart: more of them than this are
// dropped, and the others still searched.
const maxGrepFiles = 50

// capGrep applies maxGrepFiles to the files a search found.
func (r *related) capGrep(paths []string) []string {
	ignored := 0
	for _, p := range paths {
		if r.ignored(p) {
			ignored++
		}
	}
	switch {
	case len(paths)-ignored > maxGrepFiles:
		return nil
	case ignored > maxGrepFiles:
		return slices.DeleteFunc(paths, r.ignored)
	}
	return paths
}

func (r *related) grep(word string, globs []string) []string {
	key := word + "\x00" + strings.Join(globs, "\x00")
	if paths, ok := r.greps[key]; ok {
		return paths
	}
	paths, err := r.repo.Grep(r.ctx, word, globs...)
	if err != nil {
		paths = nil
	}
	paths = r.capGrep(paths)
	r.greps[key] = paths
	return paths
}

// wordGrepper is a Repo that can search for many words in one go.
type wordGrepper interface {
	GrepWords(ctx context.Context, words []string, globs ...string) (map[string][]string, error)
}

// ignoreChecker is a Repo that knows which files .barkignore leaves out of
// the diff, such as generated or vendored code. git.ReviewDiff implements it.
type ignoreChecker interface {
	IgnoredFiles(ctx context.Context) func(path string) bool
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
		paths := r.capGrep(found[w])
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

// read reads a file that isn't parsed, such as a tsconfig.json, from the
// side of the diff its line numbers would match.
func (r *related) read(p string) ([]byte, error) {
	if r.diffFiles[p] {
		return r.repo.NewSideFile(r.ctx, p)
	}
	return r.repo.RepoFile(r.ctx, p)
}

// definitions finds the declarations c may refer to, types only for a
// type. A Go qualifier naming an import keeps only declarations in the
// imported package, so a call on a package outside the repository finds
// none. A qualifier that names a package directory, or a reference from
// the same directory, picks among same-named declarations; a name with
// too many candidates is skipped,
// unless leaving out files .barkignore leaves out makes it few enough.
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
			if d.name == c.name && (c.kind != refType || d.isType) {
				cands = append(cands, d)
			}
		}
	}
	// A Go call qualified by an imported package (time.Since) refers to
	// that package only: outside the repository, nothing here is it.
	if imp, ok := from.imports[c.qualifier]; ok && c.qualifier != "" {
		cands = slices.DeleteFunc(cands, func(d decl) bool {
			return !r.importsDir(imp, path.Dir(d.path))
		})
	}

	if ds, ok := r.pick(c, from, cands); ok {
		return ds
	}

	// Still ambiguous: leave out files .barkignore leaves out, such as
	// generated code, and try again. They count only when nothing else
	// declares the name, or a qualifier or the directory picks them.
	kept := slices.DeleteFunc(slices.Clone(cands), func(d decl) bool {
		return r.ignored(d.path)
	})
	if len(kept) > 0 && len(kept) < len(cands) {
		if ds, ok := r.pick(c, from, kept); ok {
			return ds
		}
	}
	return nil
}

// pick chooses among same-named candidates for c: those a qualifier or
// from's directory picks, or all of them, if few enough. It reports false
// when too many are left.
func (r *related) pick(c call, from *parsedFile, cands []decl) ([]decl, bool) {
	if len(cands) <= 1 {
		return cands, true
	}
	var picked []decl
	for _, d := range cands {
		dir := path.Dir(d.path)
		imp, imported := from.imports[c.qualifier]
		switch {
		case c.qualifier != "" && imported:
			if r.importsDir(imp, dir) {
				picked = append(picked, d)
			}
		case c.qualifier != "":
			if path.Base(dir) == c.qualifier {
				picked = append(picked, d)
			}
		case dir == path.Dir(from.path):
			picked = append(picked, d)
		}
	}
	switch {
	case len(picked) > 0 && len(picked) <= maxDefinitions:
		return picked, true
	case len(cands) <= maxDefinitions:
		return cands, true
	}
	return nil, false
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
			if !callRe.MatchString(line) || f.signatures[n] || (p == d.path && d.start <= n && n <= d.end) ||
				(d.packageScoped && !r.callsPackageFunc(f, line, callRe, d, from)) {
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

// qualifierRe matches the name before the dot of a qualified call, and
// whether a dot comes before that name too.
var qualifierRe = regexp.MustCompile(`(\.\s*)?\b([A-Za-z_][A-Za-z0-9_]*)\s*\.\s*$`)

// callsPackageFunc reports whether line, in f, may call the package-scoped
// function d, declared in from: by bare name from d's own directory, or
// through d's package name or an alias that imports d's directory. Calls
// of a same-named method (err.ExitCode(), wait().Start()) are left out.
func (r *related) callsPackageFunc(f *parsedFile, line string, callRe *regexp.Regexp, d decl, from *parsedFile) bool {
	dir := path.Dir(d.path)
	importsDir := func(imp string) bool { return r.importsDir(imp, dir) }
	importsAny := func(imps []string) bool { return slices.ContainsFunc(imps, importsDir) }
	for _, m := range callRe.FindAllStringIndex(line, -1) {
		before := strings.TrimRight(line[:m[0]], " \t")
		if !strings.HasSuffix(before, ".") {
			if path.Dir(f.path) == dir || importsAny(f.dotImports) {
				return true
			}
			continue
		}
		q := qualifierRe.FindStringSubmatch(before)
		if q == nil || q[1] != "" {
			continue // a call's result or a field: a method
		}
		if imp, ok := f.imports[q[2]]; ok {
			if importsDir(imp) {
				return true
			}
		} else if q[2] == from.pkg && importsAny(f.plainImports) {
			// A package named unlike its directory, imported without an
			// alias. Without the import, q is a variable named like it.
			return true
		}
	}
	return false
}

// importsDir reports whether the import path imp names the repository
// directory dir: the module path of the nearest go.mod plus dir's path
// below it. Without a go.mod, a path ending in dir matches, and nothing
// matches the root directory: only a module root can be imported there.
func (r *related) importsDir(imp, dir string) bool {
	switch want := r.importPath(dir); {
	case imp == "":
		return false
	case want != "":
		return imp == want
	case dir == ".":
		return false
	}
	return strings.HasSuffix(imp, "/"+dir)
}

var moduleRe = regexp.MustCompile(`(?m)^module\s+"?([^\s"]+)"?`)

// importPath returns the import path of the package in dir, or "" if no
// go.mod is found in it or above it.
func (r *related) importPath(dir string) string {
	for d := dir; ; d = path.Dir(d) {
		if mod := r.goModule(d); mod != "" {
			rel := strings.TrimPrefix(strings.TrimPrefix(dir, d), "/")
			if d == "." {
				rel = dir
			}
			return path.Join(mod, rel)
		}
		if d == "." {
			return ""
		}
	}
}

// goModule returns the module path in dir's go.mod, or "".
func (r *related) goModule(dir string) string {
	if mod, ok := r.modules[dir]; ok {
		return mod
	}
	mod := ""
	if src, err := r.read(path.Join(dir, "go.mod")); err == nil {
		if m := moduleRe.FindSubmatch(src); m != nil {
			mod = string(m[1])
		}
	}
	r.modules[dir] = mod
	return mod
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
	// family groups languages that share a grammar's node types (see
	// types.go).
	family string
	// globs are the files a name is searched in.
	globs []string
	// decls maps a declaration node type to how to read it.
	decls map[string]declKind
	// calls maps a node that uses a function or type to the field holding
	// what it uses.
	calls map[string]string
	// builtins are names never looked up.
	builtins map[string]bool
	// signatures are the nodes that declare a method without a body (an
	// interface or trait method); their lines look like calls but aren't.
	signatures map[string]bool
}

type declKind struct {
	callable bool
	// isType is a type declaration: a class, interface, struct or alias.
	isType bool
	// packageScoped is a function called by bare name only from its own
	// package, and through the package's name or an import alias from
	// others (a Go function without a receiver).
	packageScoped bool
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
	family: famJS,
	globs:  []string{"*.ts", "*.tsx", "*.js", "*.jsx", "*.mjs", "*.cjs"},
	decls: map[string]declKind{
		"function_declaration":       {callable: true},
		"method_definition":          {callable: true},
		"variable_declarator":        {callable: true, outer: "lexical_declaration"},
		"class_declaration":          {isType: true},
		"abstract_class_declaration": {isType: true},
		"interface_declaration":      {isType: true},
		"type_alias_declaration":     {isType: true},
		"enum_declaration":           {isType: true},
	},
	calls:      map[string]string{"call_expression": "function", "new_expression": "constructor"},
	signatures: set("method_signature", "abstract_method_signature"),
	builtins: set("require", "log", "push", "map", "filter", "then", "catch", "toString", "parseInt", "String", "Number", "Error", "Promise", "Array", "Object", "Map", "Set",
		// Types from the language, the DOM, RxJS and Angular.
		"Record", "Partial", "Required", "Readonly", "ReadonlyArray", "Pick", "Omit", "Exclude", "Extract", "NonNullable",
		"ReturnType", "Parameters", "InstanceType", "Awaited", "PromiseLike", "Iterable", "AsyncIterable", "Date", "RegExp",
		"Function", "Symbol", "Uint8Array", "WeakMap", "WeakSet", "HTMLElement", "Element", "Event", "Window", "Document",
		"Observable", "Subject", "BehaviorSubject", "ReplaySubject", "Subscription", "EventEmitter",
		"Signal", "WritableSignal", "InputSignal", "ModelSignal", "OutputEmitterRef", "ElementRef", "TemplateRef", "Injectable", "Component", "Input", "Output", "inject", "input", "output", "signal", "computed", "effect"),
}

var specs = map[string]spec{
	"go": {
		family: famGo,
		globs:  []string{"*.go"},
		decls: map[string]declKind{
			"function_declaration": {callable: true, packageScoped: true},
			"method_declaration":   {callable: true},
			"type_spec":            {isType: true, outer: "type_declaration"},
			"type_alias":           {isType: true, outer: "type_declaration"},
		},
		calls:      map[string]string{"call_expression": "function", "composite_literal": "type"},
		signatures: set("method_elem", "method_spec"),
		builtins: set("append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make",
			"max", "min", "new", "panic", "print", "println", "real", "recover",
			"bool", "byte", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64",
			"rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "any",
			"Error", "String"),
	},
	"python": {
		family: famPython,
		globs:  []string{"*.py"},
		decls: map[string]declKind{
			"function_definition": {callable: true},
			"class_definition":    {isType: true},
		},
		calls: map[string]string{"call": "function"},
		builtins: set("print", "len", "str", "int", "float", "list", "dict", "set", "tuple", "range", "isinstance", "super", "open", "append", "format", "get", "join",
			"bool", "bytes", "object", "type", "None", "self", "cls", "Exception", "ValueError", "TypeError", "KeyError",
			// typing, pydantic, dataclasses and SQLAlchemy.
			"Any", "Optional", "Union", "List", "Dict", "Set", "Tuple", "Type", "Literal", "Annotated", "Callable", "ClassVar",
			"Final", "Iterable", "Iterator", "Sequence", "Mapping", "Generator", "Awaitable", "Coroutine", "Self", "TypeVar",
			"Generic", "Protocol", "TypedDict", "NamedTuple", "BaseModel", "Field", "ConfigDict", "field_validator",
			"model_validator", "validator", "root_validator", "computed_field", "dataclass", "field", "Mapped", "mapped_column",
			"Enum", "classmethod", "staticmethod", "property", "datetime", "date", "Decimal", "UUID"),
	},
	"rust": {
		family: famRust,
		globs:  []string{"*.rs"},
		decls: map[string]declKind{
			"function_item": {callable: true},
			"struct_item":   {isType: true},
			"enum_item":     {isType: true},
			"trait_item":    {isType: true},
			"type_item":     {isType: true},
		},
		calls:      map[string]string{"call_expression": "function", "struct_expression": "name"},
		signatures: set("function_signature_item"),
		builtins: set("new", "clone", "unwrap", "expect", "into", "from", "to_string", "iter", "map", "collect", "Some", "Ok", "Err", "Box", "Vec", "String",
			"Option", "Result", "Self", "HashMap", "HashSet", "BTreeMap", "BTreeSet", "VecDeque", "Arc", "Rc", "RefCell", "Cell",
			"Mutex", "RwLock", "Cow", "PhantomData", "Default", "Debug", "Clone", "Display"),
	},
	"typescript": jsSpec,
	"tsx":        jsSpec,
	"javascript": jsSpec,
}

type decl struct {
	path          string
	name          string
	callable      bool
	packageScoped bool
	isType        bool
	// topLevel is declared at the top of its file, so it can be exported.
	topLevel   bool
	start, end int // 1-based, inclusive
	// sig are the types a use of the declaration yields (see signature),
	// and params a function's parameter types by name.
	sig    []call
	params map[string][]call
	// members are a type's fields and methods (see members).
	members map[string][]member
	// owner is the type a Go method's receiver or a Rust impl names, and
	// self the name the method reads it through.
	owner, self string
}

// call is a reference to a function, type or value: a call, unless kind
// says otherwise.
type call struct {
	qualifier, name string
	kind            refKind
}

type parsedFile struct {
	path  string
	spec  spec
	lines []string
	decls []decl
	// uses are the calls and composite literals in the file, and types the
	// type references, by line.
	uses, types map[int][]call
	// props are the names of the members the file reads (x.name), selfs
	// those it reads through this, self or a Go receiver, and idents the
	// identifiers, by line.
	props, selfs, idents map[int][]string
	// aliases maps names imported under an alias to the names they import:
	// P to Project for import { Project as P }.
	aliases map[string]string
	// ts is what a TypeScript or JavaScript file imports and exports.
	ts *tsModule
	// py is what a Python file imports.
	py *pyModule
	// signatures are the lines that start a method signature without a
	// body.
	signatures map[int]bool
	// pkg is the file's Go package name, and imports its Go import paths
	// by alias.
	pkg     string
	imports map[string]string
	// plainImports are the Go import paths without an alias, and
	// dotImports those imported with ".".
	plainImports, dotImports []string
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

	f := &parsedFile{
		path: p, spec: sp, lines: strings.Split(strings.TrimSuffix(string(src), "\n"), "\n"),
		uses: map[int][]call{}, types: map[int][]call{}, props: map[int][]string{}, selfs: map[int][]string{}, idents: map[int][]string{},
		signatures: map[int]bool{}, imports: map[string]string{}, aliases: map[string]string{},
	}
	if sp.family == famJS {
		f.ts = &tsModule{imports: map[string]tsImport{}, localExports: map[string]string{}}
	}
	if sp.family == famPython {
		f.py = newPyModule()
	}
	// recv is the receiver of the Go method being visited.
	var visit func(n *gotreesitter.Node, recv string)
	visit = func(n *gotreesitter.Node, recv string) {
		t := n.Type(lang)
		row := int(n.StartPoint().Row) + 1
		if k, ok := sp.decls[t]; ok {
			if d, ok := readDecl(n, k, t, lang, src); ok {
				d.path = p
				d.sig = signature(sp.family, n, t, lang, src)
				if d.isType {
					d.members = members(sp.family, n, t, lang, src)
				}
				if d.callable {
					d.params = params(sp.family, n, lang, src)
					d.owner, d.self = owner(sp.family, n, t, lang, src)
					if sp.family == famGo {
						recv = d.self
					}
				}
				f.decls = append(f.decls, d)
			}
		}
		if obj, name := access(sp.family, n, t, lang); obj != nil && name != nil {
			f.props[row] = append(f.props[row], name.Text(src))
			if isSelf(sp.family, obj, lang, src, recv) {
				f.selfs[row] = append(f.selfs[row], name.Text(src))
			}
		}
		switch t {
		case "identifier":
			f.idents[row] = append(f.idents[row], n.Text(src))
			if f.py != nil && isPyValue(n, lang) {
				f.py.values[row] = append(f.py.values[row], n.Text(src))
			}
		case "import_statement", "export_statement", "import_from_statement":
			if f.ts != nil && n.Parent() != nil && n.Parent().Type(lang) == "program" {
				readTSModule(f.ts, n, lang, src)
			}
			if f.py != nil {
				readPyImport(f.py, n, lang, src)
			}
		case "aliased_import", "use_as_clause": // Python, Rust
			name, alias := n.ChildByFieldName("name", lang), n.ChildByFieldName("alias", lang)
			if name == nil {
				name = n.ChildByFieldName("path", lang)
			}
			if name != nil && alias != nil {
				f.aliases[alias.Text(src)] = lastSegment(name.Text(src))
			}
		case "package_clause":
			if n.NamedChildCount() > 0 {
				f.pkg = n.NamedChild(0).Text(src)
			}
		case "import_spec":
			alias, imp := n.ChildByFieldName("name", lang), n.ChildByFieldName("path", lang)
			if imp == nil {
				break
			}
			p := strings.Trim(imp.Text(src), "`\"")
			switch {
			case alias == nil:
				// By the path's last element: usually the package name.
				// When it isn't, callsPackageFunc checks the callee's
				// package name against plainImports.
				f.imports[path.Base(p)] = p
				f.plainImports = append(f.plainImports, p)
			case alias.Text(src) == ".":
				f.dotImports = append(f.dotImports, p)
			default:
				f.imports[alias.Text(src)] = p
			}
		}
		if sp.signatures[t] {
			f.signatures[row] = true
		}
		if field, ok := sp.calls[t]; ok {
			if c, ok := readCall(n.ChildByFieldName(field, lang), lang, src); ok {
				f.uses[row] = append(f.uses[row], c)
			}
		}
		if typeRef(sp.family, n, lang, src, func(c call) {
			// One letter is enough: it may be an alias (import { Project
			// as P }); lookupAll drops it if it isn't.
			if isIdent(c.name) && !sp.builtins[c.name] {
				f.types[row] = append(f.types[row], c)
			}
		}) {
			return // a type reference holds no declarations or calls
		}
		for i := 0; i < n.ChildCount(); i++ {
			visit(n.Child(i), recv)
		}
	}
	visit(tree.RootNode(), "")
	if f.ts != nil {
		for local, imp := range f.ts.imports {
			if imp.name != "*" && imp.name != "default" && imp.name != local {
				f.aliases[local] = imp.name
			}
		}
	}
	return f
}

// unalias returns c with the name an alias stands for.
func (f *parsedFile) unalias(c call) call {
	if orig, ok := f.aliases[c.name]; ok {
		c.name = orig
	}
	return c
}

func readDecl(n *gotreesitter.Node, k declKind, t string, lang *gotreesitter.Language, src []byte) (decl, bool) {
	name := n.ChildByFieldName("name", lang)
	if name == nil {
		return decl{}, false
	}
	if t == "variable_declarator" {
		// Functions assigned to a name, const f = () => {...}, and other
		// top-level constants, such as a schema a type is inferred from.
		v := n.ChildByFieldName("value", lang)
		isFunc := v != nil && (v.Type(lang) == "arrow_function" || v.Type(lang) == "function_expression" || v.Type(lang) == "function")
		if !isFunc {
			if v == nil || !isTopLevel(n.Parent(), lang) {
				return decl{}, false
			}
			k.callable = false
		}
	}
	shown := n
	if p := n.Parent(); k.outer != "" && p != nil && p.Type(lang) == k.outer && p.NamedChildCount() == 1 {
		shown = p
	}
	return decl{
		name:          name.Text(src),
		callable:      k.callable,
		packageScoped: k.packageScoped,
		isType:        k.isType,
		topLevel:      isTopLevel(shown, lang),
		start:         int(shown.StartPoint().Row) + 1,
		end:           int(shown.EndPoint().Row) + 1,
	}, true
}

// isTopLevel reports whether n is at the top of its file, exported or not.
func isTopLevel(n *gotreesitter.Node, lang *gotreesitter.Language) bool {
	if n == nil {
		return false
	}
	p := n.Parent()
	if p != nil && (p.Type(lang) == "export_statement" || p.Type(lang) == "decorated_definition") {
		p = p.Parent()
	}
	if p == nil {
		return false
	}
	switch p.Type(lang) {
	case "program", "module", "source_file":
		return true
	}
	return false
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

var (
	nameRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]+$`)
	identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// isName reports whether s is an identifier worth looking up: at least two
// characters, so a one-letter variable isn't searched for.
func isName(s string) bool { return nameRe.MatchString(s) }

// isIdent reports whether s is an identifier, one letter included.
func isIdent(s string) bool { return identRe.MatchString(s) }

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

// enclosingType returns the innermost type declaration around line, for a
// field read through this or self.
func (f *parsedFile) enclosingType(line int) (decl, bool) {
	return f.innermost(line, func(d decl) bool { return d.isType })
}

// enclosingCallable returns the innermost function around line.
func (f *parsedFile) enclosingCallable(line int) (decl, bool) {
	return f.innermost(line, func(d decl) bool { return d.callable })
}

func (f *parsedFile) innermost(line int, ok func(decl) bool) (decl, bool) {
	var best decl
	found := false
	for _, d := range f.decls {
		if ok(d) && d.start <= line && line <= d.end && (!found || d.end-d.start < best.end-best.start) {
			best, found = d, true
		}
	}
	return best, found
}
