package instructions

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"github.com/ionut-t/bark/v2/internal/assets"
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

// Resolve turns the values given to --instructions into one instruction
// text. Each value is the path of a file to read, the name of an
// instruction in saved, or else the instruction text itself. The results
// are combined in order by Join; blank values are skipped.
func Resolve(values []string, saved []Instruction) (string, error) {
	var prompts []string
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		text, err := resolveOne(value, saved)
		if err != nil {
			return "", err
		}
		prompts = append(prompts, text)
	}
	return Join(prompts), nil
}

// Join combines instruction prompts into one text, in order, separated by
// blank lines. Blank prompts are skipped.
func Join(prompts []string) string {
	var parts []string
	for _, prompt := range prompts {
		if prompt = strings.TrimSpace(prompt); prompt != "" {
			parts = append(parts, prompt)
		}
	}
	return strings.Join(parts, "\n\n")
}

func resolveOne(value string, saved []Instruction) (string, error) {
	if info, err := os.Stat(value); err == nil && !info.IsDir() {
		content, err := os.ReadFile(value)
		if err != nil {
			return "", fmt.Errorf("error reading instructions file %q: %w", value, err)
		}
		return string(content), nil
	}
	if len(saved) > 0 {
		if instruction, err := Find(value, saved); err == nil {
			return instruction.Prompt, nil
		}
	}
	return value, nil
}

func GetEmbedded(name string) (*Instruction, error) {
	content, err := instructions.ReadFile("prompts/" + name + ".md")
	if err != nil {
		return nil, fmt.Errorf("instruction '%s' not found", name)
	}
	return &Instruction{Name: name, Prompt: string(content)}, nil
}

func RemoveDir(storage string) error {
	return assets.RemoveAssetDir(storage, assetDirName)
}

func Add(storage, name, editor string) error {
	return assets.Add(storage, assetDirName, name, editor)
}

func Delete(storage, name string) error {
	return assets.Delete(storage, assetDirName, name)
}

func GetPath(storage, name string) (string, error) {
	return assets.GetPath(storage, assetDirName, name)
}
