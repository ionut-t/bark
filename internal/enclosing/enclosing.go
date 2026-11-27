package enclosing

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// isStructuralNode checks if the given node type represents an enclosing declaration block in that language.
func isStructuralNode(langName string, nodeType string) bool {
	switch langName {
	case "go":
		// Only top-level declarations: function/method bodies and named types.
		// var/const and bare struct/interface types are intentionally excluded so
		// that a changed local `var x T` resolves up to its enclosing function
		// rather than emitting a useless one-line fragment.
		return nodeType == "function_declaration" ||
			nodeType == "method_declaration" ||
			nodeType == "type_declaration"
	case "rust":
		return nodeType == "function_item" ||
			nodeType == "struct_item" ||
			nodeType == "impl_item" ||
			nodeType == "trait_item" ||
			nodeType == "const_item" ||
			nodeType == "mod_item"
	case "python":
		return nodeType == "function_definition" ||
			nodeType == "class_definition"
	case "typescript", "tsx", "javascript":
		// export_statement is included so that changes inside a decorator
		// (e.g. Angular's @Component metadata, including inline templates and
		// styles) resolve to the full decorated class: decorators attach to
		// the export_statement, not the class_declaration, so without it
		// they'd resolve to nothing. Changes inside the class body still stop
		// at the inner class_declaration/method_definition, so they don't
		// drag the decorator into the snippet.
		return nodeType == "function_declaration" ||
			nodeType == "method_definition" ||
			nodeType == "class_declaration" ||
			nodeType == "arrow_function" ||
			nodeType == "lexical_declaration" || // let / const
			nodeType == "variable_declaration" || // var
			nodeType == "export_statement"
	case "html":
		return nodeType == "element"
	case "angular":
		// Angular templates: elements plus @if/@for/@switch/@defer
		// control-flow blocks. Elements match first for changes inside them;
		// the block statements catch changes on the block syntax itself
		// (e.g. the @if condition line).
		return nodeType == "element" ||
			nodeType == "if_statement" ||
			nodeType == "for_statement" ||
			nodeType == "switch_statement" ||
			nodeType == "defer_statement"
	case "css":
		return nodeType == "rule_set"
	default:
		// Fallback for substring matching for any language
		nt := strings.ToLower(nodeType)
		return strings.Contains(nt, "definition") ||
			strings.Contains(nt, "declaration") ||
			strings.Contains(nt, "specifier") ||
			strings.Contains(nt, "element") ||
			strings.Contains(nt, "rule_set") ||
			strings.Contains(nt, "item")
	}
}

// detectLanguage resolves the grammar entry for a file. Files that detect as
// html are parsed with the angular grammar instead: it is a superset of html
// that additionally understands Angular template syntax (@if/@for blocks,
// event bindings, interpolation). Angular templates are plain .html files
// with no reliable naming convention, and the html grammar truncates its
// parse tree at the first control-flow block; on plain HTML both grammars
// produce the same element structure, so this is safe for non-Angular files.
func detectLanguage(filePath string) *grammars.LangEntry {
	entry := grammars.DetectLanguage(filePath)
	if entry != nil && entry.Name == "html" {
		if angular := grammars.DetectLanguageByName("angular"); angular != nil {
			return angular
		}
	}
	return entry
}

// Declarations parses the file source and returns the enclosing definitions
// for the modified lines. A definition whose every line is in added (1-based
// line numbers of the diff's added lines) is left out: the diff already shows
// it whole. A nil added leaves nothing out.
func Declarations(filePath string, source []byte, modifiedLines []int, added map[int]bool) ([]string, error) {
	blocks, err := declarations(filePath, source, modifiedLines, added)
	snippets := make([]string, 0, len(blocks))
	for _, b := range blocks {
		snippets = append(snippets, b.text)
	}
	if len(snippets) == 0 {
		snippets = nil
	}
	return snippets, err
}

// block is one enclosing declaration: its trimmed source, its name (see
// declName) and how many of the diff's modified lines it holds.
type block struct {
	text    string
	name    string
	changed int
}

// lines is the number of lines the block takes in the prompt.
func (b block) lines() int {
	return strings.Count(b.text, "\n") + 1
}

