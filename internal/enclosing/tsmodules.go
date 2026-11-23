package enclosing

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// TypeScript module resolution: a name a file imports is looked up in the
// file it is imported from, through relative paths, tsconfig path aliases
// (an Nx workspace's @org/models) and barrel files (export * from), instead
// of by grep. That finds the right declaration of a common name like
// Project, and one imported under an alias.

// tsImport is a name a file imports: the module and the name it exports
// ("default", or "*" for import * as ns).
type tsImport struct{ source, name string }

// tsReexport is export { name as as } from 'source'.
type tsReexport struct{ source, name, as string }

// tsModule is what a TypeScript file imports and exports.
type tsModule struct {
	imports   map[string]tsImport // by local name
	reexports []tsReexport
	// exportAll are the modules of export * from.
	exportAll []string
	// localExports maps export { a as b } (without from) b to a.
	localExports map[string]string
	// defaultName is the declaration export default names.
	defaultName string
}

// readTSModule records n, a top-level import or export statement, in m.
func readTSModule(m *tsModule, n *node, lang *gotreesitter.Language, src []byte) {
	t := n.Type(lang)
	source := ""
	if s := n.ChildByFieldName("source", lang); s != nil {
		source = strings.Trim(s.Text(src), "'\"`")
	}
	text := func(n *node) string {
		if n == nil {
			return ""
		}
		return n.Text(src)
	}
	switch t {
	case "import_statement":
		if source == "" {
			return
		}
		for _, c := range descendants(n, lang, "import_clause") {
			for i := 0; i < c.NamedChildCount(); i++ {
				switch k := c.NamedChild(i); k.Type(lang) {
				case "identifier":
					m.imports[k.Text(src)] = tsImport{source, "default"}
				case "namespace_import":
					if k.NamedChildCount() > 0 {
						m.imports[k.NamedChild(0).Text(src)] = tsImport{source, "*"}
					}
				case "named_imports":
					for _, s := range descendants(k, lang, "import_specifier") {
						name, alias := text(s.ChildByFieldName("name", lang)), text(s.ChildByFieldName("alias", lang))
						if alias == "" {
							alias = name
						}
						m.imports[alias] = tsImport{source, name}
					}
				}
			}
		}
	case "export_statement":
		isDefault := false
		for i := 0; i < n.ChildCount(); i++ {
			if c := n.Child(i); !c.IsNamed() && c.Text(src) == "default" {
				isDefault = true
			}
		}
		if isDefault {
			if d := n.ChildByFieldName("declaration", lang); d != nil {
				m.defaultName = text(d.ChildByFieldName("name", lang))
			} else if v := n.ChildByFieldName("value", lang); v != nil && v.Type(lang) == "identifier" {
				m.defaultName = v.Text(src)
			}
			return
		}
		clauses := descendants(n, lang, "export_clause")
		switch {
		case len(clauses) > 0:
			for _, s := range descendants(clauses[0], lang, "export_specifier") {
				name, as := text(s.ChildByFieldName("name", lang)), text(s.ChildByFieldName("alias", lang))
				if as == "" {
					as = name
				}
				if source != "" {
					m.reexports = append(m.reexports, tsReexport{source, name, as})
				} else {
					m.localExports[as] = name
				}
			}
		case source != "" && !hasChild(n, lang, "namespace_export"):
			m.exportAll = append(m.exportAll, source)
		}
	}
}

// maxExportDepth caps how many files a lookup follows through re-exports.
const maxExportDepth = 12

// resolveImport looks c up through from's imports: a name it imports, a
// name in a namespace it imports, or, if neither, a declaration in from
// itself. It reports false when it can't tell, for name-based lookup.
// TypeScript, JavaScript and Python follow imports; see pymodules.go.
func (r *related) resolveImport(c call, from *parsedFile) ([]decl, bool) {
	if from.ts == nil && from.py == nil {
		return nil, false
	}
	key := from.path + "\x00" + c.qualifier + "\x00" + c.name + "\x00" + string(rune('0'+c.kind))
	if res, ok := r.imported[key]; ok {
		return res, res != nil
	}
	res := r.resolveImportUncached(c, from)
	r.imported[key] = res
	return res, res != nil
}

