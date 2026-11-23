package enclosing

import (
	"path"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// Python module resolution: a name a file imports is looked up in the
// module it is imported from, through relative imports, packages'
// __init__.py re-exports and star imports, instead of by grep. That finds
// the right class among several of the same name (a model, a schema and a
// report row all called Project), and one imported under an alias.

// pyImport is a name a Python file imports: from module import name, or,
// with no name, the module itself (import a.b as m). Relative modules keep
// their leading dots.
type pyImport struct{ module, name string }

// pyModule is what a Python file imports.
type pyModule struct {
	imports map[string]pyImport // by local name
	// rebound are the local names more than one import binds to different
	// things, such as a function's own import or a try/except ImportError
	// fallback. Which one a use means depends on scope and control flow,
	// so they are left to the lookup by name.
	rebound map[string]bool
	// starImports are the modules of from m import *.
	starImports []string
	// values are the identifiers used as values, by line: not attribute
	// names (Project in models.Project) or keyword argument names.
	values map[int][]string
}

func newPyModule() *pyModule {
	return &pyModule{imports: map[string]pyImport{}, rebound: map[string]bool{}, values: map[int][]string{}}
}

// bind records that an import binds local to imp.
func (m *pyModule) bind(local string, imp pyImport) {
	if prev, ok := m.imports[local]; ok && prev != imp {
		m.rebound[local] = true
	}
	m.imports[local] = imp
}

// imported returns what local is imported as, unless more than one import
// binds it.
func (m *pyModule) imported(local string) (pyImport, bool) {
	imp, ok := m.imports[local]
	return imp, ok && !m.rebound[local]
}

// isPyValue reports whether n, an identifier, is used as a value: not the
// attribute of x.attr or the name of a keyword argument.
func isPyValue(n *node, lang *gotreesitter.Language) bool {
	p := n.Parent()
	if p == nil {
		return true
	}
	field := ""
	switch p.Type(lang) {
	case "attribute":
		field = "attribute"
	case "keyword_argument":
		field = "name"
	default:
		return true
	}
	f := p.ChildByFieldName(field, lang)
	return f == nil || f.StartByte() != n.StartByte()
}

// readPyImport records n, an import statement, in m. Imports anywhere in
// the file count, such as those under if TYPE_CHECKING: or in a function;
// a name two of them bind differently is marked rebound.
func readPyImport(m *pyModule, n *node, lang *gotreesitter.Language, src []byte) {
	t := n.Type(lang)
	if t != "import_statement" && t != "import_from_statement" {
		return
	}
	module := ""
	if t == "import_from_statement" {
		mod := n.ChildByFieldName("module_name", lang)
		if mod == nil {
			return
		}
		module = mod.Text(src)
	}
	for i := 0; i < n.ChildCount(); i++ {
		c := n.Child(i)
		switch {
		case c.Type(lang) == "wildcard_import":
			m.starImports = append(m.starImports, module)
		case n.FieldNameForChild(i, lang) != "name":
		case c.Type(lang) == "aliased_import":
			name, alias := c.ChildByFieldName("name", lang), c.ChildByFieldName("alias", lang)
			if name == nil || alias == nil {
				continue
			}
			if t == "import_statement" {
				m.bind(alias.Text(src), pyImport{module: name.Text(src)})
			} else {
				m.bind(alias.Text(src), pyImport{module, name.Text(src)})
			}
		case t == "import_statement":
			// import a.b: a.b.f() is qualified by a.b.
			m.bind(c.Text(src), pyImport{module: c.Text(src)})
		default:
			m.bind(c.Text(src), pyImport{module, c.Text(src)})
		}
	}
}

// resolvePyImport looks c up through from's imports: a name it imports, a
// name in a module it imports, or, if neither, a declaration in from
// itself or a module it star-imports. It returns nil when it can't tell,
// for name-based lookup.
func (r *related) resolvePyImport(c call, from *parsedFile) []decl {
	if c.qualifier != "" {
		imp, ok := from.py.imported(c.qualifier)
		if !ok {
			return nil
		}
		mod := r.pyImportedModule(from.path, imp)
		if mod == "" {
			return nil
		}
		return r.findPyExport(mod, c.name, c.kind, map[string]bool{})
	}
	return r.findPyExport(from.path, c.name, c.kind, map[string]bool{})
}

// importedClasses returns the classes line of f names as values, such as
// User in db.get(User, user_id) or select(User): names f imports from a
// module in the repository that declares a class of that name. Only
// imports are followed, so no name is searched for.
func (r *related) importedClasses(f *parsedFile, line int) []ref {
	if f.py == nil {
		return nil
	}
	var out []ref
	for _, id := range f.py.values[line] {
		if imp, ok := f.py.imported(id); !ok || imp.name == "" {
			continue
		}
		c := call{name: id, kind: refType}
		if _, ok := r.resolveImport(c, f); ok {
			out = append(out, ref{c, f})
		}
	}
	return out
}

// pyImportedModule returns the file of the module imp binds: import a.b as
// m, or from a import b where b is a submodule.
func (r *related) pyImportedModule(from string, imp pyImport) string {
	if imp.name == "" {
		return r.resolvePyModule(from, imp.module)
	}
	sub := imp.module + "." + imp.name
	if strings.HasSuffix(imp.module, ".") {
		sub = imp.module + imp.name // from .. import models
	}
	return r.resolvePyModule(from, sub)
}

// findPyExport finds the declaration named name in module p: declared
// there, imported there (an __init__.py re-exporting its submodules'
// names), or star-imported.
func (r *related) findPyExport(p, name string, k refKind, seen map[string]bool) []decl {
	if seen[p+"\x00"+name] || len(seen) > maxExportDepth {
		return nil
	}
	seen[p+"\x00"+name] = true
	f := r.file(p, false)
	if f == nil || f.py == nil {
		return nil
	}
	if ds := exported(f, name, k); len(ds) > 0 {
		return ds
	}
	if f.py.rebound[name] {
		return nil
	}
	if imp, ok := f.py.imports[name]; ok {
		if imp.name == "" {
			return nil // a module
		}
		if mod := r.resolvePyModule(p, imp.module); mod != "" {
			return r.findPyExport(mod, imp.name, k, seen)
		}
		return nil
	}
	for _, m := range f.py.starImports {
		if mod := r.resolvePyModule(p, m); mod != "" {
			if ds := r.findPyExport(mod, name, k, seen); len(ds) > 0 {
				return ds
			}
		}
	}
	return nil
}

// resolvePyModule returns the repository file of the module an import in
// the file from names, or "".
func (r *related) resolvePyModule(from, module string) string {
	key := "py\x00" + path.Dir(from) + "\x00" + module
	if p, ok := r.modulePaths[key]; ok {
		return p
	}
	p := ""
	top, _, _ := strings.Cut(module, ".")
	for _, b := range pyModuleBases(from, module) {
		// An absolute import's top package must be there too: one check
		// rules out a whole root for a package from outside the
		// repository, such as sqlalchemy.orm.
		if top != "" && r.pyModuleFile(strings.TrimSuffix(b, strings.ReplaceAll(strings.TrimPrefix(module, top), ".", "/"))) == "" {
			continue
		}
		if p = r.pyModuleFile(b); p != "" {
			break
		}
	}
	r.modulePaths[key] = p
	return p
}

// pyModuleFile returns the file a module path without its extension
// names: a module, or a package's __init__.py.
func (r *related) pyModuleFile(base string) string {
	for _, c := range []string{base + ".py", base + "/__init__.py"} {
		if r.file(c, false) != nil {
			return c
		}
	}
	return ""
}

// pyModuleBases returns the paths, without extensions, the module an
// import in the file from names may be at. A relative module is under
// from's package. An absolute one is searched for from from's directory
// up to the repository root, and in a src directory at each level, the
// nearest first: that finds a src layout, a package in a monorepo's
// subdirectory (server/polar) and the repository root without reading
// any configuration.
func pyModuleBases(from, module string) []string {
	dir := path.Dir(from)
	rest := strings.TrimLeft(module, ".")
	rel := strings.ReplaceAll(rest, ".", "/")
	if dots := len(module) - len(rest); dots > 0 {
		for range dots - 1 {
			if dir == "." {
				return nil
			}
			dir = path.Dir(dir)
		}
		return []string{path.Join(dir, rel)}
	}
	var out []string
	seen := map[string]bool{}
	for d := dir; ; d = path.Dir(d) {
		for _, b := range []string{path.Join(d, rel), path.Join(d, "src", rel)} {
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
		if d == "." || d == "/" {
			break
		}
	}
	return out
}
