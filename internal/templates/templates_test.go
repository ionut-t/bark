package templates

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    string
	}{
		{"v2.35.0", "v2.35.0"},
		{"v3.0.1", "v3.0.1"},
		// Anything that isn't a release tag follows the major version.
		{"dev", "v2"},
		{"dev-abc1234", "v2"},
		{"", "v2"},
		{"v2.35.0-0.20261121120000-abcdef123456", "v2"},
		{"v2.35.0+dirty", "v2"},
		{"2.35.0", "v2"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, actionRef(tt.version))
		})
	}
}

func TestActionTemplatesArePinned(t *testing.T) {
	t.Parallel()

	uses := regexp.MustCompile(`uses: ionut-t/bark/\.github/actions/[a-z-]+@(\S+)`)
	templates := map[string]string{
		"review":         renderAction(defaultReviewActionTemplate, "v2.35.0"),
		"pr-description": renderAction(defaultPRDescriptionActionTemplate, "v2.35.0"),
		"combined":       renderAction(defaultCombinedActionTemplate, "v2.35.0"),
	}
	for name, tmpl := range templates {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			refs := uses.FindAllStringSubmatch(tmpl, -1)
			if assert.NotEmpty(t, refs, "no bark action in the template") {
				for _, m := range refs {
					assert.Equal(t, "v2.35.0", m[1])
				}
			}
			assert.NotContains(t, tmpl, "{{ACTION_REF}}")
			// The action installs the release it comes from; a
			// bark-version here would let the two drift apart.
			assert.NotContains(t, tmpl, "bark-version")
		})
	}
}

func TestDefaultActionTemplatesUseThisVersion(t *testing.T) {
	t.Parallel()

	// Tests build without release ldflags, so the version isn't a tag.
	assert.Contains(t, GetDefaultReviewActionTemplate(), "bark-review@v2\n")
	assert.Contains(t, GetDefaultPRDescriptionActionTemplate(), "bark-pr-description@v2\n")
	assert.Contains(t, GetDefaultCombinedActionTemplate(), "bark-review@v2\n")
}
