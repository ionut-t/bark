package cmd

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"charm.land/fang/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHelpWidth is the terminal width the help tests render at. fang reads its
// width once per process, so every test uses the same one.
const testHelpWidth = 80

// renderHelp prints a command's --help through fang, as Execute does.
func renderHelp(t *testing.T, args ...string) string {
	t.Helper()
	t.Setenv("__FANG_TEST_WIDTH", strconv.Itoa(testHelpWidth))
	root := newRootCmd(func(string) (config.Config, error) {
		t.Error("help loaded the config")
		return nil, os.ErrInvalid
	})
	wrapFlagUsages(root, testHelpWidth)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs(append(args, "--help"))
	require.NoError(t, fang.Execute(context.Background(), root, fangOptions()...))
	return out.String()
}

func TestHelpFitsTheWidth(t *testing.T) {
	for _, args := range [][]string{nil, {"review"}, {"commit"}, {"pr"}, {"config"}, {"actions"}, {"prices"}} {
		out := renderHelp(t, args...)
		require.Contains(t, out, "FLAGS")
		for line := range strings.SplitSeq(out, "\n") {
			// fang pads the description block, the usage box and the
			// section titles with trailing spaces.
			line = strings.TrimRight(line, " ")
			assert.LessOrEqual(t, ansi.StringWidth(line), testHelpWidth, "bark %v --help: %q", args, line)
		}
	}
}

func TestHelpWrapsUnderTheDescriptionColumn(t *testing.T) {
	out := renderHelp(t, "review")

	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	i := slicesIndexPrefix(lines, "    --approve ")
	require.GreaterOrEqual(t, i, 0, out)
	col := strings.Index(lines[i], "Post")
	require.Positive(t, col)
	// The description goes on below, starting in its own column.
	next := lines[i+1]
	assert.Equal(t, strings.Repeat(" ", col), next[:col], "continuation %q", next)
	assert.NotEqual(t, ' ', next[col], "continuation %q", next)
}

func TestHelpKeepsAcronyms(t *testing.T) {
	out := renderHelp(t, "review")
	assert.NotContains(t, out, "Llm")
}

func slicesIndexPrefix(lines []string, prefix string) int {
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return i
		}
	}
	return -1
}

func TestHelpSpacesFlagsApart(t *testing.T) {
	out := renderHelp(t, "review")

	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	for _, flag := range []string{"    --as ", "    -h --help ", "    --check "} {
		i := slicesIndexPrefix(lines, flag)
		require.GreaterOrEqual(t, i, 0, out)
		assert.Empty(t, lines[i-1], "no blank line before %q", flag)
	}
	// The default stays with its description instead of landing on the
	// blank line.
	i := slicesIndexPrefix(lines, "    --check ")
	for lines[i+1] != "" {
		i++
	}
	assert.True(t, strings.HasSuffix(lines[i], "(true)"), "%q", lines[i])
}

func TestHelpEndsInOneBlankLine(t *testing.T) {
	for _, args := range [][]string{nil, {"review"}, {"add"}, {"config"}, {"prices"}} {
		out := renderHelp(t, args...)
		var lines []string
		for line := range strings.SplitSeq(out, "\n") {
			lines = append(lines, strings.TrimRight(line, " "))
		}
		// out ends in a newline, so the last element is always empty.
		n := len(lines)
		require.Greater(t, n, 3)
		assert.Empty(t, lines[n-2], "bark %v --help", args)
		assert.NotEmpty(t, lines[n-3], "bark %v --help ends in two blank lines", args)
	}
}

func TestHelpSpacesTheVersionFlag(t *testing.T) {
	out := renderHelp(t)
	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	i := slicesIndexPrefix(lines, "    -v --version ")
	require.GreaterOrEqual(t, i, 0, out)
	assert.Empty(t, lines[i-1])
}

func TestHelpWidth(t *testing.T) {
	t.Setenv("__FANG_TEST_WIDTH", "77")
	assert.Equal(t, 77, helpWidth())

	t.Setenv("__FANG_TEST_WIDTH", "wide")
	assert.Equal(t, maxHelpWidth, helpWidth(), "an invalid override is ignored; no terminal means the max")

	t.Setenv("__FANG_TEST_WIDTH", "")
	assert.Equal(t, maxHelpWidth, helpWidth())
}
