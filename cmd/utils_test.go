package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateDirs points Bark's directories at a fresh temporary tree and
// returns the config directory.
func isolateDirs(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("XDG defaults differ on Windows")
	}
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	return filepath.Join(root, "config", "bark")
}

// scriptEditor returns an editor that writes content into the file it's
// given.
func scriptEditor(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "editor.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nprintf '"+content+"' > \"$1\"\n"), 0o755))
	return path
}

func withFlags(t *testing.T, cmd *cobra.Command, flags ...string) *cobra.Command {
	t.Helper()
	require.NoError(t, cmd.ParseFlags(flags))
	return cmd
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestPrintError(t *testing.T) {
	var w bytes.Buffer
	printError(&w, errors.New("boom"))
	assert.Contains(t, w.String(), "Error: boom")
	assert.True(t, strings.HasSuffix(w.String(), "\n"))
}

func TestIsPlainMode(t *testing.T) {
	cmd := &cobra.Command{}
	addPlainFlag(cmd)
	require.NoError(t, cmd.ParseFlags([]string{"--plain"}))
	assert.True(t, isPlainMode(cmd))

	// Tests don't run with a terminal on stdout, so that alone means plain.
	cmd = &cobra.Command{}
	addPlainFlag(cmd)
	assert.True(t, isPlainMode(cmd))
}

func TestSyncBuffer(t *testing.T) {
	var b syncBuffer
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			n, err := b.Write([]byte("ab"))
			assert.NoError(t, err)
			assert.Equal(t, 2, n)
		})
	}
	wg.Wait()
	assert.Len(t, b.Bytes(), 100)
}