func (r *related) resolveImportUncached(c call, from *parsedFile) []decl {
	if from.py != nil {
		return r.resolvePyImport(c, from)
	}
	var source, name string
	if c.qualifier != "" {
		imp, ok := from.ts.imports[c.qualifier]
		if !ok || imp.name != "*" {
			return nil
		}
		source, name = imp.source, c.name
	} else {
		imp, ok := from.ts.imports[c.name]
		if !ok {
			return exported(from, c.name, c.kind)
		}
		if imp.name == "*" {
			return nil
		}
		source, name = imp.source, imp.name
	}
	mod := r.resolveModule(from.path, source)
	if mod == "" {
		return nil
	}
	return r.findExport(mod, name, c.kind, map[string]bool{})
}

// exported returns f's top-level declarations named name, of the kind a
// reference of kind k can name.
func exported(f *parsedFile, name string, k refKind) []decl {
	var out []decl
	for _, d := range f.decls {
		if d.topLevel && d.name == name && (k != refType || d.isType) {
			out = append(out, d)
		}
	}
	return out
}

// findExport finds the declaration module p exports as name, through
// re-exports.
func (r *related) findExport(p, name string, k refKind, seen map[string]bool) []decl {
	if seen[p+"\x00"+name] || len(seen) > maxExportDepth {
		return nil
	}
	seen[p+"\x00"+name] = true
	f := r.file(p, false)
	if f == nil || f.ts == nil {
		return nil
	}
	if name == "default" {
		if f.ts.defaultName == "" {
			return nil
		}
		name = f.ts.defaultName
	}
	if ds := exported(f, name, k); len(ds) > 0 {
		return ds
	}
	follow := func(source, name string) []decl {
		if mod := r.resolveModule(p, source); mod != "" {
			return r.findExport(mod, name, k, seen)
		}
		return nil
	}
	if orig, ok := f.ts.localExports[name]; ok && orig != name {
		if ds := r.findExport(p, orig, k, seen); len(ds) > 0 {
			return ds
		}
	}
	if imp, ok := f.ts.imports[name]; ok && imp.name != "*" {
		return follow(imp.source, imp.name)
	}
	for _, re := range f.ts.reexports {
		if re.as == name {
			return follow(re.source, re.name)
		}
	}
	for _, src := range f.ts.exportAll {
		if ds := follow(src, name); len(ds) > 0 {
			return ds
		}
	}
	return nil
}

// resolveModule returns the repository file an import of source from the
// file from refers to, or "".
func (r *related) resolveModule(from, source string) string {
	key := path.Dir(from) + "\x00" + source
	if p, ok := r.modulePaths[key]; ok {
		return p
	}
	var bases []string
	if strings.HasPrefix(source, ".") {
		bases = []string{path.Join(path.Dir(from), source)}
	} else {
		bases = r.tsconfig(path.Dir(from)).candidates(source)
	}
	p := ""
	for _, b := range bases {
		if p = r.moduleFile(b); p != "" {
			break
		}
	}
	r.modulePaths[key] = p
	return p
}

// moduleFile returns the TypeScript or JavaScript file a module path
// without its extension names, or a directory's index file.
func (r *related) moduleFile(base string) string {
	if base == ".." || strings.HasPrefix(base, "../") {
		return ""
	}
	var cands []string
	switch ext := path.Ext(base); ext {
	case ".ts", ".tsx", ".mts", ".cts":
		cands = append(cands, base)
	case ".js", ".jsx", ".mjs", ".cjs":
		// ESM imports name the compiled file: './project.js'.
		stem := strings.TrimSuffix(base, ext)
		cands = append(cands, stem+".ts", stem+".tsx", base)
	}
	for _, ext := range []string{".ts", ".tsx", ".d.ts", "/index.ts", "/index.tsx", ".js", "/index.js"} {
		cands = append(cands, base+ext)
	}
	for _, c := range cands {
		if r.file(c, false) != nil {
			return c
		}
	}
	return ""
}

// tsPaths is the module resolution a tsconfig sets up.
type tsPaths struct {
	// base is the directory paths are relative to; baseURL, if set, also
	// resolves bare module names.
	base, baseURL string
	paths         map[string][]string
}

// candidates returns the paths, without extensions, a bare module name may
// be at: by the most specific matching paths pattern, then by baseUrl.
func (c tsPaths) candidates(source string) []string {
	var out []string
	best, star, bestLen := "", "", -1
	for pat := range c.paths {
		if pat == source {
			best, star = pat, ""
			break
		}
		pre, suf, ok := strings.Cut(pat, "*")
		if ok && len(pre) > bestLen && strings.HasPrefix(source, pre) && strings.HasSuffix(source, suf) && len(source) >= len(pre)+len(suf) {
			best, star, bestLen = pat, source[len(pre):len(source)-len(suf)], len(pre)
		}
	}
	if best != "" {
		for _, t := range c.paths[best] {
			out = append(out, path.Join(c.base, strings.Replace(t, "*", star, 1)))
		}
	}
	if c.baseURL != "" {
		out = append(out, path.Join(c.baseURL, source))
	}
	return out
}

