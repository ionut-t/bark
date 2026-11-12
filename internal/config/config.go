package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/pelletier/go-toml/v2"
)

const (
	EditorKey             = "editor"
	LLMProviderKey        = "llm_provider"
	LLMModelKey           = "llm_model"
	MaxDiffLinesKey       = "max_diff_lines"
	RelativeNumberKey     = "relative_number"
	ContextEnrichmentKey  = "context_enrichment"
	StructuredFindingsKey = "structured_findings"
	CheckFindingsKey      = "check_findings"
	MinSeverityKey        = "min_severity"

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
	// LLMProviderChoice returns the provider this run is configured with
	// ("" if none) and whether it is pinned: set at the same layer as the
	// model or a higher one (run override > repository > user config). An
	// unpinned provider was chosen for a different model, so the factory may
	// infer the provider from the model instead.
	LLMProviderChoice() (provider string, pinned bool)
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
	SetStructuredFindings(structured bool) error
	GetStructuredFindings() bool
	OverrideStructuredFindings(structured bool)
	SetCheckFindings(check bool) error
	GetCheckFindings() bool
	OverrideCheckFindings(check bool)
	// SetMinSeverity saves the lowest severity a structured review shows;
	// "" shows every finding.
	SetMinSeverity(severity string) error
	GetMinSeverity() string
	OverrideMinSeverity(severity string) error
	// ApplyRepo layers a repository's config (read from path) over the
	// saved values for this run. Run overrides still win, and setters
	// never save the repository's values.
	ApplyRepo(path string, repo RepoData)
	// RepoLLMWarning says which provider and model this run takes from the
	// repository's config instead of the user's own; "" if none.
	RepoLLMWarning() string
}

// Data is the contents of the config file. The toml tags name the keys both
// when saving and when loading.
type Data struct {
	Editor             string `toml:"editor" comment:"The editor will be used to edit the config file and LLM instructions"`
	LLMProvider        string `toml:"llm_provider" comment:"It can be set to Gemini, VertexAI, OpenAI, Anthropic or Ollama. If not set, Bark infers it from the model name (claude-*, gpt-*, gemini-*, ...), then from the credentials it finds. A --model or repository model naming another provider's model switches to that provider unless the provider is set at the same level."`
	LLMModel           string `toml:"llm_model" comment:"The LLM model is required for VertexAI/Gemini/OpenAI LLMs, e.g., gemini-2.5-pro"`
	MaxDiffLines       uint32 `toml:"max_diff_lines" comment:"Maximum number of diff lines to include in the prompt (0 disables the limit)"`
	RelativeNumber     bool   `toml:"relative_number" comment:"Whether to use relative line numbers in the editor (default: false)"`
	ContextEnrichment  bool   `toml:"context_enrichment" comment:"Whether to include enclosing declarations (functions, structs, classes) and related code (definitions the change uses, callers of changed functions) as context for review (default: false)"`
	StructuredFindings bool   `toml:"structured_findings" comment:"Whether reviews ask the model for structured findings (file, line, severity, ...) instead of free text (default: false)"`
	CheckFindings      bool   `toml:"check_findings" comment:"Whether structured reviews make a second model call that checks each finding against the code and sets aside the ones that don't hold up (default: true)"`
	MinSeverity        string `toml:"min_severity" comment:"The lowest severity a structured review shows: critical, high, medium or low (default: empty, every finding)"`
}

// config is safe for concurrent use: the TUI saves relative_number from a tea.Cmd
// goroutine while Update reads it.
type config struct {
	path string

	// mu guards data and overrides. Setters hold it while writing the file,
	// so saves can't interleave and the file matches data.
	mu sync.Mutex
	// data is what the setters save; the overrides below are never saved.
	data      Data
	overrides overrides
	// repo holds the values a repository's config sets, read from
	// repoPath. They beat data and lose to overrides.
	repo     RepoData
	repoPath string
}

// overrides hold the values set for the current run only. A nil field means
// the saved value is used.
type overrides struct {
	llmProvider       *string
	llmModel          *string
	maxDiffLines      *uint32
	contextEnrichment *bool
	structured        *bool
	checkFindings     *bool
	minSeverity       *string
}

// valueOr returns the override if set, otherwise the saved value.
func valueOr[T any](override *T, saved T) T {
	if override != nil {
		return *override
	}

	return saved
}

// New returns a Config holding data. The setters save every value to path;
// the Override methods change values for the current run only and are never
// saved. Setting an overridden value replaces the override. An empty
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
		Editor:        DefaultEditor(),
		LLMModel:      defaultLLMModel,
		MaxDiffLines:  DEFAULT_MAX_DIFF_LINES,
		CheckFindings: true,
	}
}

func (c *config) Path() string {
	return c.path
}

func (c *config) SetEditor(editor string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if editor == c.data.Editor {
		return nil
	}

	c.data.Editor = editor

	return writeConfig(c.path, c.data)
}

func (c *config) GetEditor() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.data.Editor
}

