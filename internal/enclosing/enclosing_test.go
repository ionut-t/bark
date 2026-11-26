package enclosing

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeclarations_Go(t *testing.T) {
	source := []byte(`package main

import "fmt"

type User struct {
	Name string
}

func main() {
	fmt.Println("Hello, World!")
}
`)

	// Line 10: fmt.Println("Hello, World!") -> belongs to func main()
	snippets, err := Declarations("main.go", source, []int{10}, nil)
	require.NoError(t, err)

	expected := []string{
		"func main() {\n\tfmt.Println(\"Hello, World!\")\n}",
	}

	assert.Equal(t, expected, snippets)
}

func TestDeclarations_Typescript(t *testing.T) {
	source := []byte(`class Greeting {
    greet() {
        console.log("Hello");
    }
}
`)

	// Line 3: console.log("Hello") -> belongs to greet() method
	snippets, err := Declarations("greet.ts", source, []int{3}, nil)
	require.NoError(t, err)

	expected := []string{
		"greet() {\n        console.log(\"Hello\");\n    }",
	}

	assert.Equal(t, expected, snippets)
}

func TestDeclarations_Python(t *testing.T) {
	source := []byte(`def hello():
    print("hello world")
`)

	// Line 2: print("hello world") -> belongs to def hello()
	snippets, err := Declarations("hello.py", source, []int{2}, nil)
	require.NoError(t, err)

	expected := []string{
		"def hello():\n    print(\"hello world\")",
	}

	assert.Equal(t, expected, snippets)
}

func TestModifiedLinesFromDiff(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
index 1234567..89abcdf 100644
--- a/main.go
+++ b/main.go
@@ -5,4 +5,5 @@
  type User struct {
  	Name string
  }
  
  func main() {
+	fmt.Println("Hello, World!")
  }
`

	modified := ModifiedLinesFromDiff(diffText)
	expected := map[string][]int{
		"main.go": {10},
	}

	assert.Equal(t, expected, modified)
}

func TestModifiedLinesFromDiff_SkipsAddedAndDeletedFiles(t *testing.T) {
	diffText := `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..89abcdf
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func added() {}
diff --git a/gone.go b/gone.go
deleted file mode 100644
index 89abcdf..0000000
--- a/gone.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func removed() {}
diff --git a/edited.go b/edited.go
index 1234567..89abcdf 100644
--- a/edited.go
+++ b/edited.go
@@ -5,4 +5,5 @@
 func main() {
+	fmt.Println("Hello")
 }
`

	modified := ModifiedLinesFromDiff(diffText)

	// Added (new.go) and deleted (gone.go) files are fully present in the diff
	// already, so they must not be enriched. Only the edited file is reported.
	expected := map[string][]int{
		"edited.go": {6},
	}

	assert.Equal(t, expected, modified)
}

func TestModifiedLinesFromDiff_QuotedAndSpacedPaths(t *testing.T) {
	// Git quotes paths containing non-ASCII characters (core.quotepath=true),
	// escaping bytes as octal. Such headers must not leak the previous file's
	// state, and lines must be attributed to the unquoted path. Paths with
	// spaces are emitted unquoted and must also resolve correctly.
	diffText := "diff --git a/first.go b/first.go\n" +
		"index 1234567..89abcdf 100644\n" +
		"--- a/first.go\n" +
		"+++ b/first.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package main\n" +
		"+var a = 1\n" +
		" \n" +
		"diff --git \"a/caf\\303\\251.go\" \"b/caf\\303\\251.go\"\n" +
		"index 1234567..89abcdf 100644\n" +
		"--- \"a/caf\\303\\251.go\"\n" +
		"+++ \"b/caf\\303\\251.go\"\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package main\n" +
		"+var b = 2\n" +
		" \n" +
		"diff --git a/my file.go b/my file.go\n" +
		"index 1234567..89abcdf 100644\n" +
		"--- a/my file.go\n" +
		"+++ b/my file.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package main\n" +
		"+var c = 3\n" +
		" \n"

	modified := ModifiedLinesFromDiff(diffText)
	expected := map[string][]int{
		"first.go":   {2},
		"café.go":    {2},
		"my file.go": {2},
	}

	assert.Equal(t, expected, modified)
}

// Git ends a "--- "/"+++ " line with a tab when the path has a space, so
// the path is the same with and without the tab.
func TestModifiedLinesFromDiff_PathWithSpaceEndsWithTab(t *testing.T) {
	diffText := "diff --git a/my file.go b/my file.go\n" +
		"index 1234567..89abcdf 100644\n" +
		"--- a/my file.go\t\n" +
		"+++ b/my file.go\t\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package main\n" +
		"+var c = 3\n" +
		" \n"

	assert.Equal(t, map[string][]int{"my file.go": {2}}, ModifiedLinesFromDiff(diffText))
	assert.Equal(t, map[string]bool{"my file.go": true}, changedFiles(diffText))
	assert.Equal(t, map[string]map[int]bool{"my file.go": {2: true}}, addedLines(diffText))
}

func TestModifiedLinesFromDiff_ConsecutiveDeletionsDeduped(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
index 1234567..89abcdf 100644
--- a/main.go
+++ b/main.go
@@ -5,6 +5,3 @@
 func main() {
-	a()
-	b()
-	c()
 	d()
 }
`

	// All three deleted lines map to the same following line (6); it must be
	// recorded once, not three times.
	modified := ModifiedLinesFromDiff(diffText)
	expected := map[string][]int{
		"main.go": {6},
	}

	assert.Equal(t, expected, modified)
}

