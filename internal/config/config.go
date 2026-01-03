package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

//go:embed commit.md
var defaultCommitInstructions string

//go:embed pull_request_description.md
var defaultPRInstructions string

const (
	EditorKey      = "editor"
	LLMProviderKey = "llm_provider"
	LLMModelKey    = "llm_model"

	rootDir                    = ".bark"
	configFileName             = ".config.toml"
	commitInstructionsFileName = "commit.md"
	prInstructionsFileName     = "pull_request_description.md"
)

type Config interface {
	SetEditor(editor string) error
	GetEditor() string
	SetLLMProvider(provider string) error
	GetLLMProvider() (string, error)
	SetLLMModel(model string) error
	GetLLMModel() (string, error)
	GetCommitInstructions() string
	GetPRInstructions() string
}

type configData struct {
	Editor      string `toml:"editor" comment:"The editor will be used to edit the config file and LLM instructions"`
	LLMProvider string `toml:"llm_provider" comment:"It can be set to Gemini or Vertex AI"`
	LLMModel    string `toml:"llm_model" comment:"The LLM model is required for Vertex AI/Gemini LLMs, e.g., gemini-2.5-pro"`
}

type config struct {
	data configData
}

func getConfigData() configData {
	return configData{
		Editor:      GetEditor(),
		LLMProvider: viper.GetString(LLMProviderKey),
		LLMModel:    viper.GetString(LLMModelKey),
	}
}

func New() Config {
	return &config{
		data: getConfigData(),
	}
}

func (c *config) SetEditor(editor string) error {
	if editor == c.GetEditor() {
		return nil
	}

	c.data.Editor = editor

	return writeConfig(c.data)
}

func (c *config) GetEditor() string {
	return c.data.Editor
}

func (c *config) SetLLMProvider(provider string) error {
	if provider == c.data.LLMProvider {
		return nil
	}

	c.data.LLMProvider = provider

	return writeConfig(c.data)
}

func (c *config) GetLLMProvider() (string, error) {
	provider := c.data.LLMProvider

	if provider == "" {
		return "", fmt.Errorf("%s not set in config", LLMProviderKey)
	}

	return provider, nil
}

func (c *config) SetLLMModel(model string) error {
	if model == c.data.LLMModel {
		return nil
	}

	c.data.LLMModel = model

	return writeConfig(c.data)
}

func (c *config) GetLLMModel() (string, error) {
	model := c.data.LLMModel

	if model == "" {
		return "", fmt.Errorf("%s not set in config", LLMModelKey)
	}

	return model, nil
}

func (c *config) GetCommitInstructions() string {
	return getInstructions(commitInstructionsFileName, defaultCommitInstructions)
}

func (c *config) GetPRInstructions() string {
	content, err := getInstructionsFromCurrentDir(prInstructionsFileName)
	if err != nil {
		return getInstructions(prInstructionsFileName, defaultPRInstructions)
	}

	return content
}

func writeConfig(config configData) error {
	out, err := toml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(GetConfigFilePath(), out, 0o644)
}

func getDefaultEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}

	if os.Getenv("WINDIR") != "" {
		return "notepad"
	}

	return "vim"
}

func GetEditor() string {
	editor := viper.GetString(EditorKey)

	if editor == "" {
		return getDefaultEditor()
	}

	return editor
}

func InitialiseConfigFile() (string, error) {
	configPath := viper.ConfigFileUsed()

	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		dir := filepath.Join(home, rootDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}

		configPath = filepath.Join(dir, configFileName)
		viper.SetConfigFile(configPath)

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			viper.SetDefault(EditorKey, GetEditor())
			viper.SetDefault(LLMProviderKey, "")
			viper.SetDefault(LLMModelKey, "gemini-2.0-flash")

			if err := writeConfig(getConfigData()); err != nil {
				return "", err
			}

			fmt.Println("Created config at", configPath)
		} else {
			viper.SetConfigFile(configPath)
			return "", viper.ReadInConfig()
		}
	}

	return configPath, nil
}

func InitialiseCommitInstructions() error {
	commitPath := GetCommitFilePath()
	prPath := GetPRFilePath()

	if _, err := os.Stat(commitPath); os.IsNotExist(err) {
		if err := os.WriteFile(commitPath, []byte(defaultCommitInstructions), 0o644); err != nil {
			return fmt.Errorf("failed to write commit instructions: %w", err)
		}
	}

	if _, err := os.Stat(prPath); os.IsNotExist(err) {
		if err := os.WriteFile(prPath, []byte(defaultPRInstructions), 0o644); err != nil {
			return fmt.Errorf("failed to write PR instructions: %w", err)
		}
	}

	return nil
}

func GetConfigFilePath() string {
	return viper.ConfigFileUsed()
}

func GetStorage() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, rootDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	return dir, nil
}

func GetCommitFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, rootDir, commitInstructionsFileName)
}

func GetPRFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, rootDir, prInstructionsFileName)
}

func getInstructions(filePath, defaultContent string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultContent
	}

	path := filepath.Join(home, rootDir, filePath)
	content, err := os.ReadFile(path)
	if err != nil || len(content) == 0 {
		return defaultContent
	}

	return string(content)
}

func getInstructionsFromCurrentDir(fileName string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if len(content) == 0 {
		return "", fmt.Errorf("file %s is empty", path)
	}

	return string(content), nil
}
