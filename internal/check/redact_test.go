package check

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
)

// The model is sent the code with secrets masked (see redact.Wrap), so it
// quotes the placeholder. That evidence still backs a finding.
func TestApplyEvidenceWithMaskedSecret(t *testing.T) {
	t.Parallel()

	key := "AKIA" + "IOSFODNN7EXAMPLE"
	in := Input{
		Diff:  "+++ b/aws.go\n@@ -0,0 +1,1 @@\n     1 +const accessKey = \"" + key + "\"\n",
		Files: map[string]string{"aws.go": "package aws\n\nconst secretKey = \"" + key + "\"\n"},
	}
	fs := []findings.Finding{{File: "aws.go", Line: 1, Title: "hard-coded key"}}

	fromDiff := Apply(in, fs, map[int]Verdict{1: {Verdict: "keep", Evidence: `const accessKey = "[REDACTED:aws-access-key-id]"`, Reason: "r"}})
	fromFile := Apply(in, fs, map[int]Verdict{1: {Verdict: "keep", Evidence: `const secretKey = "[REDACTED:aws-access-key-id]"`, Reason: "r"}})
	made := Apply(in, fs, map[int]Verdict{1: {Verdict: "keep", Evidence: `const otherKey = "[REDACTED:aws-access-key-id]"`, Reason: "r"}})

	assert.True(t, fromDiff[0].Kept, "evidence from the diff")
	assert.True(t, fromFile[0].Kept, "evidence from the file")
	assert.False(t, made[0].Kept, "evidence that isn't in the code")
}