// declarations returns the enclosing declarations of modifiedLines, as
// Declarations does, with their names and changed-line counts.
func declarations(filePath string, source []byte, modifiedLines []int, added map[int]bool) ([]block, error) {
	if len(modifiedLines) == 0 {
		return nil, nil
	}

	entry := detectLanguage(filePath)
	if entry == nil {
		return nil, nil // Unsupported language, return nil (graceful skip)
	}
	lang := entry.Language()
	if lang == nil {
		return nil, nil
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, err
	}

	root := tree.RootNode()
	if root == nil {
		return nil, nil
	}

	sourceLines := strings.Split(string(source), "\n")

	var blocks []block
	seen := make(map[string]bool)

	for _, line := range modifiedLines {
		// Tree-sitter rows are 0-indexed.
		row := uint32(line - 1)

		// Find the first non-whitespace column in that line
		col := firstNonWhitespaceColumn(sourceLines, row)
		startPoint := gotreesitter.Point{Row: row, Column: col}

		node := root.DescendantForPointRange(startPoint, startPoint)
		if node == nil {
			continue
		}

		// Walk up to find an enclosing structural node
		curr := node
		var structuralNode *gotreesitter.Node
		for curr != nil {
			if isStructuralNode(entry.Name, curr.Type(lang)) {
				structuralNode = curr
				break
			}
			curr = curr.Parent()
		}

		// If we found a structural node, extract its source
		if structuralNode != nil && !allAdded(structuralNode, added) {
			start := structuralNode.StartByte()
			end := structuralNode.EndByte()
			if start < end && end <= uint32(len(source)) {
				snippet := string(source[start:end])
				snippet = strings.TrimSpace(snippet)
				if snippet != "" && !seen[snippet] {
					seen[snippet] = true
					blocks = append(blocks, block{
						text:    snippet,
						name:    declName(structuralNode, lang, source),
						changed: countWithin(structuralNode, modifiedLines),
					})
				}
			}
		}
	}

	return blocks, nil
}

// maxNameLen caps a declaration name taken from its first line.
const maxNameLen = 60

// declName names a declaration for reports: its name field ("Config",
// "(*Server).Handle" for a Go method), the name of its first named child
// that has one (a Go type_declaration's type_spec, an exported function,
// a const's declarator), or else its first line.
func declName(n *gotreesitter.Node, lang *gotreesitter.Language, source []byte) string {
	name := n.ChildByFieldName("name", lang)
	for i := 0; name == nil && i < n.NamedChildCount(); i++ {
		name = n.NamedChild(i).ChildByFieldName("name", lang)
	}
	if name == nil {
		first, _, _ := strings.Cut(strings.TrimSpace(n.Text(source)), "\n")
		return truncateName(first)
	}
	text := name.Text(source)
	if recv := n.ChildByFieldName("receiver", lang); recv != nil {
		for i := range recv.NamedChildCount() {
			if t := recv.NamedChild(i).ChildByFieldName("type", lang); t != nil {
				return "(" + t.Text(source) + ")." + text
			}
		}
	}
	return text
}

// truncateName cuts s to maxNameLen characters (runes, so never inside a
// multi-byte one), marking the cut with "…".
func truncateName(s string) string {
	if r := []rune(s); len(r) > maxNameLen {
		return string(r[:maxNameLen]) + "…"
	}
	return s
}

// countWithin counts the distinct 1-based lines that fall within n.
func countWithin(n *gotreesitter.Node, lines []int) int {
	first, last := int(n.StartPoint().Row)+1, int(n.EndPoint().Row)+1
	within := make(map[int]bool)
	for _, l := range lines {
		if l >= first && l <= last {
			within[l] = true
		}
	}
	return len(within)
}

// allAdded reports whether every line of n is an added line.
func allAdded(n *gotreesitter.Node, added map[int]bool) bool {
	if len(added) == 0 {
		return false
	}
	start, end := n.StartPoint(), n.EndPoint()
	last := end.Row
	if end.Column == 0 && last > start.Row {
		last-- // n ends at a line break, so its last line is the one before
	}
	for row := start.Row; row <= last; row++ {
		if !added[int(row)+1] {
			return false
		}
	}
	return true
}

// firstNonWhitespaceColumn returns the column index of the first non-whitespace character in the line.
func firstNonWhitespaceColumn(lines []string, row uint32) uint32 {
	if row >= uint32(len(lines)) {
		return 0
	}
	line := lines[row]
	for col, char := range line {
		if char != ' ' && char != '\t' && char != '\r' {
			return uint32(col)
		}
	}
	return 0
}