func (c *config) SetLLMProvider(provider string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.llmProvider = nil

	if provider == c.data.LLMProvider {
		return nil
	}

	c.data.LLMProvider = provider

	return writeConfig(c.path, c.data)
}

func (c *config) GetLLMProvider() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	provider := valueOr(c.overrides.llmProvider, valueOr(c.repo.LLMProvider, c.data.LLMProvider))

	if provider == "" {
		return "", fmt.Errorf("%s not set in config", LLMProviderKey)
	}

	return provider, nil
}

func (c *config) LLMProviderChoice() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	provider := valueOr(c.overrides.llmProvider, valueOr(c.repo.LLMProvider, c.data.LLMProvider))
	if provider == "" {
		return "", false
	}

	providerLayer := layerOf(c.overrides.llmProvider, c.repo.LLMProvider)
	modelLayer := layerOf(c.overrides.llmModel, c.repo.LLMModel)

	return provider, providerLayer >= modelLayer
}

// layerOf ranks where a value comes from: 2 for a run override, 1 for the
// repository's config, 0 for the user's config.
func layerOf(override, repo *string) int {
	switch {
	case override != nil:
		return 2
	case repo != nil:
		return 1
	default:
		return 0
	}
}

func (c *config) OverrideModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if model != "" {
		c.overrides.llmModel = &model
	}
}

func (c *config) OverrideProvider(provider string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if provider == "" {
		return nil
	}

	if !isValidProvider(provider) {
		return fmt.Errorf("invalid provider: %s (supported: %s)", provider, strings.Join(supportedProviders, ", "))
	}

	c.overrides.llmProvider = &provider
	return nil
}

func (c *config) OverrideMaxDiffLines(lines uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.maxDiffLines = &lines
}

func (c *config) SetLLMModel(model string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.llmModel = nil

	if model == c.data.LLMModel {
		return nil
	}

	c.data.LLMModel = model

	return writeConfig(c.path, c.data)
}

func (c *config) GetLLMModel() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	model := valueOr(c.overrides.llmModel, valueOr(c.repo.LLMModel, c.data.LLMModel))

	if model == "" {
		return "", fmt.Errorf("%s not set in config", LLMModelKey)
	}

	return model, nil
}

func (c *config) SetMaxDiffLines(lines uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.maxDiffLines = nil

	if lines == c.data.MaxDiffLines {
		return nil
	}

	c.data.MaxDiffLines = lines

	return writeConfig(c.path, c.data)
}

func (c *config) GetMaxDiffLines() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return valueOr(c.overrides.maxDiffLines, valueOr(c.repo.MaxDiffLines, c.data.MaxDiffLines))
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if relative == c.data.RelativeNumber {
		return nil
	}

	c.data.RelativeNumber = relative

	return writeConfig(c.path, c.data)
}

func (c *config) GetRelativeNumber() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.data.RelativeNumber
}

func (c *config) SetContextEnrichment(enrich bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.contextEnrichment = nil

	if enrich == c.data.ContextEnrichment {
		return nil
	}

	c.data.ContextEnrichment = enrich

	return writeConfig(c.path, c.data)
}

func (c *config) GetContextEnrichment() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return valueOr(c.overrides.contextEnrichment, valueOr(c.repo.ContextEnrichment, c.data.ContextEnrichment))
}

func (c *config) OverrideContextEnrichment(enrich bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.contextEnrichment = &enrich
}

func (c *config) SetStructuredFindings(structured bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.structured = nil

	if structured == c.data.StructuredFindings {
		return nil
	}

	c.data.StructuredFindings = structured

	return writeConfig(c.path, c.data)
}

func (c *config) GetStructuredFindings() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return valueOr(c.overrides.structured, valueOr(c.repo.StructuredFindings, c.data.StructuredFindings))
}

func (c *config) OverrideStructuredFindings(structured bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.structured = &structured
}

func (c *config) SetCheckFindings(check bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.checkFindings = nil

	if check == c.data.CheckFindings {
		return nil
	}

	c.data.CheckFindings = check

	return writeConfig(c.path, c.data)
}

func (c *config) GetCheckFindings() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return valueOr(c.overrides.checkFindings, valueOr(c.repo.CheckFindings, c.data.CheckFindings))
}

func (c *config) SetMinSeverity(severity string) error {
	severity, err := NormaliseMinSeverity(severity)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.minSeverity = nil

	if severity == c.data.MinSeverity {
		return nil
	}

	c.data.MinSeverity = severity

	return writeConfig(c.path, c.data)
}

func (c *config) GetMinSeverity() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return valueOr(c.overrides.minSeverity, valueOr(c.repo.MinSeverity, c.data.MinSeverity))
}

func (c *config) OverrideMinSeverity(severity string) error {
	severity, err := NormaliseMinSeverity(severity)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.minSeverity = &severity
	return nil
}

func (c *config) ApplyRepo(path string, repo RepoData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.repo = repo
	c.repoPath = path
}

func (c *config) OverrideCheckFindings(check bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.overrides.checkFindings = &check
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
