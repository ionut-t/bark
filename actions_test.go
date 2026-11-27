package main

import (
	"os"
	"os/exec"
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
	ID   string `yaml:"id"`
	If   string `yaml:"if"`
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

// TestReviewActionFailsOnFindingsLast checks that with fail-on, the review
// action still posts the review comment and label before failing: bark's
// exit status 3 (findings at or above fail-on) mustn't stop the run step
// under bash -e, and a final step fails the job instead.
func TestReviewActionFailsOnFindingsLast(t *testing.T) {
	data, err := os.ReadFile(".github/actions/bark-review/action.yaml")
	require.NoError(t, err)

	var action struct {
		Inputs map[string]any `yaml:"inputs"`
		compositeAction `yaml:",inline"`
	}
	require.NoError(t, yaml.Unmarshal(data, &action))
	assert.Contains(t, action.Inputs, "fail-on")

	steps := action.Runs.Steps
	var run *step
	for i := range steps {
		if steps[i].Name == "Run bark review" {
			run = &steps[i]
		}
	}
	require.NotNil(t, run)
	assert.NotEmpty(t, run.ID, "later steps read the run step's status output")
	assert.Contains(t, run.Run, `--fail-on "$BARK_FAIL_ON"`)
	assert.Contains(t, run.Run, "|| STATUS=$?", "a non-zero exit mustn't end the step under bash -e")
	assert.Contains(t, run.Run, `"status=$STATUS" >> "$GITHUB_OUTPUT"`)

	last := steps[len(steps)-1]
	assert.Contains(t, last.If, "steps."+run.ID+".outputs.status == '3'", "the failing step comes after posting and labelling")
	assert.Contains(t, last.Run, "exit 1")
}

// TestApprovePassedOnlyWithInlineComments checks that the review action and
// Bark's own workflow pass --approve, and --approve-below with it, only when
// the review is posted with inline comments, which --approve needs.
func TestApprovePassedOnlyWithInlineComments(t *testing.T) {
	data, err := os.ReadFile(".github/actions/bark-review/action.yaml")
	require.NoError(t, err)
	var action struct {
		Inputs map[string]any `yaml:"inputs"`
		compositeAction `yaml:",inline"`
	}
	require.NoError(t, yaml.Unmarshal(data, &action))
	assert.Contains(t, action.Inputs, "approve")
	assert.Contains(t, action.Inputs, "approve-below")

	data, err = os.ReadFile(".github/workflows/bark.yaml")
	require.NoError(t, err)
	var wf workflow
	require.NoError(t, yaml.Unmarshal(data, &wf))

	scripts := map[string][]step{"action": action.Runs.Steps, "workflow": wf.Jobs["review"].Steps}
	for name, steps := range scripts {
		var run string
		for _, s := range steps {
			if s.Name == "Run bark review" {
				run = s.Run
			}
		}
		require.NotEmpty(t, run, name)
		i := strings.Index(run, "ARGS+=(--approve)")
		require.Positive(t, i, name)
		assert.Contains(t, run[:i], `"$BARK_INLINE_COMMENTS"`, "%s: --approve only with inline comments", name)
		assert.Contains(t, run[i:], `ARGS+=(--approve-below "$BARK_APPROVE_BELOW")`, "%s: --approve-below only with --approve", name)
	}
}

// TestWithContextPassedBothWays checks that the review action and Bark's
// own workflow pass the with-context setting to Bark either way. Context is
// on by default, so passing only --with-context when it's "true" leaves no
// way to turn it off; unset leaves Bark's default. The action's input
// defaults to true to match.
func TestWithContextPassedBothWays(t *testing.T) {
	data, err := os.ReadFile(".github/actions/bark-review/action.yaml")
	require.NoError(t, err)
	var action struct {
		Inputs map[string]struct {
			Default string `yaml:"default"`
		} `yaml:"inputs"`
		compositeAction `yaml:",inline"`
	}
	require.NoError(t, yaml.Unmarshal(data, &action))
	assert.Equal(t, "true", action.Inputs["with-context"].Default)

	data, err = os.ReadFile(".github/workflows/bark.yaml")
	require.NoError(t, err)
	var wf workflow
	require.NoError(t, yaml.Unmarshal(data, &wf))

	scripts := map[string][]step{"action": action.Runs.Steps, "workflow": wf.Jobs["review"].Steps}
	for name, steps := range scripts {
		var lines []string
		for _, s := range steps {
			if s.Name != "Run bark review" {
				continue
			}
			for line := range strings.SplitSeq(s.Run, "\n") {
				if strings.Contains(line, "$BARK_WITH_CONTEXT") {
					lines = append(lines, line)
				}
			}
		}
		require.NotEmpty(t, lines, name)
		script := "ARGS=()\n" + strings.Join(lines, "\n") + "\nprintf '%s\\n' \"${ARGS[@]}\"\n"

		tests := []struct {
			value string
			want  string
		}{
			{value: "", want: ""},
			{value: "true", want: "--with-context=true\n"},
			{value: "false", want: "--with-context=false\n"},
		}
		for _, tt := range tests {
			t.Run(name+"/"+tt.value, func(t *testing.T) {
				cmd := exec.Command("bash", "-c", script)
				cmd.Env = append(os.Environ(), "BARK_WITH_CONTEXT="+tt.value)
				out, err := cmd.Output()
				require.NoError(t, err)
				assert.Equal(t, tt.want, strings.TrimLeft(string(out), "\n"))
			})
		}
	}
}