// newFilePath extracts the new-side path from a `+++ ` diff header line.
// It handles git-quoted paths (`+++ "b/caf\303\251.go"`) and paths containing
// spaces, which git ends with a tab. Returns "" for `+++ /dev/null` (deleted
// file) or unparsable lines.
func newFilePath(line string) string {
	p := strings.TrimSuffix(strings.TrimPrefix(line, "+++ "), "\t")
	if p == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(p, "\"") {
		unquoted, err := strconv.Unquote(p)
		if err != nil {
			return ""
		}
		p = unquoted
	}
	return strings.TrimPrefix(p, "b/")
}

// ModifiedLinesFromDiff parses a unified diff and returns a map of filename -> modified line numbers.
func ModifiedLinesFromDiff(diffText string) map[string][]int {
	result := make(map[string][]int)
	lines := strings.Split(diffText, "\n")

	var currentFile string
	currentLine := -1
	skipFile := false

	for _, line := range lines {
		// Reset per-file state on every file header, whatever its exact shape
		// (git quotes non-ASCII paths: `diff --git "a/café.go" "b/café.go"`),
		// so a previous file's state can never leak into the next one. The
		// path itself is taken from the `+++ ` line below, which is easier to
		// parse robustly.
		if strings.HasPrefix(line, "diff --git ") {
			currentFile = ""
			skipFile = false
			currentLine = -1
			continue
		}

		// Added/deleted files are already shown in full in the diff itself, so
		// extracting their enclosing declarations would only duplicate content.
		if strings.HasPrefix(line, "new file mode ") || strings.HasPrefix(line, "deleted file mode ") {
			skipFile = true
			continue
		}

		if skipFile {
			continue
		}

		// The `+++ ` header names the new-side file. It only appears while
		// currentLine == -1, so it can never be miscounted as hunk content.
		if currentLine == -1 && strings.HasPrefix(line, "+++ ") {
			currentFile = newFilePath(line)
			if currentFile == "" {
				skipFile = true
			}
			continue
		}

		if strings.HasPrefix(line, "@@ ") {
			// Parse hunk header: "@@ -12,5 +12,12 @@"
			// We look for the "+newStart" part.
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				newPart := strings.TrimPrefix(parts[2], "+") // e.g. "12,12" or "12"
				before, _, _ := strings.Cut(newPart, ",")
				if startLine, err := strconv.Atoi(before); err == nil {
					currentLine = startLine - 1 // We'll increment when processing lines
				}
			}
			continue
		}

		// If we are inside a file and a hunk
		if currentFile != "" && currentLine >= 0 {
			if strings.HasPrefix(line, "+") {
				currentLine++
				result[currentFile] = append(result[currentFile], currentLine)
			} else if strings.HasPrefix(line, " ") {
				currentLine++
			} else if strings.HasPrefix(line, "-") {
				// Deletions have no new-side line, so record the line right
				// after the removed block as contextually modified. Past-EOF
				// entries (trailing deletions) are harmless: tree-sitter
				// returns nil for out-of-range points and Declarations skips
				// them. Consecutive deletions all map to the same line;
				// record it once.
				if last := result[currentFile]; len(last) == 0 || last[len(last)-1] != currentLine+1 {
					result[currentFile] = append(result[currentFile], currentLine+1)
				}
			}
		}
	}

	return result
}

// maxEnclosingLines caps the enclosing declarations sent for one diff.
// Declarations with the most changed lines per line are taken first (see
// keepDensest); one that doesn't fit is left out, and reported in
// Context.Dropped.
const maxEnclosingLines = 800

// Context is the context sections for a review's diff, and the enclosing
// declarations the line cap left out of them.
type Context struct {
	Text    string
	Dropped []Dropped
}

// Dropped is an enclosing declaration the line cap left out of a review.
type Dropped struct {
	File string
	// Name is the declaration's name, or its first line if it has none.
	Name  string
	Lines int
	// Changed counts the diff's changed lines inside the declaration
	// (a deletion counts as the line after it).
	Changed int
}

