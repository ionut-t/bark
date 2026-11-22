package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdded(t *testing.T) {
	diff := "diff --git a/config.go b/config.go\n" +
		"--- a/config.go\n" +
		"+++ b/config.go\n" +
		"@@ -10,3 +10,4 @@ func load() {\n" +
		" \told := \"" + googleKey + "\"\n" + // context: already in the code
		"-\tkey := \"" + ghToken + "\"\n" + // removed
		"+\tkey := \"" + awsKeyID + "\"\n" +
		"+\tkey2 := \"" + awsKeyID + "\" // " + awsKeyID + "\n" + // one finding a line
		" \treturn key\n" +
		"diff --git a/deploy/id_rsa b/deploy/id_rsa\n" +
		"new file mode 100600\n" +
		"--- /dev/null\n" +
		"+++ b/deploy/id_rsa\n" +
		"@@ -0,0 +1,3 @@\n" +
		"+" + pemBegin + "\n" +
		"+" + pemBody1 + "\n" +
		"+" + pemEnd + "\n"

	assert.Equal(t, []Secret{
		{File: "config.go", Line: 11, Kind: "aws-access-key-id"},
		{File: "config.go", Line: 12, Kind: "aws-access-key-id"},
		{File: "deploy/id_rsa", Line: 1, Kind: "private-key"},
	}, Added(diff))
}

func TestAddedNone(t *testing.T) {
	assert.Empty(t, Added("+++ b/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"))
	assert.Empty(t, Added(""))
}
