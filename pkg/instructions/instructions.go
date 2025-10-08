package instructions

import (
	"embed"
	"fmt"
	"strings"

	"github.com/ionut-t/bark/internal/assets"
)

//go:embed prompts/*.md
var instructions embed.FS

const assetDirName = "instructions"

type Instruction struct {
	Name   string
	Prompt string
}

func Config(storage string, reset bool) error {
	return assets.Config(assets.ConfigOptions{
		Storage:  storage,
		Reset:    reset,
		AssetDir: assetDirName,
		FromDir:  "prompts",
		EmbedFS:  instructions,
	})
}

func Get(storage string) ([]Instruction, error) {
	assetList, err := assets.GetAssets(storage, assetDirName)
	if err != nil {
		return nil, err
	}

	var instructionsList []Instruction
	for _, asset := range assetList {
		instructionsList = append(instructionsList, Instruction(asset))
	}

	return instructionsList, nil
}

func Find(name string, instructionsList []Instruction) (*Instruction, error) {
	for _, instruction := range instructionsList {
		if strings.Contains(strings.ToLower(instruction.Name), strings.ToLower(name)) {
			return &instruction, nil
		}
	}
	return nil, fmt.Errorf("instruction not found")
}

func RemoveDir(storage string) error {
	return assets.RemoveAssetDir(storage, assetDirName)
}

func Add(storage, name string) error {
	return assets.Add(storage, assetDirName, name)
}

func Delete(storage, name string) error {
	return assets.Delete(storage, assetDirName, name)
}

func GetPath(storage, name string) (string, error) {
	return assets.GetPath(storage, assetDirName, name)
}