func TestDeclarations_AngularComponent(t *testing.T) {
	source := []byte(`import { Component, signal } from '@angular/core';

@Component({
  selector: 'app-counter',
  templateUrl: './counter.component.html',
})
export class CounterComponent {
  count = signal(0);

  increment(): void {
    this.count.update((c) => c + 1);
  }
}
`)

	// Line 11: this.count.update(...) -> belongs to the increment() method.
	snippets, err := Declarations("counter.component.ts", source, []int{11}, nil)
	require.NoError(t, err)

	expected := []string{
		"increment(): void {\n    this.count.update((c) => c + 1);\n  }",
	}

	assert.Equal(t, expected, snippets)

	// Line 8: the count field -> resolves up to the class declaration only.
	// The @Component decorator is deliberately excluded: class-body changes
	// shouldn't drag decorator metadata (potentially a large inline template)
	// into the snippet.
	snippets, err = Declarations("counter.component.ts", source, []int{8}, nil)
	require.NoError(t, err)

	expected = []string{
		"class CounterComponent {\n  count = signal(0);\n\n  increment(): void {\n    this.count.update((c) => c + 1);\n  }\n}",
	}

	assert.Equal(t, expected, snippets)

	// Line 4: inside the @Component decorator metadata. The decorator is a
	// sibling of class_declaration under export_statement, so the change
	// resolves to the full decorated class.
	snippets, err = Declarations("counter.component.ts", source, []int{4}, nil)
	require.NoError(t, err)

	expected = []string{
		"@Component({\n  selector: 'app-counter',\n  templateUrl: './counter.component.html',\n})\nexport class CounterComponent {\n  count = signal(0);\n\n  increment(): void {\n    this.count.update((c) => c + 1);\n  }\n}",
	}

	assert.Equal(t, expected, snippets)
}

