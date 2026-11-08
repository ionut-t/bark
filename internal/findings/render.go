package findings

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Markdown renders events as a Markdown review. Rendering events one batch
// at a time and joining the results gives the same output as rendering them
// all at once, so a streamed review can be appended to as it arrives.
func Markdown(events []Event) string {
	var b strings.Builder
	for _, e := range events {
		switch {
		case e.Intro != "":
			fmt.Fprintf(&b, "%s\n\n", e.Intro)
		case e.Finding != nil:
			writeFinding(&b, *e.Finding, "")
		case e.Verdict != "":
			fmt.Fprintf(&b, "## Verdict\n\n%s\n\n", e.Verdict)
		default:
			b.WriteString(e.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// writeFinding writes f as Markdown. open names a prose field (title, body
// or remark) whose value is still streaming in: writing stops right after
// that value, so the text written is a prefix of the finished finding's.
func writeFinding(b *strings.Builder, f Finding, open string) {
	heading := strings.ToUpper(string(f.Severity))
	if f.Category != "" {
		heading += " · " + f.Category
	}
	fmt.Fprintf(b, "### [%s] %s", heading, f.Title)
	if open == "title" {
		return
	}
	b.WriteString("\n\n")

	if loc := location(f); loc != "" {
		fmt.Fprintf(b, "`%s`\n\n", loc)
	}

	if f.Body != "" {
		b.WriteString(f.Body)
		if open == "body" {
			return
		}
		b.WriteString("\n\n")
	}

	if f.Suggestion != "" {
		fence := "```"
		for strings.Contains(f.Suggestion, fence) {
			fence += "`"
		}
		fmt.Fprintf(b, "%s%s\n%s\n%s\n\n", fence, language(f.File), strings.TrimRight(f.Suggestion, "\n"), fence)
	}

	if f.Remark != "" {
		b.WriteString("> _" + f.Remark)
		if open == "remark" {
			return
		}
		b.WriteString("_\n\n")
	}
}

func location(f Finding) string {
	switch {
	case f.File == "":
		return ""
	case f.Line <= 0:
		return f.File
	case f.EndLine > f.Line:
		return fmt.Sprintf("%s:%d-%d", f.File, f.Line, f.EndLine)
	default:
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
}

var languages = map[string]string{
	".go":    "go",
	".py":    "python",
	".js":    "javascript",
	".jsx":   "jsx",
	".ts":    "typescript",
	".tsx":   "tsx",
	".rs":    "rust",
	".java":  "java",
	".kt":    "kotlin",
	".swift": "swift",
	".rb":    "ruby",
	".php":   "php",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".hpp":   "cpp",
	".cs":    "csharp",
	".sh":    "bash",
	".sql":   "sql",
	".yaml":  "yaml",
	".yml":   "yaml",
	".json":  "json",
	".toml":  "toml",
	".md":    "markdown",
	".html":  "html",
	".css":   "css",
}

// language is the code fence language for file, or "text" if unknown.
func language(file string) string {
	if lang, ok := languages[strings.ToLower(filepath.Ext(file))]; ok {
		return lang
	}
	return "text"
}
