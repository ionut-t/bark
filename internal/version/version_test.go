package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGettersReturnPackageValues(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	t.Cleanup(func() { version, commit, date = oldVersion, oldCommit, oldDate })

	version, commit, date = "v1.2.3", "abcdef1", "22/11/2026"

	assert.Equal(t, "v1.2.3", Version())
	assert.Equal(t, "abcdef1", Commit())
	assert.Equal(t, "22/11/2026", Date())
}

func TestDefaultsAreNeverEmpty(t *testing.T) {
	assert.NotEmpty(t, Version())
	assert.NotEmpty(t, Commit())
	assert.NotEmpty(t, Date())
}
