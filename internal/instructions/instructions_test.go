package instructions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "extra.md")
	require.NoError(t, os.WriteFile(file, []byte("From a file.\n"), 0o644))

	saved := []Instruction{{Name: "security.md", Prompt: "Focus on security.\n"}}

	tests := []struct {
		name   string
		values []string
		saved  []Instruction
		want   string
	}{
		{name: "none", values: nil, want: ""},
		{name: "raw text", values: []string{"Be brief."}, want: "Be brief."},
		{name: "file", values: []string{file}, want: "From a file."},
		{name: "saved instruction", values: []string{"security"}, saved: saved, want: "Focus on security."},
		{
			name:   "several are joined in order",
			values: []string{"security", file, "Be brief, please."},
			saved:  saved,
			want:   "Focus on security.\n\nFrom a file.\n\nBe brief, please.",
		},
		{name: "saved names are not looked up without a list", values: []string{"security"}, want: "security"},
		{name: "blank values are skipped", values: []string{"  ", "Be brief."}, want: "Be brief."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.values, tt.saved)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigAndGet(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, Config(storage, false))

	list, err := Get(storage)
	require.NoError(t, err)

	names := make([]string, 0, len(list))
	for _, in := range list {
		names = append(names, in.Name)
		assert.NotEmpty(t, in.Prompt, "instruction %q has an empty prompt", in.Name)
	}
	assert.ElementsMatch(t, []string{"Angular", "Go", "Python", "Rust", "TypeScript", "Zig"}, names)
}

func TestGetMissingStorage(t *testing.T) {
	_, err := Get(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

func TestFind(t *testing.T) {
	list := []Instruction{{Name: "Go", Prompt: "go"}, {Name: "Rust", Prompt: "rust"}}

	got, err := Find("rust", list)
	require.NoError(t, err)
	assert.Equal(t, "rust", got.Prompt)

	_, err = Find("zig", list)
	assert.Error(t, err)
}

func TestGetEmbedded(t *testing.T) {
	got, err := GetEmbedded("Go")
	require.NoError(t, err)
	assert.Equal(t, "Go", got.Name)
	assert.NotEmpty(t, got.Prompt)

	_, err = GetEmbedded("COBOL")
	assert.EqualError(t, err, "instruction 'COBOL' not found")
}

func TestPathDeleteAndRemoveDir(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, Config(storage, false))

	path, err := GetPath(storage, "Go")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(storage, assetDirName, "Go.md"), path)

	require.NoError(t, Delete(storage, "Go"))
	_, err = GetPath(storage, "Go")
	assert.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, RemoveDir(storage))
	_, err = os.Stat(filepath.Join(storage, assetDirName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestAddWritesEditorOutput(t *testing.T) {
	storage := t.TempDir()
	editor := filepath.Join(t.TempDir(), "editor.sh")
	require.NoError(t, os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'Custom rules.' > \"$1\"\n"), 0o755))

	require.NoError(t, Add(storage, "Custom", editor))

	list, err := Get(storage)
	require.NoError(t, err)
	assert.Equal(t, []Instruction{{Name: "Custom", Prompt: "Custom rules."}}, list)
}
