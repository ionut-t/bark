package config

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// repoFileName is a repository's own config, in its .bark directory
const repoFileName = "config.toml"

// RepoFilePath returns where the config of the repository at root lives.
func RepoFilePath(root string) string {
	return filepath.Join(root, rootDir, repoFileName)
}

// RepoData is what a repository's config can set: the review settings a
// team shares. The editor and relative line numbers stay personal (the
// editor is run as a command, so a cloned repository must not choose it).
// A nil field isn't set, and the user's value applies.
type RepoData struct {
	LLMProvider        *string `toml:"llm_provider"`
	LLMModel           *string `toml:"llm_model"`
	MaxDiffLines       *uint32 `toml:"max_diff_lines"`
	ContextEnrichment  *bool   `toml:"context_enrichment"`
	StructuredFindings *bool   `toml:"structured_findings"`
	CheckFindings      *bool   `toml:"check_findings"`
	MinSeverity        *string `toml:"min_severity"`
}

// repoKeys are the keys RepoData accepts, for error messages.
var repoKeys = []string{
	LLMProviderKey, LLMModelKey, MaxDiffLinesKey, ContextEnrichmentKey,
	StructuredFindingsKey, CheckFindingsKey, MinSeverityKey,
}

// ParseRepo reads a repository's config from content, the contents of the
// file at path (used in errors). Unknown keys are an error, so a misspelt
// or personal setting doesn't silently do nothing.
func ParseRepo(path string, content []byte) (RepoData, error) {
	var repo RepoData
	dec := toml.NewDecoder(bytes.NewReader(content)).DisallowUnknownFields()
	if err := dec.Decode(&repo); err != nil {
		if strictErr, ok := errors.AsType[*toml.StrictMissingError](err); ok {
			var keys []string
			for _, e := range strictErr.Errors {
				keys = append(keys, strings.Join(e.Key(), "."))
			}
			return RepoData{}, fmt.Errorf("%s: unknown key(s) %s; a repository's config can set %s",
				path, strings.Join(keys, ", "), strings.Join(repoKeys, ", "))
		}
		if decodeErr, ok := errors.AsType[*toml.DecodeError](err); ok {
			row, col := decodeErr.Position()
			return RepoData{}, fmt.Errorf("%s:%d:%d: %w", path, row, col, err)
		}
		return RepoData{}, fmt.Errorf("%s: %w", path, err)
	}

	if repo.LLMProvider != nil && !isValidProvider(*repo.LLMProvider) {
		return RepoData{}, fmt.Errorf("%s: invalid %s %q (supported: %s)",
			path, LLMProviderKey, *repo.LLMProvider, strings.Join(supportedProviders, ", "))
	}
	if repo.MinSeverity != nil {
		severity, err := NormaliseMinSeverity(*repo.MinSeverity)
		if err != nil {
			return RepoData{}, fmt.Errorf("%s: %w", path, err)
		}
		repo.MinSeverity = &severity
	}

	return repo, nil
}

// severities are the values min_severity accepts, most severe first.
var severities = []string{"critical", "high", "medium", "low"}

// Severities returns the values min_severity accepts, most severe first.
func Severities() []string {
	return slices.Clone(severities)
}

// NormaliseMinSeverity lowercases s and checks it's a known severity or "".
// Use it on a min_severity value from a file.
func NormaliseMinSeverity(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s != "" && !slices.Contains(severities, s) {
		return "", fmt.Errorf("invalid %s %q (supported: %s)", MinSeverityKey, s, strings.Join(severities, ", "))
	}
	return s, nil
}

func (c *config) RepoLLMWarning() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var theirs, yours []string
	describe := func(key string, repo, override *string, saved string) {
		if repo == nil || override != nil || *repo == saved {
			return
		}
		theirs = append(theirs, fmt.Sprintf("%s = %q", key, *repo))
		if saved == "" {
			yours = append(yours, key+" auto-detected")
		} else {
			yours = append(yours, fmt.Sprintf("%s = %q", key, saved))
		}
	}
	describe(LLMProviderKey, c.repo.LLMProvider, c.overrides.llmProvider, c.data.LLMProvider)
	describe(LLMModelKey, c.repo.LLMModel, c.overrides.llmModel, c.data.LLMModel)

	if len(theirs) == 0 {
		return ""
	}
	return fmt.Sprintf("%s sets %s, so this run uses it instead of your config (%s) and your credentials for it; pass --provider/--model to choose",
		c.repoPath, strings.Join(theirs, " and "), strings.Join(yours, ", "))
}