func TestDeclarations_AngularInlineTemplateAndStyles(t *testing.T) {
	source := []byte("import { Component } from '@angular/core';\n" +
		"\n" +
		"@Component({\n" +
		"  selector: 'app-badge',\n" +
		"  template: `\n" +
		"    <span class=\"badge\">\n" +
		"      {{ label }}\n" +
		"    </span>\n" +
		"  `,\n" +
		"  styles: [`\n" +
		"    .badge {\n" +
		"      color: red;\n" +
		"    }\n" +
		"  `],\n" +
		"})\n" +
		"export class BadgeComponent {\n" +
		"  label = 'New';\n" +
		"}\n")

	// Inline templates and styles are template strings in the typescript
	// grammar, not parsed HTML/CSS, so changes inside them resolve to the
	// full decorated component via its export_statement. Line 7 is the
	// {{ label }} interpolation, line 12 a CSS property; both map to the same
	// component, which must be emitted once.
	snippets, err := Declarations("badge.component.ts", source, []int{7, 12}, nil)
	require.NoError(t, err)

	expected := []string{
		"@Component({\n  selector: 'app-badge',\n  template: `\n    <span class=\"badge\">\n      {{ label }}\n    </span>\n  `,\n  styles: [`\n    .badge {\n      color: red;\n    }\n  `],\n})\nexport class BadgeComponent {\n  label = 'New';\n}",
	}

	assert.Equal(t, expected, snippets)

	// Line 17: the label field, unrelated to the template. It must resolve to
	// the class body only, without dragging the inline template and styles in.
	snippets, err = Declarations("badge.component.ts", source, []int{17}, nil)
	require.NoError(t, err)

	expected = []string{
		"class BadgeComponent {\n  label = 'New';\n}",
	}

	assert.Equal(t, expected, snippets)
}

func TestDeclarations_AngularTemplate(t *testing.T) {
	source := []byte(`<div class="counter">
  <h1>Counter</h1>
  <button (click)="increment()">
    Count: {{ count() }}
  </button>
  @if (count() > 0) {
    <p>Positive</p>
  }
</div>
`)

	// Line 4: the {{ count() }} interpolation -> belongs to the enclosing
	// <button> element, event binding syntax included. The plain filename
	// matters: Angular templates have no reliable naming convention, so all
	// html files are parsed with the angular grammar.
	snippets, err := Declarations("example.html", source, []int{4}, nil)
	require.NoError(t, err)

	expected := []string{
		"<button (click)=\"increment()\">\n    Count: {{ count() }}\n  </button>",
	}

	assert.Equal(t, expected, snippets)

	// Line 7: element inside an @if control-flow block. The angular grammar
	// parses the block properly (the plain html grammar truncates the tree at
	// the first control-flow block), so the smallest enclosing element still
	// resolves.
	snippets, err = Declarations("example.html", source, []int{7}, nil)
	require.NoError(t, err)

	expected = []string{
		"<p>Positive</p>",
	}

	assert.Equal(t, expected, snippets)
}

func TestDeclarations_PlainHtml(t *testing.T) {
	source := []byte(`<body>
  <ul class="list">
    <li>one</li>
    <li>two &amp; {{ not angular }}</li>
  </ul>
</body>
`)

	// Non-Angular HTML goes through the angular grammar too; element
	// resolution must be unaffected, even next to text the grammar cannot
	// read as an Angular expression (the handlebars-style interpolation).
	snippets, err := Declarations("page.html", source, []int{3}, nil)
	require.NoError(t, err)

	expected := []string{
		"<li>one</li>",
	}

	assert.Equal(t, expected, snippets)
}

func TestDeclarations_Tsx(t *testing.T) {
	source := []byte(`class Widget {
    render() {
        return <div>Hello</div>;
    }
}
`)

	// Line 3: the JSX return -> belongs to the render() method. The .tsx
	// extension resolves to the distinct "tsx" grammar, which must map to the
	// same structural node types as typescript.
	snippets, err := Declarations("widget.tsx", source, []int{3}, nil)
	require.NoError(t, err)

	expected := []string{
		"render() {\n        return <div>Hello</div>;\n    }",
	}

	assert.Equal(t, expected, snippets)
}

