package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

const (
	EditorKey            = "editor"
	LLMProviderKey       = "llm_provider"
	LLMModelKey          = "llm_model"
	MaxDiffLinesKey      = "max_diff_lines"
	RelativeNumberKey    = "relative_number"
	ContextEnrichmentKey = "context_enrichment"

	rootDir                    = ".bark"
	configFileName             = ".config.toml"
	commitInstructionsFileName = "commit.md"
	prInstructionsFileName     = "pull_request_description.md"

	DEFAULT_MAX_DIFF_LINES = 0
)

type Config interface {
	SetEditor(editor string) error
	GetEditor() string
	SetLLMProvider(provider string) error
	GetLLMProvider() (string, error)
	SetLLMModel(model string) error
	GetLLMModel() (string, error)
	OverrideModel(model string)
	OverrideProvider(provider string) error
	OverrideMaxDiffLines(lines uint32)
	GetCommitInstructions() string
	GetPRInstructions() string
	SetMaxDiffLines(lines uint32) error
	GetMaxDiffLines() uint32
	SetRelativeNumber(relative bool) error
	GetRelativeNumber() bool
	SetContextEnrichment(enrich bool) error
	GetContextEnrichment() bool
	OverrideContextEnrichment(enrich bool)
}

type configData struct {
	Editor            string `toml:"editor" comment:"The editor will be used to edit the config file and LLM instructions"`
	LLMProvider       string `toml:"llm_provider" comment:"It can be set to Gemini, VertexAI, OpenAI, Anthropic or Ollama. If not set, Bark will try to auto-detect the provider based on available credentials."`
	LLMModel          string `toml:"llm_model" comment:"The LLM model is required for VertexAI/Gemini/OpenAI LLMs, e.g., gemini-2.5-pro"`
	MaxDiffLines      uint32 `toml:"max_diff_lines" comment:"Maximum number of diff lines to include in the prompt (0 disables the limit)"`
	RelativeNumber    bool   `toml:"relative_number" comment:"Whether to use relative line numbers in the editor (default: false)"`
	ContextEnrichment bool   `toml:"context_enrichment" comment:"Whether to include enclosing declarations (functions, structs, classes) as context for review (default: false)"`
}

type config struct {
	data configData
}

func getConfigData() configData {
	return configData{
		Editor:            GetEditor(),
		LLMProvider:       viper.GetString(LLMProviderKey),
		LLMModel:          viper.GetString(LLMModelKey),
		MaxDiffLines:      viper.GetUint32(MaxDiffLinesKey),
		RelativeNumber:    viper.GetBool(RelativeNumberKey),
		ContextEnrichment: viper.GetBool(ContextEnrichmentKey),
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

func (c *config) OverrideModel(model string) {
	if model != "" {
		c.data.LLMModel = model
	}
}

func (c *config) OverrideProvider(provider string) error {
	if provider != "" && !isValidProvider(provider) {
		return fmt.Errorf("invalid provider: %s. Supported providers are 'gemini', 'vertexai', 'openai', 'anthropic', and 'ollama'", provider)
	}

	c.data.LLMProvider = provider
	return nil
}

func (c *config) OverrideMaxDiffLines(lines uint32) {
	c.data.MaxDiffLines = lines
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

func (c *config) SetMaxDiffLines(lines uint32) error {
	if lines == c.data.MaxDiffLines {
		return nil
	}

	c.data.MaxDiffLines = lines

	return writeConfig(c.data)
}

func (c *config) GetMaxDiffLines() uint32 {
	return c.data.MaxDiffLines
}

func (c *config) GetCommitInstructions() string {
	return getInstructions(commitInstructionsFileName, templates.GetDefaultCommitInstructions())
}

func (c *config) GetPRInstructions() string {
	content, err := getInstructionsFromCurrentDir(prInstructionsFileName)
	if err != nil {
		return getInstructions(prInstructionsFileName, templates.GetDefaultPRInstructions())
	}

	return content
}

func (c *config) SetRelativeNumber(relative bool) error {
	if relative == c.data.RelativeNumber {
		return nil
	}

	c.data.RelativeNumber = relative

	return writeConfig(c.data)
}

func (c *config) GetRelativeNumber() bool {
	return c.data.RelativeNumber
}

func (c *config) SetContextEnrichment(enrich bool) error {
	if enrich == c.data.ContextEnrichment {
		return nil
	}

	c.data.ContextEnrichment = enrich

	return writeConfig(c.data)
}

func (c *config) GetContextEnrichment() bool {
	return c.data.ContextEnrichment
}

func (c *config) OverrideContextEnrichment(enrich bool) {
	c.data.ContextEnrichment = enrich
}

func writeConfig(config configData) error {
	out, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
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
			viper.SetDefault(LLMModelKey, "gemini-2.5-pro")
			viper.SetDefault(MaxDiffLinesKey, DEFAULT_MAX_DIFF_LINES)
			viper.SetDefault(RelativeNumberKey, false)
			viper.SetDefault(ContextEnrichmentKey, false)

			if err := writeConfig(getConfigData()); err != nil {
				return "", err
			}

			fmt.Fprintln(os.Stderr, "Created config at", configPath)
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
		if err := os.WriteFile(commitPath, []byte(templates.GetDefaultCommitInstructions()), 0o644); err != nil {
			return fmt.Errorf("failed to write commit instructions: %w", err)
		}
	}

	if _, err := os.Stat(prPath); os.IsNotExist(err) {
		if err := os.WriteFile(prPath, []byte(templates.GetDefaultPRInstructions()), 0o644); err != nil {
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

func isValidProvider(provider string) bool {
	return provider == "gemini" || provider == "vertexai" || provider == "openai" || provider == "anthropic" || provider == "ollama"
}
