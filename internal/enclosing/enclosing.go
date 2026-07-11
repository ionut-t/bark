package enclosing

import (
	"context"
	"fmt"
	"path/filepath"
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

// Declarations parses the file source and returns the enclosing definitions for the modified lines.
func Declarations(filePath string, source []byte, modifiedLines []int) ([]string, error) {
	if len(modifiedLines) == 0 {
		return nil, nil
	}

	entry := grammars.DetectLanguage(filePath)
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

	var snippets []string
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
		if structuralNode != nil {
			start := structuralNode.StartByte()
			end := structuralNode.EndByte()
			if start < end && end <= uint32(len(source)) {
				snippet := string(source[start:end])
				snippet = strings.TrimSpace(snippet)
				if snippet != "" && !seen[snippet] {
					seen[snippet] = true
					snippets = append(snippets, snippet)
				}
			}
		}
	}

	return snippets, nil
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
// spaces. Returns "" for `+++ /dev/null` (deleted file) or unparsable lines.
func newFilePath(line string) string {
	p := strings.TrimPrefix(line, "+++ ")
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

// DeclarationsForDiff parses a diff, reads the files at the specified ref,
// and extracts the enclosing declarations for all modified lines.
func DeclarationsForDiff(ctx context.Context, diffText string, ref string) (string, error) {
	modifiedMap := ModifiedLinesFromDiff(diffText)
	if len(modifiedMap) == 0 {
		return "", nil
	}

	// Iterate in sorted order so the generated prompt section is deterministic.
	files := make([]string, 0, len(modifiedMap))
	for file := range modifiedMap {
		files = append(files, file)
	}
	sort.Strings(files)

	var sb strings.Builder

	for _, file := range files {
		// Get file content at ref
		content, err := git.GetFileContent(ctx, ref, file)
		if err != nil {
			// Gracefully skip files we can't read (e.g. deleted files)
			continue
		}

		snippets, err := Declarations(file, content, modifiedMap[file])
		if err != nil || len(snippets) == 0 {
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

	return sb.String(), nil
}
