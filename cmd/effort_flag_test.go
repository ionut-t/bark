package cmd

import (
	"io"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every command that calls a model takes --effort for one run, and rejects
// an unknown level before doing any work.
func TestEffortFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  func(configFunc) *cobra.Command
	}{
		{name: "review", cmd: reviewCmd},
		{name: "commit", cmd: commitCmd},
		{name: "pr", cmd: prCmd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := tt.cmd(staticConfig(config.New("", config.Data{})))
			flag := cmd.Flags().Lookup("effort")
			require.NotNil(t, flag)
			for _, effort := range config.Efforts() {
				assert.Contains(t, flag.Usage, effort)
			}

			cmd.SetArgs([]string{"--effort", "ludicrous"})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ludicrous")
		})
	}
}