// DroppedSummary describes what the line cap left out, one line per
// declaration after a total; "" if nothing.
func (c Context) DroppedSummary() string {
	if len(c.Dropped) == 0 {
		return ""
	}
	lines := 0
	for _, d := range c.Dropped {
		lines += d.Lines
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d enclosing declarations (%d lines) over the %d-line cap left out", len(c.Dropped), lines, maxEnclosingLines)
	for _, d := range c.Dropped {
		fmt.Fprintf(&sb, "\n  %s %s: %d lines, %d changed", d.File, d.Name, d.Lines, d.Changed)
	}
	return sb.String()
}

// reviewRelatedLines caps the related code a review is shown. Measured on
// Bark's own pull requests, 600 lines plus enclosing declarations added
// about 22k input tokens per review.
const reviewRelatedLines = 400

// ForReview returns the context sections for a review's diff: the enclosing
// declarations of the changed lines, then related code (see Related). Files
// are read from the version whose line numbers the diff uses (see
// git.ReviewDiff.NewSideFile). A diff without a source gets none.
func ForReview(ctx context.Context, r git.ReviewDiff) (Context, error) {
	c, err := DeclarationsForDiff(ctx, r.Diff, r.NewSideFile)
	if err != nil {
		return Context{}, err
	}
	if rel := FormatRelated(Related(ctx, r.Diff, r, RelatedOptions{MaxLines: reviewRelatedLines})); rel != "" {
		c.Text += "\n" + rel + "\n"
	}
	return c, nil
}

// DeclarationsForDiff parses a diff, reads each changed file with read, and
// extracts the enclosing declarations for all modified lines. read must
// return the new side of the diff, so that its line numbers match.
func DeclarationsForDiff(ctx context.Context, diffText string, read func(ctx context.Context, path string) ([]byte, error)) (Context, error) {
	modifiedMap := ModifiedLinesFromDiff(diffText)
	if len(modifiedMap) == 0 {
		return Context{}, nil
	}
	added := addedLines(diffText)

	// Iterate in sorted order so the generated prompt section is deterministic.
	files := make([]string, 0, len(modifiedMap))
	for file := range modifiedMap {
		files = append(files, file)
	}
	sort.Strings(files)

	perFile := make([][]block, len(files))
	for i, file := range files {
		content, err := read(ctx, file)
		if err != nil {
			// Gracefully skip files we can't read (e.g. deleted files)
			continue
		}
		perFile[i], _ = declarations(file, content, modifiedMap[file], added[file])
	}
	kept := keepDensest(perFile, maxEnclosingLines)

	var sb strings.Builder
	var dropped []Dropped
	for i, file := range files {
		var snippets []string
		for j, b := range perFile[i] {
			if kept[i][j] {
				snippets = append(snippets, b.text)
			} else {
				dropped = append(dropped, Dropped{File: file, Name: b.name, Lines: b.lines(), Changed: b.changed})
			}
		}
		if len(snippets) == 0 {
			continue
		}

		if sb.Len() == 0 {
			sb.WriteString("\n## Enclosing Code Context\n")
			sb.WriteString("_Read-only reference showing how the changed code is defined and used. This is NOT under review — only the diff below is. Use it to judge correctness in context (signatures, types, call sites, surrounding control flow) and to avoid false positives about seemingly undefined or unused symbols._\n")
		}

		fmt.Fprintf(&sb, "\n### File: %s\n", file)
		langClass := "text"
		if entry := grammars.DetectLanguage(file); entry != nil {
			langClass = entry.Name
		} else if ext := strings.TrimPrefix(filepath.Ext(file), "."); ext != "" {
			langClass = ext
		}

		for _, snippet := range snippets {
			fmt.Fprintf(&sb, "```%s\n%s\n```\n", langClass, snippet)
		}
	}

	return Context{Text: sb.String(), Dropped: dropped}, nil
}

// keepDensest reports which of perFile's blocks fit in budget lines. Blocks
// with the most changed lines per line go first (ties: the smaller block,
// then diff order), so a large declaration with one changed line can't
// crowd out small ones that are mostly changed. Measured on Bark's four
// diffs over the cap (C4): 82% of changed lines kept, against 51% in path
// order, for the same lines.
func keepDensest(perFile [][]block, budget int) [][]bool {
	type ref struct{ file, i, lines, changed int }
	var refs []ref
	kept := make([][]bool, len(perFile))
	for f, blocks := range perFile {
		kept[f] = make([]bool, len(blocks))
		for i, b := range blocks {
			refs = append(refs, ref{f, i, b.lines(), b.changed})
		}
	}
	slices.SortStableFunc(refs, func(a, b ref) int {
		// a.changed/a.lines vs b.changed/b.lines, without division.
		if c := cmp.Compare(b.changed*a.lines, a.changed*b.lines); c != 0 {
			return c
		}
		return cmp.Compare(a.lines, b.lines)
	})
	for _, r := range refs {
		if r.lines <= budget {
			budget -= r.lines
			kept[r.file][r.i] = true
		}
	}
	return kept
}
