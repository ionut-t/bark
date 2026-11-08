package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/pelletier/go-toml/v2"
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

	defaultLLMModel = "gemini-2.5-pro"
)

type Config interface {
	// Path is the config file the setters write to.
	Path() string
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

// Data is the contents of the config file. The toml tags name the keys when
// saving and the mapstructure tags when loading (viper); keep them the same.
type Data struct {
	Editor            string `toml:"editor" mapstructure:"editor" comment:"The editor will be used to edit the config file and LLM instructions"`
	LLMProvider       string `toml:"llm_provider" mapstructure:"llm_provider" comment:"It can be set to Gemini, VertexAI, OpenAI, Anthropic or Ollama. If not set, Bark will try to auto-detect the provider based on available credentials."`
	LLMModel          string `toml:"llm_model" mapstructure:"llm_model" comment:"The LLM model is required for VertexAI/Gemini/OpenAI LLMs, e.g., gemini-2.5-pro"`
	MaxDiffLines      uint32 `toml:"max_diff_lines" mapstructure:"max_diff_lines" comment:"Maximum number of diff lines to include in the prompt (0 disables the limit)"`
	RelativeNumber    bool   `toml:"relative_number" mapstructure:"relative_number" comment:"Whether to use relative line numbers in the editor (default: false)"`
	ContextEnrichment bool   `toml:"context_enrichment" mapstructure:"context_enrichment" comment:"Whether to include enclosing declarations (functions, structs, classes) as context for review (default: false)"`
}

type config struct {
	path string
	data Data
}

// New returns a Config holding data. The setters save every value to path;
// the Override methods change values for the current run only. An empty
// editor falls back to $EDITOR, then the platform default.
func New(path string, data Data) Config {
	if data.Editor == "" {
		data.Editor = DefaultEditor()
	}

	return &config{path: path, data: data}
}

// DefaultData is what a new config file contains.
func DefaultData() Data {
	return Data{
		Editor:       DefaultEditor(),
		LLMModel:     defaultLLMModel,
		MaxDiffLines: DEFAULT_MAX_DIFF_LINES,
	}
}

func (c *config) Path() string {
	return c.path
}

func (c *config) SetEditor(editor string) error {
	if editor == c.GetEditor() {
		return nil
	}

	c.data.Editor = editor

	return writeConfig(c.path, c.data)
}

func (c *config) GetEditor() string {
	return c.data.Editor
}

func (c *config) SetLLMProvider(provider string) error {
	if provider == c.data.LLMProvider {
		return nil
	}

	c.data.LLMProvider = provider

	return writeConfig(c.path, c.data)
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
	if provider == "" {
		return nil
	}

	if !isValidProvider(provider) {
		return fmt.Errorf("invalid provider: %s (supported: %s)", provider, strings.Join(supportedProviders, ", "))
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

	return writeConfig(c.path, c.data)
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

	return writeConfig(c.path, c.data)
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

	return writeConfig(c.path, c.data)
}

func (c *config) GetRelativeNumber() bool {
	return c.data.RelativeNumber
}

func (c *config) SetContextEnrichment(enrich bool) error {
	if enrich == c.data.ContextEnrichment {
		return nil
	}

	c.data.ContextEnrichment = enrich

	return writeConfig(c.path, c.data)
}

func (c *config) GetContextEnrichment() bool {
	return c.data.ContextEnrichment
}

func (c *config) OverrideContextEnrichment(enrich bool) {
	c.data.ContextEnrichment = enrich
}

func writeConfig(path string, data Data) error {
	out, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, out, 0o644)
}

// DefaultEditor is the editor used when the config doesn't set one: $EDITOR,
// then notepad on Windows or vim elsewhere.
func DefaultEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}

	if os.Getenv("WINDIR") != "" {
		return "notepad"
	}

	return "vim"
}

// DefaultFilePath returns the config file path, $HOME/.bark/.config.toml,
// creating its directory if needed.
func DefaultFilePath() (string, error) {
	dir, err := GetStorage()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, configFileName), nil
}

// EnsureFile writes DefaultData to path if no file exists there, and reports
// whether it did. An existing file is left untouched.
func EnsureFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := writeConfig(path, DefaultData()); err != nil {
		return false, err
	}

	return true, nil
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
	return slices.Contains(supportedProviders, provider)
}

// supportedProviders lists every LLM provider Bark accepts, in the order
// shown to users.
var supportedProviders = []string{"gemini", "vertexai", "openai", "anthropic", "ollama"}

// SupportedProviders returns the LLM providers Bark accepts.
func SupportedProviders() []string {
	return slices.Clone(supportedProviders)
}
