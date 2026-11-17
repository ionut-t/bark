package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// step is the part of an action or workflow step these tests read.
type step struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// compositeAction is the part of an action.yaml these tests read.
type compositeAction struct {
	Runs struct {
		Steps []step `yaml:"steps"`
	} `yaml:"runs"`
}

// workflow is the part of a workflow file these tests read.
type workflow struct {
	Jobs map[string]struct {
		Steps []step `yaml:"steps"`
	} `yaml:"jobs"`
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

// scriptSteps returns the steps with a run script in the published
// composite actions and this repo's workflows, keyed by file.
func scriptSteps(t *testing.T) map[string][]step {
	t.Helper()

	actions, err := filepath.Glob(".github/actions/*/action.yaml")
	require.NoError(t, err)
	workflows, err := filepath.Glob(".github/workflows/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, actions)
	require.NotEmpty(t, workflows)

	steps := map[string][]step{}
	for _, file := range actions {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var action compositeAction
		require.NoError(t, yaml.Unmarshal(data, &action))
		steps[file] = action.Runs.Steps
	}
	for _, file := range workflows {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var wf workflow
		require.NoError(t, yaml.Unmarshal(data, &wf))
		for _, job := range wf.Jobs {
			steps[file] = append(steps[file], job.Steps...)
		}
	}
	return steps
}

// ghAPICall matches a gh api command up to the end of its line, after
// backslash-continued lines are joined. It runs past pipes, since a --jq
// filter can contain one.
var ghAPICall = regexp.MustCompile(`gh api [^\n]*`)

// TestScriptsPaginateGHAPIReads checks that every gh api read (a call
// without -X) in the actions and workflows passes --paginate. Without it,
// gh returns only the first page (30 items), so the lookup for Bark's
// earlier review comment misses it on a busy PR and a duplicate is posted.
func TestScriptsPaginateGHAPIReads(t *testing.T) {
	reads := 0
	for file, steps := range scriptSteps(t) {
		for _, s := range steps {
			script := strings.ReplaceAll(s.Run, "\\\n", " ")
			for _, call := range ghAPICall.FindAllString(script, -1) {
				if strings.Contains(call, " -X ") || strings.Contains(call, " --method ") {
					continue
				}
				reads++
				assert.Contains(t, call, "--paginate", "%s, step %q", file, s.Name)
			}
		}
	}
	// Guards against the pattern silently matching nothing.
	assert.Positive(t, reads)
}
