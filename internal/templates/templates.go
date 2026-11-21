package templates

import (
	_ "embed"
	"regexp"
	"strings"

	"github.com/ionut-t/bark/v2/internal/version"
)

//go:embed commit.md
var defaultCommitInstructions string

//go:embed pull_request_description.md
var defaultPRInstructions string

//go:embed review-action.yaml
var defaultReviewActionTemplate string

//go:embed pr-description-action.yaml
var defaultPRDescriptionActionTemplate string

//go:embed combined-action.yaml
var defaultCombinedActionTemplate string

// GetDefaultCommitInstructions returns the default commit message instructions.
func GetDefaultCommitInstructions() string {
	return defaultCommitInstructions
}

// GetDefaultPRInstructions returns the default pull request description instructions.
func GetDefaultPRInstructions() string {
	return defaultPRInstructions
}

// GetDefaultReviewActionTemplate returns the default GitHub Actions workflow template for AI code reviews.
func GetDefaultReviewActionTemplate() string {
	return renderAction(defaultReviewActionTemplate, version.Version())
}

// GetDefaultPRDescriptionActionTemplate returns the default GitHub Actions workflow template for PR description generation.
func GetDefaultPRDescriptionActionTemplate() string {
	return renderAction(defaultPRDescriptionActionTemplate, version.Version())
}

// GetDefaultCombinedActionTemplate returns the default GitHub Actions workflow template combining code review and PR description generation.
func GetDefaultCombinedActionTemplate() string {
	return renderAction(defaultCombinedActionTemplate, version.Version())
}

// majorActionRef is the tag the release workflow moves to every release of
// this major version (the module's /v2).
const majorActionRef = "v2"

// releaseTag matches a release's tag, such as v2.35.0.
var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// actionRef is the ref a workflow written by Bark at version v uses the
// bark actions at: the release itself, so the workflow keeps running the
// action and binary it was written for. A build that isn't a release
// (dev, a pseudo-version) has no tag of its own and follows the major
// version.
func actionRef(v string) string {
	if releaseTag.MatchString(v) {
		return v
	}
	return majorActionRef
}

// renderAction fills in the action ref of a workflow template for Bark at
// version v.
func renderAction(tmpl, v string) string {
	return strings.ReplaceAll(tmpl, "{{ACTION_REF}}", actionRef(v))
}
