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

func GetDefaultCommitInstructions() string {
	return defaultCommitInstructions
}

func GetDefaultPRInstructions() string {
	return defaultPRInstructions
}

func GetDefaultReviewActionTemplate() string {
	return defaultReviewActionTemplate
}

func GetDefaultPRDescriptionActionTemplate() string {
	return defaultPRDescriptionActionTemplate
}

func GetDefaultCombinedActionTemplate() string {
	return defaultCombinedActionTemplate
}