// tsconfig returns the module resolution for files in dir: from the
// nearest tsconfig.json or tsconfig.base.json at or above it that sets
// paths or baseUrl, following extends.
func (r *related) tsconfig(dir string) tsPaths {
	if c, ok := r.tsconfigs[dir]; ok {
		return c
	}
	var c tsPaths
	for d := dir; ; d = path.Dir(d) {
		found := false
		for _, name := range []string{"tsconfig.json", "tsconfig.base.json"} {
			if cfg, ok := r.loadTsconfig(path.Join(d, name), 0); ok && (len(cfg.paths) > 0 || cfg.baseURL != "") {
				c, found = cfg, true
				break
			}
		}
		if found || d == "." || d == "/" {
			break
		}
	}
	r.tsconfigs[dir] = c
	return c
}

// loadTsconfig reads the config at p and the configs it extends.
func (r *related) loadTsconfig(p string, depth int) (tsPaths, bool) {
	src, err := r.read(p)
	if err != nil || depth > 5 {
		return tsPaths{}, false
	}
	var cfg struct {
		Extends         json.RawMessage `json:"extends"`
		CompilerOptions struct {
			BaseURL *string             `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if json.Unmarshal(stripJSONC(src), &cfg) != nil {
		return tsPaths{}, false
	}
	var out tsPaths
	var parents []string
	if json.Unmarshal(cfg.Extends, &parents) != nil {
		var one string
		if json.Unmarshal(cfg.Extends, &one) == nil && one != "" {
			parents = []string{one}
		}
	}
	for _, e := range parents {
		if !strings.HasPrefix(e, ".") {
			continue // a package's config
		}
		ep := path.Join(path.Dir(p), e)
		parent, ok := r.loadTsconfig(ep, depth+1)
		if !ok && path.Ext(ep) != ".json" {
			parent, ok = r.loadTsconfig(ep+".json", depth+1)
		}
		if ok {
			if parent.baseURL != "" {
				out.baseURL = parent.baseURL
			}
			if parent.paths != nil {
				out.paths, out.base = parent.paths, parent.base
			}
		}
	}
	dir := path.Dir(p)
	if b := cfg.CompilerOptions.BaseURL; b != nil {
		out.baseURL = path.Join(dir, *b)
	}
	if cfg.CompilerOptions.Paths != nil {
		out.paths, out.base = cfg.CompilerOptions.Paths, dir
	}
	if out.baseURL != "" {
		out.base = out.baseURL // paths are relative to baseUrl when set
	}
	return out, true
}

// stripJSONC removes the comments and trailing commas tsconfig files allow.
func stripJSONC(src []byte) []byte {
	return dropTrailingCommas(dropComments(src))
}

// scanJSON calls f for each byte outside strings with its index, and
// copies the rest; f returns how many bytes it consumed and what to write
// instead.
func scanJSON(src []byte, f func(i int) (n int, out []byte)) []byte {
	var b []byte
	inStr := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case inStr:
			b = append(b, c)
			if c == '\\' && i+1 < len(src) {
				i++
				b = append(b, src[i])
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			b = append(b, c)
		default:
			n, out := f(i)
			b = append(b, out...)
			i += n - 1
		}
	}
	return b
}

func dropComments(src []byte) []byte {
	return scanJSON(src, func(i int) (int, []byte) {
		switch {
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '/':
			j := i
			for j < len(src) && src[j] != '\n' {
				j++
			}
			return j - i, nil
		case src[i] == '/' && i+1 < len(src) && src[i+1] == '*':
			j := i + 2
			for j+1 < len(src) && (src[j] != '*' || src[j+1] != '/') {
				j++
			}
			return min(j+2, len(src)) - i, []byte{' '}
		}
		return 1, src[i : i+1]
	})
}

func dropTrailingCommas(src []byte) []byte {
	return scanJSON(src, func(i int) (int, []byte) {
		if src[i] == ',' {
			j := i + 1
			for j < len(src) && strings.ContainsRune(" \t\r\n", rune(src[j])) {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				return 1, nil
			}
		}
		return 1, src[i : i+1]
	})
}
