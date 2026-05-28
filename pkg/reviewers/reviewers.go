package reviewers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ionut-t/bark/v2/internal/assets"
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

func FromFile(path string) (*Reviewer, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading reviewer file: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return &Reviewer{Name: name, Prompt: string(content)}, nil
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
