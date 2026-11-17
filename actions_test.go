package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// compositeAction is the part of an action.yaml these tests read.
type compositeAction struct {
	Runs struct {
		Steps []struct {
			Name string `yaml:"name"`
			Run  string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"runs"`
}

// TestActionScriptsHaveNoExpressions checks that no run script in the
// published composite actions contains a ${{ }} expression. GitHub pastes
// an expression's value into the script before the shell runs it, so an
// input holding a quote breaks the step and one holding $(...) runs as a
// command. Values reach scripts through env: instead.
func TestActionScriptsHaveNoExpressions(t *testing.T) {
	files, err := filepath.Glob(".github/actions/*/action.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)

			var action compositeAction
			require.NoError(t, yaml.Unmarshal(data, &action))
			require.NotEmpty(t, action.Runs.Steps)

			for _, step := range action.Runs.Steps {
				for i, line := range strings.Split(step.Run, "\n") {
					assert.NotContains(t, line, "${{", "step %q, script line %d", step.Name, i+1)
				}
			}
		})
	}
}
