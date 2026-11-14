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
