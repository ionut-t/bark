package reviewers

import (
	"embed"
	"fmt"
	"strings"

	"github.com/ionut-t/bark/internal/assets"
)

//go:embed prompts/*.md
var reviewers embed.FS

const assetDirName = "reviewers"

type Reviewer struct {
	Name   string
	Prompt string
}

func Config(storage string, reset bool) error {
	return assets.Config(assets.ConfigOptions{
		Storage:  storage,
		Reset:    reset,
		AssetDir: assetDirName,
		FromDir:  "prompts",
		EmbedFS:  reviewers,
	})
}

func Get(storage string) ([]Reviewer, error) {
	assetList, err := assets.GetAssets(storage, assetDirName)
	if err != nil {
		return nil, err
	}

	var reviewersList []Reviewer
	for _, asset := range assetList {
		reviewersList = append(reviewersList, Reviewer(asset))
	}

	return reviewersList, nil
}

func Find(name string, reviewersList []Reviewer) (*Reviewer, error) {
	for _, reviewer := range reviewersList {
		if strings.Contains(strings.ToLower(reviewer.Name), strings.ToLower(name)) {
			return &reviewer, nil
		}
	}

	return nil, fmt.Errorf("reviewer '%s' not found", name)
}

func RemoveDir(storage string) error {
	return assets.RemoveAssetDir(storage, assetDirName)
}

func Delete(storage, name string) error {
	return assets.Delete(storage, assetDirName, name)
}

func GetPath(storage, name string) (string, error) {
	return assets.GetPath(storage, assetDirName, name)
}
