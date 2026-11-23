package enclosing

import (
	"strings"
	"unicode"

	"github.com/odvcencio/gotreesitter"
)

// Type lookups: the types a change uses, the types in the signatures of
// the functions it changes, the types of the fields it reads through this,
// self or a Go receiver, and the members of long type definitions that it
// uses. Like calls, they are found by name; TypeScript also follows imports
// (see tsmodules.go).

type node = gotreesitter.Node

// refKind says what a reference names.
type refKind uint8

const (
	refCall  refKind = iota // a function or constructor call
	refType                 // a type
	refValue                // a value used in a type (TypeScript typeof)
)

// span is a range of lines, 1-based and inclusive.
type span struct{ start, end int }

// member is a field or method of a type, or a validator of a field
// (pydantic's field_validator), with the types its declaration names.
type member struct {
	span
	types []call
}

// lang families, from spec.family.
const (
	famGo     = "go"
	famPython = "python"
	famRust   = "rust"
	famJS     = "js"
)

// typeRef records, through add, the types the type reference n names, and
// reports whether n was one: its children are then not visited. Names in
// declarations (interface Project, type P = ...) aren't references.
func typeRef(fam string, n *node, lang *gotreesitter.Language, src []byte, add func(call)) bool {
	t := n.Type(lang)
	switch fam {
	case famJS:
		switch t {
		case "type_identifier":
			if !isDeclName(n, lang, "class_declaration", "abstract_class_declaration", "interface_declaration", "type_alias_declaration", "enum_declaration", "type_parameter", "class") {
				add(call{name: n.Text(src), kind: refType})
			}
			return true
		case "nested_type_identifier":
			addQualified(n.ChildByFieldName("module", lang), n.ChildByFieldName("name", lang), src, add)
			return true
		case "type_query":
			if c := n.NamedChild(0); c != nil && c.Type(lang) == "identifier" {
				add(call{name: c.Text(src), kind: refValue})
			}
			return true
		case "extends_clause":
			// A class's base is a value: class C extends Base.
			if v := n.ChildByFieldName("value", lang); v != nil && v.Type(lang) == "identifier" {
				add(call{name: v.Text(src), kind: refType})
			}
		}
	case famGo:
		switch t {
		case "type_identifier":
			if !isDeclName(n, lang, "type_spec", "type_alias", "type_parameter_declaration") {
				add(call{name: n.Text(src), kind: refType})
			}
			return true
		case "qualified_type":
			addQualified(n.ChildByFieldName("package", lang), n.ChildByFieldName("name", lang), src, add)
			return true
		}
	case famRust:
		switch t {
		case "type_identifier":
			if !isDeclName(n, lang, "struct_item", "enum_item", "trait_item", "type_item", "union_item") {
				add(call{name: n.Text(src), kind: refType})
			}
			return true
		case "scoped_type_identifier":
			addQualified(n.ChildByFieldName("path", lang), n.ChildByFieldName("name", lang), src, add)
			return true
		}
	case famPython:
		if t != "type" {
			return false
		}
		// A type hint: every name in it, Optional[list[Team]] included,
		// and forward references in strings.
		var walk func(n *node)
		walk = func(n *node) {
			switch n.Type(lang) {
			case "identifier":
				add(call{name: n.Text(src), kind: refType})
				return
			case "attribute":
				addQualified(n.ChildByFieldName("object", lang), n.ChildByFieldName("attribute", lang), src, add)
				return
			case "string_content":
				add(call{name: n.Text(src), kind: refType})
				return
			}
			for i := 0; i < n.ChildCount(); i++ {
				walk(n.Child(i))
			}
		}
		walk(n)
		return true
	}
	return false
}

// addQualified adds the type name, qualified by the last segment of q
// (models in models.Project, Foo in crate::models::Foo).
func addQualified(q, name *node, src []byte, add func(call)) {
	if name == nil {
		return
	}
	c := call{name: name.Text(src), kind: refType}
	if q != nil {
		c.qualifier = lastSegment(q.Text(src))
	}
	add(c)
}

