package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(name, []byte(content), 0o644))
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "empty dir", want: ""},
		{name: "go", files: map[string]string{"go.mod": ""}, want: "Go"},
		{name: "rust", files: map[string]string{"Cargo.toml": ""}, want: "Rust"},
		{name: "zig", files: map[string]string{"build.zig": ""}, want: "Zig"},
		{name: "angular", files: map[string]string{"angular.json": ""}, want: "Angular"},
		{name: "typescript", files: map[string]string{"tsconfig.json": ""}, want: "TypeScript"},
		{name: "angular wins over typescript", files: map[string]string{"angular.json": "", "tsconfig.json": ""}, want: "Angular"},
		{name: "go wins over python", files: map[string]string{"go.mod": "", "setup.py": ""}, want: "Go"},
		{
			name:  "nx workspace with angular",
			files: map[string]string{"nx.json": "", "package.json": `{"dependencies":{"@angular/core":"^18"}}`},
			want:  "Angular",
		},
		{
			name:  "nx workspace without angular",
			files: map[string]string{"nx.json": "", "package.json": `{"dependencies":{"react":"^18"}}`},
			want:  "",
		},
		{name: "nx workspace without package.json", files: map[string]string{"nx.json": ""}, want: ""},
		{name: "pyproject", files: map[string]string{"pyproject.toml": ""}, want: "Python"},
		{name: "requirements", files: map[string]string{"requirements.txt": ""}, want: "Python"},
		{name: "setup.py", files: map[string]string{"setup.py": ""}, want: "Python"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for name, content := range tt.files {
				write(t, name, content)
			}
			assert.Equal(t, tt.want, detectProjectType())
		})
	}
}

func TestRunWritesWorkflowsOnly(t *testing.T) {
	t.Chdir(t.TempDir())

	require.NoError(t, Run(Options{Workflows: map[string]string{
		"bark.yaml":  "name: bark\n",
		"other.yaml": "name: other\n",
	}}))

	assert.Equal(t, "name: bark\n", read(t, filepath.Join(".github", "workflows", "bark.yaml")))
	assert.Equal(t, "name: other\n", read(t, filepath.Join(".github", "workflows", "other.yaml")))
	_, err := os.Stat(".bark")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunCreatesBarkDir(t *testing.T) {
	reviewer, err := reviewers.GetEmbedded("Linus Torvalds")
	require.NoError(t, err)

	t.Run("unknown project uses default review focus", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, Run(Options{CreateBarkDir: true}))

		assert.Equal(t, reviewer.Prompt, read(t, filepath.Join(".bark", "reviewer.md")))
		assert.Equal(t, defaultReviewInstructions, read(t, filepath.Join(".bark", "review.md")))
		assert.Equal(t, templates.GetDefaultPRInstructions(), read(t, filepath.Join(".bark", "pr.md")))
		_, err := os.Stat(filepath.Join(".github", "workflows"))
		assert.NoError(t, err, "workflows dir is always created")
	})

	t.Run("detected project uses its instructions", func(t *testing.T) {
		t.Chdir(t.TempDir())
		write(t, "go.mod", "module x\n")
		require.NoError(t, Run(Options{CreateBarkDir: true}))

		goInstr, err := instructions.GetEmbedded("Go")
		require.NoError(t, err)
		assert.Equal(t, goInstr.Prompt, read(t, filepath.Join(".bark", "review.md")))
	})

	t.Run("overwrites existing files", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.MkdirAll(".bark", 0o755))
		write(t, filepath.Join(".bark", "pr.md"), "old")
		require.NoError(t, Run(Options{CreateBarkDir: true}))
		assert.Equal(t, templates.GetDefaultPRInstructions(), read(t, filepath.Join(".bark", "pr.md")))
	})
}

func TestRunFailsWhenWorkflowsPathIsAFile(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, ".github", "not a dir")
	assert.Error(t, Run(Options{}))
}
