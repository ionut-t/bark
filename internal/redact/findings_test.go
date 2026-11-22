package redact

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindings(t *testing.T) {
	diff := "diff --git a/config.go b/config.go\n" +
		"--- a/config.go\n" +
		"+++ b/config.go\n" +
		"@@ -10,2 +10,3 @@\n" +
		" \tx := 1\n" +
		"+\tkey := \"" + awsKeyID + "\"\n" +
		" \treturn key\n"

	events := Findings(diff)
	require.Len(t, events, 1)
	f := events[0].Finding
	require.NotNil(t, f)
	assert.Equal(t, "config.go", f.File)
	assert.Equal(t, 11, f.Line)
	assert.Equal(t, findings.Critical, f.Severity)
	assert.Equal(t, "security", f.Category)
	assert.Equal(t, "Hard-coded secret (aws-access-key-id)", f.Title)
	assert.Contains(t, f.Body, "(aws-access-key-id)")
	assert.Contains(t, f.Body, "revoke or rotate it")
	assert.NotContains(t, f.Body, awsKeyID, "the finding never repeats the secret")
}

func TestFindingsNone(t *testing.T) {
	assert.Empty(t, Findings(""))
	assert.Empty(t, Findings("+++ b/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"))
}