// A declaration whose every line is added is already shown whole in the
// diff, so repeating it as enclosing context only adds tokens (C1: 46% of
// enclosing blocks on 14 of Bark's commits). One with an unchanged line
// still gives context the diff lacks.
func TestDeclarationsForDiff_SkipsWhollyAddedDeclarations(t *testing.T) {
	src := "package app\n" + //    1
		"\n" + //                  2
		"func old() int {\n" + //  3
		"\tx := 1\n" + //          4 added
		"\treturn x\n" + //        5
		"}\n" + //                 6
		"\n" + //                  7 added
		"func added() int {\n" + // 8 added
		"\treturn 2\n" + //        9 added
		"}\n" //                   10 added
	diff := "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n" +
		"@@ -3,3 +3,4 @@\n func old() int {\n+\tx := 1\n \treturn x\n }\n" +
		"@@ -6,0 +7,4 @@\n+\n+func added() int {\n+\treturn 2\n+}\n"
	read := func(context.Context, string) ([]byte, error) { return []byte(src), nil }

	got, err := DeclarationsForDiff(t.Context(), diff, read)
	require.NoError(t, err)

	assert.Contains(t, got.Text, "func old() int {")
	assert.NotContains(t, got.Text, "func added() int {")
}

func TestDeclarationsForDiff_OnlyWhollyAddedDeclarations(t *testing.T) {
	src := "package app\n\nfunc added() int {\n\treturn 2\n}\n"
	diff := "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n" +
		"@@ -1,0 +2,4 @@\n+\n+func added() int {\n+\treturn 2\n+}\n"
	read := func(context.Context, string) ([]byte, error) { return []byte(src), nil }

	got, err := DeclarationsForDiff(t.Context(), diff, read)
	require.NoError(t, err)
	assert.Empty(t, got.Text, "no section header without a declaration to show")
}

// The dry run and BARK_DEBUG report what the line cap leaves out (C4), so
// it can be measured: each dropped declaration's file, name, size and how
// many of the diff's changed lines it holds.
func TestDeclarationsForDiff_ReportsDroppedDeclarations(t *testing.T) {
	// a.go: nine 100-line functions with one changed line each; eight fill
	// the 800-line cap.
	var a, diff strings.Builder
	a.WriteString("package app\n")
	diff.WriteString("diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n")
	line := 2
	for i := range 9 {
		fmt.Fprintf(&a, "\nfunc f%d() {\n", i)
		line += 2
		fmt.Fprintf(&diff, "@@ -%d,0 +%d @@\n+\t_ = 0\n", line, line)
		for range 98 {
			a.WriteString("\t_ = 0\n")
			line++
		}
		a.WriteString("}\n")
		line++
	}
	b := "package app\n" + //            1
		"\n" + //                        2
		"type Config struct {\n" + //    3
		"\tName string\n" + //           4 changed
		"\tPort int\n" + //              5 changed
		"}\n" + //                       6
		"\n" + //                        7
		"func (s *Server) Handle() {\n" + // 8
		"\ts.n++\n" + //                 9 changed
		"}\n" //                         10
	diff.WriteString("diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n" +
		"@@ -4,2 +4,2 @@\n-\tName int\n-\tPort string\n+\tName string\n+\tPort int\n" +
		"@@ -9 +9 @@\n-\ts.n--\n+\ts.n++\n")
	files := map[string]string{"a.go": a.String(), "b.go": b}
	read := func(_ context.Context, path string) ([]byte, error) { return []byte(files[path]), nil }

	got, err := DeclarationsForDiff(t.Context(), diff.String(), read)
	require.NoError(t, err)

	assert.Contains(t, got.Text, "func f7() {")
	assert.NotContains(t, got.Text, "func f8() {")
	assert.NotContains(t, got.Text, "### File: b.go")
	assert.Equal(t, []Dropped{
		{File: "a.go", Name: "f8", Lines: 100, Changed: 1},
		{File: "b.go", Name: "Config", Lines: 4, Changed: 2},
		{File: "b.go", Name: "(*Server).Handle", Lines: 3, Changed: 1},
	}, got.Dropped)
	assert.Equal(t, "3 enclosing declarations (107 lines) over the 800-line cap left out\n"+
		"  a.go f8: 100 lines, 1 changed\n"+
		"  b.go Config: 4 lines, 2 changed\n"+
		"  b.go (*Server).Handle: 3 lines, 1 changed", got.DroppedSummary())
	assert.Empty(t, Context{Text: "x"}.DroppedSummary())
}
