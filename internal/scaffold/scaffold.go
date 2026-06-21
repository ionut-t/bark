package scaffold

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
)

const defaultReviewInstructions = `
## Review Focus

- **Correctness**: Logic errors, edge cases, and potential bugs
- **Security**: Vulnerabilities and unsafe practices
- **Maintainability**: Overly complex or brittle code
- **Performance**: Obvious inefficiencies worth addressing
- **Conventions**: Language idioms and project-specific patterns

Be specific and actionable. Explain why a change is needed, not just what to change.
`

// Options defines what the scaffold operation will write to disk.
type Options struct {
	// Workflows maps filename to content for .github/workflows/.
	Workflows map[string]string
	// CreateBarkDir controls whether to create .bark/ with default instruction files.
	CreateBarkDir bool
}

// Run writes the CI scaffold files to disk in the current working directory.
func Run(opts Options) error {
	if err := os.MkdirAll(filepath.Join(".github", "workflows"), 0o755); err != nil {
		return err
	}

	for filename, content := range opts.Workflows {
		if err := os.WriteFile(filepath.Join(".github", "workflows", filename), []byte(content), 0o644); err != nil {
			return err
		}
	}

	if !opts.CreateBarkDir {
		return nil
	}

	if err := os.MkdirAll(".bark", 0o755); err != nil {
		return err
	}

	reviewer, err := reviewers.GetEmbedded("Linus Torvalds")
	if err != nil {
		return err
	}

	reviewContent := defaultReviewInstructions
	if projectType := detectProjectType(); projectType != "" {
		if instr, err := instructions.GetEmbedded(projectType); err == nil {
			reviewContent = instr.Prompt
		}
	}

	barkFiles := map[string]string{
		"reviewer.md": reviewer.Prompt,
		"review.md":   reviewContent,
		"pr.md":       templates.GetDefaultPRInstructions(),
	}
	for filename, content := range barkFiles {
		if err := os.WriteFile(filepath.Join(".bark", filename), []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// detectProjectType returns the detected project type based on well-known files
// in the current working directory, or an empty string if unrecognised.
func detectProjectType() string {
	for _, c := range []struct {
		file    string
		project string
	}{
		{"go.mod", "Go"},
		{"Cargo.toml", "Rust"},
		{"build.zig", "Zig"},
		{"angular.json", "Angular"},
		{"tsconfig.json", "TypeScript"},
	} {
		if _, err := os.Stat(c.file); err == nil {
			return c.project
		}
	}

	if _, err := os.Stat("nx.json"); err == nil {
		if content, err := os.ReadFile("package.json"); err == nil {
			if strings.Contains(string(content), `"@angular/core"`) {
				return "Angular"
			}
		}
	}

	for _, f := range []string{"pyproject.toml", "requirements.txt", "setup.py"} {
		if _, err := os.Stat(f); err == nil {
			return "Python"
		}
	}

	return ""
}
