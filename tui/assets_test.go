package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A model that failed to load its assets has no list. Bubble Tea still sends
// it messages, starting with the window size.
func TestAssetsModel_LoadErrorSurvivesMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		assetType AssetType
	}{
		{name: "reviewers", assetType: AssetReviewer},
		{name: "instructions", assetType: AssetInstruction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// A storage directory without the assets directory in it.
			m := NewAssetsModel(t.TempDir(), tt.assetType, AssetActionEdit, "true")
			require.Error(t, m.error)

			var model tea.Model = m
			for _, msg := range []tea.Msg{
				tea.WindowSizeMsg{Width: 80, Height: 24},
				lightBackground,
				tea.FocusMsg{},
			} {
				require.NotPanics(t, func() { model, _ = model.Update(msg) }, "%T", msg)
			}

			got := model.(AssetsModel)
			assert.Contains(t, got.View().Content, m.error.Error())

			// Any key leaves the screen.
			_, cmd := got.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			require.NotNil(t, cmd)
			assert.Equal(t, tea.Quit(), cmd())
		})
	}
}
