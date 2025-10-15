package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/spf13/viper"
)

//go:embed commit.md
var defaultCommitInstructions string

//go:embed pull_request_description.md
var defaultPRInstructions string

//go:embed config.toml
var defaultConfig string

const (
	EditorKey      = "EDITOR"
	LLMProviderKey = "LLM_PROVIDER"
	LLMModelKey    = "LLM_MODEL"
	AutoUpdateKey  = "AUTO_UPDATE_ENABLED"

	rootDir                    = ".bark"
	configFileName             = ".config.toml"
	commitInstructionsFileName = "commit.md"
	prInstructionsFileName     = "pull_request_description.md"
)

type Config interface {
	GetEditor() string
	Storage() string
	GetLLMProvider() (string, error)
	GetLLMModel() (string, error)
	GetCommitInstructions() string
	GetPRInstructions() string
	AutoUpdateEnabled() bool
}

type config struct {
	storage string
}

func New() (Config, error) {
	storage, err := GetStorage()

	if err != nil {
		return nil, err
	}

	return &config{
		storage: storage,
	}, nil
}

func (c *config) AutoUpdateEnabled() bool {
	return viper.GetBool(AutoUpdateKey)
}

func (c *config) GetEditor() string {
	return GetEditor()
}

func (c *config) Storage() string {
	return c.storage
}

func (c *config) GetLLMProvider() (string, error) {
	provider := viper.GetString(LLMProviderKey)

	if provider == "" {
		return "", fmt.Errorf("%s not set in config", LLMProviderKey)
	}

	return provider, nil
}

func (c *config) GetLLMModel() (string, error) {
	model := viper.GetString(LLMModelKey)

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
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}

		configPath = filepath.Join(dir, configFileName)
		viper.SetConfigFile(configPath)

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			viper.SetDefault(AutoUpdateKey, true)
			viper.SetDefault(EditorKey, GetEditor())
			viper.SetDefault(LLMProviderKey, "")
			viper.SetDefault(LLMModelKey, "")

			if err := writeDefaultConfig(); err != nil {
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
		if err := os.WriteFile(commitPath, []byte(defaultCommitInstructions), 0644); err != nil {
			return fmt.Errorf("failed to write commit instructions: %w", err)
		}
	}

	if _, err := os.Stat(prPath); os.IsNotExist(err) {
		if err := os.WriteFile(prPath, []byte(defaultPRInstructions), 0644); err != nil {
			return fmt.Errorf("failed to write PR instructions: %w", err)
		}
	}

	return nil
}

func GetConfigFilePath() string {
	return viper.ConfigFileUsed()
}

func GetStorage() (string, error) {
	storage := viper.GetString("storage")

	if storage != "" {
		return storage, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, rootDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
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

func writeDefaultConfig() error {
	tmpl, err := template.New("config").Parse(defaultConfig)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	data := map[string]any{
		"Editor": GetEditor(),
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	return os.WriteFile(GetConfigFilePath(), buf.Bytes(), 0644)
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
