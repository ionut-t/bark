package cmd

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderFlagHelpListsSupportedProviders(t *testing.T) {
	t.Parallel()

	cfg := config.New("", config.Data{})

	tests := []struct {
		name string
		cmd  *cobra.Command
		flag string
	}{
		{name: "review", cmd: reviewCmd(cfg), flag: "provider"},
		{name: "commit", cmd: commitCmd(cfg), flag: "provider"},
		{name: "pr", cmd: prCmd(cfg), flag: "provider"},
		{name: "config", cmd: configCmd(cfg), flag: "provider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			flag := tt.cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag)

			for _, provider := range config.SupportedProviders() {
				assert.Contains(t, flag.Usage, provider)
			}
		})
	}
}
