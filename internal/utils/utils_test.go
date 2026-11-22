package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormaliseCodeFences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic code fence normalization",
			input:    "Some text\n    ```go\n    fmt.Println(\"hello\")\n    ```",
			expected: "Some text\n```go\n    fmt.Println(\"hello\")\n```",
		},
		{
			name:     "diff markers inside code block",
			input:    "```diff\n    +func main() {\n    -func old() {\n```",
			expected: "```diff\n+func main() {\n-func old() {\n```",
		},
		{
			name:     "line ending with +``` should be split",
			input:    "Some code here+```\nMore text",
			expected: "Some code here\n+```\nMore text",
		},
		{
			name:     "line ending with -``` should be split",
			input:    "Some code here-```\nMore text",
			expected: "Some code here\n-```\nMore text",
		},
		{
			name:     "line ending with +``` and whitespace should be split",
			input:    "Some code here+```  \nMore text",
			expected: "Some code here\n+```\nMore text",
		},
		{
			name:     "code block with diff showing added code fence",
			input:    "```diff\n+```go\n+print(\"hello\")\n+```\n```",
			expected: "```diff\n+```go\n+print(\"hello\")\n+```\n```",
		},
		{
			name:     "multiple code blocks",
			input:    "First block:\n    ```js\n    console.log(\"test\")\n    ```\nSecond block:\n    ```py\n    print(\"test\")\n    ```",
			expected: "First block:\n```js\n    console.log(\"test\")\n```\nSecond block:\n```py\n    print(\"test\")\n```",
		},
		{
			name:     "diff with indented lines inside code block",
			input:    "```diff\n    +    if condition:\n    +        print(\"indented\")\n    -    if old:\n```",
			expected: "```diff\n+    if condition:\n+        print(\"indented\")\n-    if old:\n```",
		},
		{
			name:     "preserve non-diff lines inside code block",
			input:    "```go\n    func hello():\n        print(\"world\")\n```",
			expected: "```go\n    func hello():\n        print(\"world\")\n```",
		},
		{
			name:     "empty content",
			input:    "",
			expected: "",
		},
		{
			name:     "no code fences",
			input:    "Just some regular text\nwith multiple lines\nand no code blocks",
			expected: "Just some regular text\nwith multiple lines\nand no code blocks",
		},
		{
			name:     "code fence with language specifier ending with +```",
			input:    "text+```go",
			expected: "text\n+```go",
		},
		{
			name:     "realistic streaming scenario",
			input:    "Here's the fix:\n    ```go\n    +func NewFunction() {\n    +    return \"new\"\n    +}\ncontent+```",
			expected: "Here's the fix:\n```go\n+func NewFunction() {\n+    return \"new\"\n+}\ncontent\n+```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormaliseCodeFences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormaliseCodeFences_PreservesIndentation(t *testing.T) {
	input := "```go\n    func function() {\n        if true {\n            print(\"deeply nested\")\n```"
	expected := "```go\n    func function() {\n        if true {\n            print(\"deeply nested\")\n```"

	result := NormaliseCodeFences(input)
	assert.Equal(t, expected, result)
}

func TestNormaliseCodeFences_DiffWithTabs(t *testing.T) {
	input := "```diff\n    +\tfunc() {\n    +\t\treturn\n    +\t}\n```"
	expected := "```diff\n+\tfunc() {\n+\t\treturn\n+\t}\n```"

	result := NormaliseCodeFences(input)
	assert.Equal(t, expected, result)
}

func TestRemoveCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "no fences", input: "  feat: add thing  ", want: "feat: add thing"},
		{name: "plain fence", input: "```\nfeat: add thing\n```", want: "feat: add thing"},
		{name: "language fence", input: "```markdown\n## Summary\n```", want: "## Summary"},
		{name: "text around fence", input: "Here:\n```\nfix: bug\n```\nDone.", want: "Here:\nfix: bug\n\nDone."},
		{name: "several fences", input: "```\none\n```\n```go\ntwo\n```", want: "one\n\ntwo"},
		{name: "unclosed fence is left alone", input: "```\nfeat: x", want: "```\nfeat: x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RemoveCodeFences(tt.input))
		})
	}
}

func TestOpenInEditorCmd(t *testing.T) {
	cmd, err := OpenInEditorCmd("code --wait", "/tmp/file.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"code", "--wait", "/tmp/file.md"}, cmd.Args)

	cmd, err = OpenInEditorCmd("vim", "a.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"vim", "a.md"}, cmd.Args)

	_, err = OpenInEditorCmd("   ", "a.md")
	assert.EqualError(t, err, "editor is not configured")
}

func TestOpenEditor(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out.md")
	editor := filepath.Join(t.TempDir(), "editor.sh")
	require.NoError(t, os.WriteFile(editor, []byte("#!/bin/sh\nprintf edited > \"$1\"\n"), 0o755))

	require.NoError(t, OpenEditor(editor, target))
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "edited", string(content))

	assert.Error(t, OpenEditor("false", target), "a failing editor is reported")
	assert.Error(t, OpenEditor("", target))
}

func TestReadLocalOverrideAndGetInstructions(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "review.md")
	require.NoError(t, os.WriteFile(full, []byte("Local rules."), 0o644))
	empty := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(empty, nil, 0o644))
	missing := filepath.Join(dir, "missing.md")

	got, err := ReadLocalOverride(full)
	require.NoError(t, err)
	assert.Equal(t, "Local rules.", got)

	for _, path := range []string{empty, missing} {
		got, err = ReadLocalOverride(path)
		require.NoError(t, err)
		assert.Empty(t, got)

		got, err = GetInstructions(path, "fallback")
		require.NoError(t, err)
		assert.Equal(t, "fallback", got)
	}

	got, err = GetInstructions(full, "fallback")
	require.NoError(t, err)
	assert.Equal(t, "Local rules.", got)

	// A directory can't be read as a file: that's an error, not a fallback.
	_, err = ReadLocalOverride(dir)
	assert.ErrorContains(t, err, "could not read")
	_, err = GetInstructions(dir, "fallback")
	assert.Error(t, err)
}

func TestDispatchMsg(t *testing.T) {
	type ping struct{ n int }
	assert.Equal(t, ping{n: 3}, DispatchMsg(ping{n: 3})())
}

func TestDispatchClearMsg(t *testing.T) {
	assert.Equal(t, ClearMsg{}, DispatchClearMsg(time.Millisecond)())
}
