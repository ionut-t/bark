package templates

import _ "embed"

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
	return defaultReviewActionTemplate
}

// GetDefaultPRDescriptionActionTemplate returns the default GitHub Actions workflow template for PR description generation.
func GetDefaultPRDescriptionActionTemplate() string {
	return defaultPRDescriptionActionTemplate
}

// GetDefaultCombinedActionTemplate returns the default GitHub Actions workflow template combining code review and PR description generation.
func GetDefaultCombinedActionTemplate() string {
	return defaultCombinedActionTemplate
}