func lastSegment(s string) string {
	if i := strings.LastIndexAny(s, ".:"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// isDeclName reports whether n is the name of a node of one of the types.
func isDeclName(n *node, lang *gotreesitter.Language, types ...string) bool {
	p := n.Parent()
	if p == nil {
		return false
	}
	for _, t := range types {
		if p.Type(lang) == t {
			name := p.ChildByFieldName("name", lang)
			return name != nil && name.StartByte() == n.StartByte()
		}
	}
	return false
}

// typeRefs returns the types n's subtree refers to.
func typeRefs(fam string, n *node, lang *gotreesitter.Language, src []byte) []call {
	if n == nil {
		return nil
	}
	var out []call
	add := func(c call) {
		if isIdent(c.name) {
			out = append(out, c)
		}
	}
	var walk func(n *node)
	walk = func(n *node) {
		if typeRef(fam, n, lang, src, add) {
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(n)
	return out
}

// typeQualifier returns the type a call is made on, if its qualifier looks
// like one: Project in Project.model_validate(...) or Foo::new().
func typeQualifier(c call) (call, bool) {
	if c.qualifier == "" || c.kind != refCall {
		return call{}, false
	}
	q := lastSegment(c.qualifier)
	if !isName(q) || !unicode.IsUpper(rune(q[0])) {
		return call{}, false
	}
	return call{name: q, kind: refType}, true
}

// signature returns the types a use of a declaration yields: a function's
// return types, a class's bases, an alias's type. They are followed one
// step further than the types a change uses: the type of
// const p = store.selected() is in selected's signature.
func signature(fam string, n *node, t string, lang *gotreesitter.Language, src []byte) []call {
	refs := func(fields ...string) []call {
		var out []call
		for _, f := range fields {
			out = append(out, typeRefs(fam, n.ChildByFieldName(f, lang), lang, src)...)
		}
		return out
	}
	childOf := func(types ...string) []call {
		var out []call
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			for _, ct := range types {
				if c.Type(lang) == ct {
					out = append(out, typeRefs(fam, c, lang, src)...)
				}
			}
		}
		return out
	}
	switch fam {
	case famJS:
		switch t {
		case "function_declaration", "method_definition":
			return refs("return_type")
		case "variable_declarator":
			out := refs("type")
			if v := n.ChildByFieldName("value", lang); v != nil {
				out = append(out, typeRefs(fam, v.ChildByFieldName("return_type", lang), lang, src)...)
			}
			return out
		case "class_declaration", "abstract_class_declaration":
			return childOf("class_heritage")
		case "interface_declaration":
			return childOf("extends_type_clause")
		case "type_alias_declaration":
			return refs("value")
		}
	case famGo:
		switch t {
		case "function_declaration", "method_declaration":
			return refs("result")
		case "type_alias":
			return refs("type")
		case "type_spec":
			ty := n.ChildByFieldName("type", lang)
			if ty == nil {
				return nil
			}
			if ty.Type(lang) != "struct_type" {
				return typeRefs(fam, ty, lang, src)
			}
			// Embedded fields.
			var out []call
			for _, fd := range structFields(ty, lang) {
				if fd.ChildByFieldName("name", lang) == nil {
					out = append(out, typeRefs(fam, fd.ChildByFieldName("type", lang), lang, src)...)
				}
			}
			return out
		}
	case famRust:
		switch t {
		case "function_item":
			return refs("return_type")
		case "type_item":
			return refs("type")
		}
	case famPython:
		switch t {
		case "function_definition":
			return refs("return_type")
		case "class_definition":
			sc := n.ChildByFieldName("superclasses", lang)
			if sc == nil {
				return nil
			}
			var out []call
			for i := 0; i < sc.NamedChildCount(); i++ {
				switch b := sc.NamedChild(i); b.Type(lang) {
				case "identifier":
					out = append(out, call{name: b.Text(src), kind: refType})
				case "attribute":
					addQualified(b.ChildByFieldName("object", lang), b.ChildByFieldName("attribute", lang), src, func(c call) { out = append(out, c) })
				}
			}
			return out
		}
	}
	return nil
}

// params returns the types of a function's parameters by name, the Go
// receiver's included. A change's function contributes the types of the
// parameters the change uses.
func params(fam string, n *node, lang *gotreesitter.Language, src []byte) map[string][]call {
	out := map[string][]call{}
	lists := []*node{n.ChildByFieldName("parameters", lang), n.ChildByFieldName("receiver", lang)}
	if v := n.ChildByFieldName("value", lang); v != nil { // const f = (p: P) => ...
		lists = append(lists, v.ChildByFieldName("parameters", lang))
	}
	for _, ps := range lists {
		for i := 0; ps != nil && i < ps.NamedChildCount(); i++ {
			p := ps.NamedChild(i)
			var names []string
			var ty *node
			switch fam {
			case famJS, famRust:
				if pat := p.ChildByFieldName("pattern", lang); pat != nil && pat.Type(lang) == "identifier" {
					names = []string{pat.Text(src)}
				}
				ty = p.ChildByFieldName("type", lang)
			case famGo:
				for j := 0; j < p.ChildCount(); j++ {
					if p.FieldNameForChild(j, lang) == "name" {
						names = append(names, p.Child(j).Text(src))
					}
				}
				ty = p.ChildByFieldName("type", lang)
			case famPython:
				if t := p.Type(lang); (t == "typed_parameter" || t == "typed_default_parameter") && p.NamedChildCount() > 0 {
					name := p.ChildByFieldName("name", lang)
					if name == nil {
						name = p.NamedChild(0)
					}
					names = []string{name.Text(src)}
				}
				ty = p.ChildByFieldName("type", lang)
			}
			if types := typeRefs(fam, ty, lang, src); len(types) > 0 {
				for _, name := range names {
					out[name] = types
				}
			}
		}
	}
	return out
}

// structFields returns a Go struct type's field declarations.
func structFields(st *node, lang *gotreesitter.Language) []*node {
	var out []*node
	for i := 0; i < st.NamedChildCount(); i++ {
		if l := st.NamedChild(i); l.Type(lang) == "field_declaration_list" {
			for j := 0; j < l.NamedChildCount(); j++ {
				if fd := l.NamedChild(j); fd.Type(lang) == "field_declaration" {
					out = append(out, fd)
				}
			}
		}
	}
	return out
}

// members returns a type declaration's fields and methods by name, with
// the types each field's declaration names.
func members(fam string, n *node, t string, lang *gotreesitter.Language, src []byte) map[string][]member {
	out := map[string][]member{}
	add := func(name string, at *node, types []call) {
		if name == "" || at == nil {
			return
		}
		out[name] = append(out[name], member{span{int(at.StartPoint().Row) + 1, int(at.EndPoint().Row) + 1}, types})
	}
	text := func(n *node) string {
		if n == nil {
			return ""
		}
		return n.Text(src)
	}
	switch fam {
	case famJS:
		body := n.ChildByFieldName("body", lang)
		if body == nil {
			return out
		}
		for i := 0; i < body.NamedChildCount(); i++ {
			m := body.NamedChild(i)
			name := m.ChildByFieldName("name", lang)
			switch m.Type(lang) {
			case "public_field_definition", "property_signature":
				add(text(name), m, append(typeRefs(fam, m, lang, src), injected(m.ChildByFieldName("value", lang), lang, src)...))
			case "method_definition":
				add(text(name), m, nil)
				if text(name) == "constructor" {
					// Parameter properties: constructor(private store: Store).
					ps := m.ChildByFieldName("parameters", lang)
					for j := 0; ps != nil && j < ps.NamedChildCount(); j++ {
						p := ps.NamedChild(j)
						if hasChild(p, lang, "accessibility_modifier") {
							add(text(p.ChildByFieldName("pattern", lang)), p, typeRefs(fam, p, lang, src))
						}
					}
				}
			case "method_signature", "abstract_method_signature":
				add(text(name), m, nil)
			}
		}
	case famPython:
		body := n.ChildByFieldName("body", lang)
		if body == nil {
			return out
		}
		for i := 0; i < body.NamedChildCount(); i++ {
			m := body.NamedChild(i)
			if m.Type(lang) == "expression_statement" && m.NamedChildCount() > 0 {
				m = m.NamedChild(0)
			}
			switch m.Type(lang) {
			case "assignment":
				if l := m.ChildByFieldName("left", lang); l != nil && l.Type(lang) == "identifier" {
					add(l.Text(src), m, typeRefs(fam, m.ChildByFieldName("type", lang), lang, src))
				}
			case "function_definition":
				add(text(m.ChildByFieldName("name", lang)), m, nil)
				pyInstanceFields(m, lang, src, add)
			case "decorated_definition":
				def := m.ChildByFieldName("definition", lang)
				if def == nil || def.Type(lang) != "function_definition" {
					continue
				}
				add(text(def.ChildByFieldName("name", lang)), m, nil)
				pyInstanceFields(def, lang, src, add)
				// A validator: @field_validator('team_members').
				for j := 0; j < m.NamedChildCount(); j++ {
					d := m.NamedChild(j)
					if d.Type(lang) != "decorator" {
						continue
					}
					for _, s := range descendants(d, lang, "string_content") {
						if isName(s.Text(src)) {
							add(s.Text(src), m, nil)
						}
					}
				}
			}
		}
	case famGo:
		ty := n.ChildByFieldName("type", lang)
		if ty == nil || ty.Type(lang) != "struct_type" {
			return out
		}
		for _, fd := range structFields(ty, lang) {
			types := typeRefs(fam, fd.ChildByFieldName("type", lang), lang, src)
			named := false
			for j := 0; j < fd.ChildCount(); j++ {
				if fd.FieldNameForChild(j, lang) == "name" {
					named = true
					add(fd.Child(j).Text(src), fd, types)
				}
			}
			if !named && len(types) > 0 {
				add(types[len(types)-1].name, fd, types) // embedded
			}
		}
	case famRust:
		body := n.ChildByFieldName("body", lang)
		if body == nil || body.Type(lang) != "field_declaration_list" {
			return out
		}
		for i := 0; i < body.NamedChildCount(); i++ {
			if fd := body.NamedChild(i); fd.Type(lang) == "field_declaration" {
				add(text(fd.ChildByFieldName("name", lang)), fd, typeRefs(fam, fd.ChildByFieldName("type", lang), lang, src))
			}
		}
	}
	return out
}

// pyInstanceFields adds the fields a Python method sets on self: with
// their annotation, or the annotation of the parameter they are set from
// (self.repo = repo).
func pyInstanceFields(fn *node, lang *gotreesitter.Language, src []byte, add func(string, *node, []call)) {
	params := map[string][]call{}
	if ps := fn.ChildByFieldName("parameters", lang); ps != nil {
		for i := 0; i < ps.NamedChildCount(); i++ {
			p := ps.NamedChild(i)
			if p.Type(lang) == "typed_parameter" || p.Type(lang) == "typed_default_parameter" {
				name := p.ChildByFieldName("name", lang)
				if name == nil && p.NamedChildCount() > 0 {
					name = p.NamedChild(0)
				}
				if name != nil {
					params[name.Text(src)] = typeRefs(famPython, p.ChildByFieldName("type", lang), lang, src)
				}
			}
		}
	}
	for _, a := range descendants(fn.ChildByFieldName("body", lang), lang, "assignment") {
		l := a.ChildByFieldName("left", lang)
		if l == nil || l.Type(lang) != "attribute" {
			continue
		}
		obj, attr := l.ChildByFieldName("object", lang), l.ChildByFieldName("attribute", lang)
		if obj == nil || attr == nil || obj.Text(src) != "self" {
			continue
		}
		types := typeRefs(famPython, a.ChildByFieldName("type", lang), lang, src)
		if r := a.ChildByFieldName("right", lang); len(types) == 0 && r != nil && r.Type(lang) == "identifier" {
			types = params[r.Text(src)]
		}
		add(attr.Text(src), a, types)
	}
}

// injected returns the classes a TypeScript field's initializer is made
// from: inject(ProjectStore), new Store().
func injected(v *node, lang *gotreesitter.Language, src []byte) []call {
	if v == nil {
		return nil
	}
	var out []call
	capital := func(n *node) {
		if n != nil && n.Type(lang) == "identifier" && unicode.IsUpper(rune(n.Text(src)[0])) {
			out = append(out, call{name: n.Text(src), kind: refType})
		}
	}
	for _, c := range append(descendants(v, lang, "call_expression", "new_expression"), v) {
		switch c.Type(lang) {
		case "new_expression":
			capital(c.ChildByFieldName("constructor", lang))
		case "call_expression":
			if args := c.ChildByFieldName("arguments", lang); args != nil {
				for i := 0; i < args.NamedChildCount(); i++ {
					capital(args.NamedChild(i))
				}
			}
		}
	}
	return out
}

func hasChild(n *node, lang *gotreesitter.Language, t string) bool {
	for i := 0; i < n.NamedChildCount(); i++ {
		if n.NamedChild(i).Type(lang) == t {
			return true
		}
	}
	return false
}

// descendants returns the nodes of the types below n, outermost first.
func descendants(n *node, lang *gotreesitter.Language, types ...string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	var walk func(n *node)
	walk = func(n *node) {
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			for _, t := range types {
				if c.Type(lang) == t {
					out = append(out, c)
				}
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// owner returns the type a method belongs to when it isn't written inside
// it, and the name the method's body reads it through: a Go receiver
// (s *Store), or the type of a Rust impl block (self).
func owner(fam string, n *node, t string, lang *gotreesitter.Language, src []byte) (typ, self string) {
	switch {
	case fam == famGo && t == "method_declaration":
		recv := n.ChildByFieldName("receiver", lang)
		if recv == nil || recv.NamedChildCount() == 0 {
			return "", ""
		}
		p := recv.NamedChild(0)
		if name := p.ChildByFieldName("name", lang); name != nil {
			self = name.Text(src)
		}
		if refs := typeRefs(fam, p.ChildByFieldName("type", lang), lang, src); len(refs) > 0 {
			typ = refs[0].name
		}
		return typ, self
	case fam == famRust && t == "function_item":
		for p := n.Parent(); p != nil; p = p.Parent() {
			if p.Type(lang) == "impl_item" {
				if refs := typeRefs(fam, p.ChildByFieldName("type", lang), lang, src); len(refs) > 0 {
					return refs[0].name, "self"
				}
				return "", ""
			}
		}
	}
	return "", ""
}

// access reads a member access: the object and the member's name.
func access(fam string, n *node, t string, lang *gotreesitter.Language) (obj, name *node) {
	field := map[string][2]string{
		famJS:     {"object", "property"},
		famPython: {"object", "attribute"},
		famGo:     {"operand", "field"},
		famRust:   {"value", "field"},
	}[fam]
	want := map[string]string{famJS: "member_expression", famPython: "attribute", famGo: "selector_expression", famRust: "field_expression"}[fam]
	if t != want {
		return nil, nil
	}
	return n.ChildByFieldName(field[0], lang), n.ChildByFieldName(field[1], lang)
}

// isSelf reports whether obj is this, self, or the Go receiver recv.
func isSelf(fam string, obj *node, lang *gotreesitter.Language, src []byte, recv string) bool {
	switch fam {
	case famJS:
		return obj.Type(lang) == "this"
	case famPython:
		return obj.Type(lang) == "identifier" && obj.Text(src) == "self"
	case famRust:
		return obj.Type(lang) == "self"
	case famGo:
		return recv != "" && obj.Type(lang) == "identifier" && obj.Text(src) == recv
	}
	return false
}
