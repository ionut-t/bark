package gemini

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuildsClientWithoutNetwork(t *testing.T) {
	client, err := New(context.Background(), "gemini-test", "fake-key", "low")
	require.NoError(t, err)
	assert.NotNil(t, client)
}
