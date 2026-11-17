package tui

import (
	"context"
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// truncatedLLM answers every call with a message cut off at the output limit.
type truncatedLLM struct{ llm.LLM }

func (truncatedLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Content: "feat: add the", Truncated: true}, nil
}

// A commit message or PR description the model cut off at its output limit
// is shown with a warning, and isn't changed.
func TestGeneratedMessageTruncated(t *testing.T) {
	t.Parallel()

	t.Run("commit", func(t *testing.T) {
		t.Parallel()

		msg := getCommitMessage(t.Context(), truncatedLLM{}, "", "")()
		require.IsType(t, commitResponseMsg{}, msg)
		m := newCommitChangesModel(nil, "", "", false, 80, 40)
		m, cmd := m.Update(msg)
		assert.NotNil(t, cmd, "the warning is cleared later")
		assert.Equal(t, "feat: add the", m.response)
		assert.Contains(t, m.View(), "output limit")
	})

	t.Run("pr", func(t *testing.T) {
		t.Parallel()

		msg := getPRMessage(t.Context(), truncatedLLM{}, "", "")()
		require.IsType(t, prResponseMsg{}, msg)
		m := newPRModel(nil, 80, 40)
		m, cmd := m.Update(msg)
		assert.NotNil(t, cmd, "the warning is cleared later")
		assert.Equal(t, "feat: add the", m.response)
		assert.Contains(t, m.View(), "output limit")
	})
}
