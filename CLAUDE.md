# Bark Codebase Architecture Guide

## Overview

Bark is a sophisticated CLI tool written in Go that leverages AI (specifically Google's Gemini and Vertex AI) to provide intelligent code reviews, automated commit message generation, and pull request description creation. It combines Cobra for CLI commands, Bubble Tea for TUI, and integrates deeply with Git.

**Version**: 2.5.0 (based on recent commits)  
**Go Version**: 1.25.1  
**Key Dependencies**: Cobra, Bubble Tea, Charmbracelet libraries, Google GenAI SDK

---

## Architecture Overview

```
bark/
├── main.go                 # Entry point
├── cmd/                    # CLI command handlers (Cobra commands)
├── pkg/                    # Core packages (reusable logic)
│   ├── git/                # Git integration
│   ├── llm/                # LLM abstractions and implementations
│   │   ├── llm.go          # Interface definitions
│   │   ├── genai/          # Shared GenAI implementation
│   │   ├── gemini/         # Gemini API adapter
│   │   ├── vertexai/       # Vertex AI adapter
│   │   └── llm_factory/    # Factory for LLM instantiation
│   ├── reviewers/          # Reviewer personas (embedded prompts)
│   └── instructions/       # Custom instruction management
├── internal/               # Private packages
│   ├── config/             # Configuration management
│   ├── assets/             # Asset (reviewer/instruction) management
│   ├── utils/              # Utility functions
│   └── version/            # Version info
├── tui/                    # Terminal UI (Bubble Tea models)
└── README.md
```

---

## 1. Main Entry Point & Command Structure

### main.go

- **Simple bootstrap**: Calls `cmd.Execute()` and handles top-level errors
- **Error styling**: Uses `coffee/styles` for colored error output

### cmd/root.go

- **Cobra root command**: Defines the main "bark" command
- **Command registration**: Registers all subcommands (review, commit, pr, config, reset, add, delete, edit)
- **Initialization**: Calls `initConfig()` on startup to set up storage, reviewers, and instructions
- **Default behavior**: Running `bark` without arguments opens the interactive TUI menu

### Command Structure

#### cmd/review.go

- Flags: `--as`, `--commit/-t`, `--changes/-c`, `--instructions/-i`, `--branch/-b`, `--staged/-s`, `--skip-instruction/-k`
- Mutually exclusive: `changes`, `commit`, `branch`, `staged`
- Creates TUI model with `TaskReview` and passes review options

#### cmd/commit.go

- Flags: `--all/-a`, `--hint/-i`
- Creates TUI model with `TaskCommit`

#### cmd/pr.go

- Flags: `--branch/-b` (optional, for base branch comparison)
- Creates TUI model with `TaskPRDescription`

#### cmd/config.go

- Manages configuration: editor, LLM provider, LLM model
- **initConfig()**: Called on startup; initializes storage, reviewers, instructions, and commit instructions
- Flags: `--editor/-e`, `--provider/-p`, `--model/-m`

#### cmd/add.go

- Adds new custom instruction
- Uses `instructions.Add()` which opens editor for content creation

#### cmd/delete.go

- Deletes instruction or reviewer (can be specified or selected from list)
- Handles both CLI and interactive TUI-based deletion

#### cmd/edit.go

- Edits existing instruction or reviewer files

#### cmd/reset.go

- Resets reviewers/instructions to defaults
- Flags: `--reviewers/-r`, `--instructions/-i`, `--hard` (destructive reset)

---

## 2. Core Packages

### pkg/git/git.go

**Purpose**: All Git operations using `exec.Command`

**Key Structures**:

```go
type Commit struct {
    Hash    string
    Author  string
    Date    string
    Message string
    Body    string
}

type BranchInfo struct {
    Name              string
    BaseBranch        string
    Commits           []Commit
    TotalFilesChanged int
    TotalAdditions    int
    TotalDeletions    int
    Diffs             string
}
```

**Core Functions**:

- `IsGitRepo()` - Validates current directory is a Git repo
- `GetCommits(limit)` - Gets recent commits with format: hash|author|date|message
- `GetDiff(hash)` - Full diff for a specific commit
- `GetWorkingTreeDiff(all bool)` - Current uncommitted changes (all or staged)
- `GetCurrentBranch()` - Current branch name
- `GetBranchDiff(branch)` - Diff between branches (truncates at 2000 lines)
- `CommitChanges(message, all)` - Creates a Git commit
- `GetBaseBranch()` - Auto-detects default branch (main/master/develop)
- `GetBranchCommits(baseBranch)` - All commits on current branch not in base
- `GetBranchStats(baseBranch)` - Files changed, additions, deletions stats
- `GetBranchInfo(baseBranch)` - Comprehensive branch metadata

**Error Handling**:

- Custom errors: `ErrNotAGitRepository`, `ErrNoChangesInRepository`, `ErrNoCommitsInRepository`
- Returns readable error messages for user feedback

### pkg/llm/

#### llm.go (Interface)

```go
type Response struct {
    Content string
    Time    time.Time
}

type LLM interface {
    Stream(ctx context.Context, prompt string) (<-chan Response, <-chan error)
    Generate(ctx context.Context, prompt string) (string, error)
}
```

#### llm_factory/llm_factory.go

**Purpose**: Factory pattern for LLM provider instantiation

**Provider Detection**:

1. Check config for `LLM_PROVIDER` setting
2. Auto-detect from environment variables if not set
3. Validate credentials for selected provider

**Supported Providers**:

- **Gemini**: Requires `GEMINI_API_KEY` environment variable
- **Vertex AI**: Requires `VERTEXAI_PROJECT_ID` and `VERTEXAI_LOCATION`

**Factory Logic**:

```go
func New(ctx context.Context, cfg config.Config) (llm.LLM, error)
```

- Loads credentials from env vars
- Validates provider has necessary credentials
- Instantiates appropriate provider adapter
- Returns generic `LLM` interface

#### genai/genai.go (Shared Implementation)

**Purpose**: Common GenAI client wrapper for both Gemini and Vertex AI

```go
type GenAI struct {
    model  string
    client *genai.Client
}
```

**Key Methods**:

- `Stream()`: Streams responses via channel (with context cancellation support)
- `Generate()`: Non-streaming response (30-second timeout)
- Both handle `ctx.Done()` checks to support early cancellation

#### gemini/gemini.go & vertexai/vertexai.go

**Purpose**: Thin adapters that wrap GenAI with provider-specific configuration

```go
// Gemini uses BackendGeminiAPI with API key
// Vertex AI uses BackendVertexAI with project ID and location
```

### pkg/reviewers/reviewers.go

**Purpose**: Manages reviewer personas

**Structure**:

```go
type Reviewer struct {
    Name   string
    Prompt string
}
```

**Key Functions**:

- `Config(storage, reset)` - Unpacks embedded reviewer prompts to storage directory
- `Get(storage)` - Loads all reviewers from storage as Asset list
- `Find(name, reviewersList)` - Finds reviewer by partial name match (case-insensitive)
- `RemoveDir()`, `Delete()`, `GetPath()` - Asset management

**Reviewer Storage**:

- Embedded via `//go:embed prompts/*.md`
- Extracted to `~/.bark/reviewers/` on first run
- Examples: "Uncle Bob" (clean code), "Linus Torvalds", "Rob Pike", "Ricky Gervais", "Sun Tzu"
- Each has detailed system prompt for code review style

### pkg/instructions/instructions.go

**Purpose**: Manages custom review instructions

**Structure**:

```go
type Instruction struct {
    Name   string
    Prompt string
}
```

**Key Functions**:

- Similar API to reviewers
- `Add()` - Creates new instruction by opening editor
- Users can create custom review guidelines

---

## 3. Configuration Management

### internal/config/config.go

**Storage Structure**:

```
~/.bark/
├── .config.toml              # Main config file
├── commit.md                 # Commit message generation instructions
├── pull_request_description.md # PR description instructions
├── reviewers/                # Reviewer prompts
│   └── *.md
└── instructions/             # Custom instructions
    └── *.md
```

**Config Keys**:

- `EDITOR` - Editor command (defaults to env var or vim)
- `LLM_PROVIDER` - "gemini" or "vertexai"
- `LLM_MODEL` - Model name (e.g., "gemini-2.5-pro")

**Config Interface**:

```go
type Config interface {
    GetEditor() string
    GetLLMProvider() (string, error)
    GetLLMModel() (string, error)
    GetCommitInstructions() string
    GetPRInstructions() string
}
```

**Key Functions**:

- `InitialiseConfigFile()` - Creates config on first run, reads if exists
- `InitialiseCommitInstructions()` - Creates default commit/PR instruction files
- `GetStorage()` - Returns `~/.bark` path, creates if needed
- `GetEditor()` - Returns configured or default editor

**Embedded Assets**:

- Default commit instructions: `//go:embed commit.md`
- Default PR instructions: `//go:embed pull_request_description.md`
- Default config template: `//go:embed config.toml`

### internal/assets/assets.go

**Purpose**: Generic asset management (reviewers and instructions)

**Asset Lifecycle**:

1. **Config**: Unpacks embedded files to storage directory
2. **GetAssets**: Loads all .md files from storage directory
3. **Add**: Opens editor in temp file, validates content, moves to storage
4. **Delete**: Removes specific asset file
5. **RemoveDir**: Clears entire asset directory

---

## 4. TUI (Terminal User Interface) Organization

### Architecture: State Machine with Bubble Tea

The TUI uses Bubble Tea's Elm-inspired architecture with a state machine pattern:

- **Model**: Central state holder (tui/app.go)
- **Views**: Enum-based view selection (viewInit, viewTasks, viewReviewers, etc.)
- **Sub-models**: Specific UI components for each view
- **Messages**: Type-driven event system

### tui/app.go (Main TUI Model)

**Central Model Structure**:

```go
type Model struct {
    width, height int
    error error
    currentView view

    llm llm.LLM
    config config.Config
    storage string

    // Task and workflow state
    tasks tasksModel
    selectedTask Task

    // Review flow
    commits commitsModel
    reviewOptions reviewOptionsModel
    selectedReviewer *reviewers.Reviewer
    review reviewModel

    // Instruction selection
    instructions instructionsModel
    skipInstruction bool

    // Commit generation
    commitChanges commitChangesModel
    hint string

    // PR generation
    branch string
    pr prModel

    // Context cancellation
    reviewCancelFunc context.CancelFunc
    operationCancelFunc context.CancelFunc
}
```

**View Enum**:

```go
const (
    viewInit
    viewTasks
    viewReviewOptions
    viewCommits
    viewReviewers
    viewInstructions
    viewReview
    viewCommitChanges
    viewPRDescription
    viewBranchInput
)
```

**Message Types**: Custom struct messages for state transitions:

- `taskSelectedMsg` - Task selection
- `reviewOptionSelectedMsg` - Review scope selection
- `reviewerSelectedMsg` - Reviewer selection
- `instructionSelectedMsg` - Instruction selection
- `commitSelectedMsg` - Commit selection
- `commitChangesMsg` - Generated commit ready
- `prInitReadyMsg` - PR generation ready
- `cancelXxxMsg` - Cancel messages for backtracking

**Initialization Logic**:

- LLM factory instantiation (with error handling)
- Git repo validation
- View selection based on options (immediate task or menu)

**Update Flow**:

- Window size changes update all sub-models
- Message routing to appropriate view model
- Context cancellation support for long-running operations
- Help toggle and retry mechanics (r key, ctrl+r)

### Key TUI Sub-models

#### tui/tasks.go (TasksModel)

- List of 3 main tasks: Review, Commit, PR Description
- Dispatches `taskSelectedMsg` on selection

#### tui/review-options.go (ReviewOptionsModel)

- 4 review scopes: Current changes, Staged, Commit, Branch
- Dispatches `reviewOptionSelectedMsg`

#### tui/commits.go (CommitsModel)

- Filterable list of recent commits
- Displays hash, author, date, message
- Dispatches `commitSelectedMsg`

#### tui/reviewers.go (ReviewersModel)

- Filterable list of available reviewers
- Shows reviewer names
- Dispatches `reviewerSelectedMsg`

#### tui/instructions.go (InstructionsModel)

- Filterable list of custom instructions
- 'x' key to skip instruction selection
- Dispatches `instructionSelectedMsg`

#### tui/review.go (ReviewModel)

- **Streaming display**: Shows LLM response in real-time
- **Loading animation**: Random humorous loading message
- **Error handling**: Display and retry capability
- **Context cancellation**: Cancels ongoing review via context
- **Spinner**: Visual feedback during streaming

**Key Messages**:

- `reviewReadyMsg` - Triggers review generation
- `reviewResultMsg` - Streaming response chunks
- `reviewErrorMsg` - Review failed

#### tui/commit-changes.go (CommitChangesModel)

- Shows generated commit message
- Edit button to open in editor
- Commit button with option to stage all changes (`-a` flag)
- Retry capability

#### tui/pr.go (PRModel)

- Shows generated PR description
- Edit button
- Copy to clipboard option
- Humorous loading messages specific to PRs

#### tui/list.go (Generic List Component)

- Reusable list model with custom item delegate
- Supports filtering, navigation, selection
- Custom styling via lipgloss

#### tui/branch-input.go (BranchInputModel)

- Text input for base branch name
- Used when comparing against specific branch

### tui/loading.go

- Spinner implementation with loading messages
- **Humorous messages**: Developer culture references, roasting themes
- Random message selection from 100+ options

### tui/assets.go

- Asset management UI for add/delete/edit reviewers and instructions
- Type-based asset selection

### Message Flow Pattern

```
User Action (tea.KeyMsg)
    ↓
Current view's Update() → Custom Message (e.g., reviewOptionSelectedMsg)
    ↓
Main Model's Update() → Handles message
    ↓
State mutation + Optional sub-model Update()
    ↓
View() renders current state
```

---

## 5. How LLM Integrations Work

### Provider Flow

1. **Configuration**:

   - User runs `bark config --provider gemini --model gemini-2.5-pro`
   - Config saved to `~/.bark/.config.toml`
   - Environment variables: `GEMINI_API_KEY` or `VERTEXAI_PROJECT_ID` + `VERTEXAI_LOCATION`

2. **Factory Instantiation** (llm_factory):

   ```
   New(context.Background(), config)
       ↓
   Load env credentials
       ↓
   Get provider from config (or auto-detect)
       ↓
   Validate credentials exist
       ↓
   Get model from config
       ↓
   Create appropriate provider (Gemini or Vertex AI)
   ```

3. **API Calls**:

   - **Stream**: `LLM.Stream(ctx, prompt)` returns `(<-chan Response, <-chan error)`
     - Non-blocking streaming via channels
     - Respects context cancellation
     - Used for review display with real-time updates
   - **Generate**: `LLM.Generate(ctx, prompt)` returns `(string, error)`
     - Blocking call with 30-second timeout
     - Used for commit messages, PR descriptions

4. **Prompt Construction**:
   - **Review**: Concatenates reviewer prompt + diff + instructions
   - **Commit**: Uses default commit instructions + diff + optional hint
   - **PR**: Uses PR instructions + branch info + commit messages

### Error Handling

- Missing credentials: Clear error message with which env var is needed
- Invalid provider: Lists supported providers
- API errors: Propagated to UI with retry capability

---

## 6. Git Integration & Diff Handling

### Diff Retrieval Patterns

1. **Working Tree** (`--staged` or current changes):

   ```go
   git diff --staged           // Staged changes only
   git diff HEAD               // All uncommitted changes
   ```

2. **Commit Diffs**:

   ```go
   git show <hash>             // Full commit diff
   ```

3. **Branch Diffs**:

   ```go
   git diff <base-branch>      // Against another branch (2000 line limit)
   ```

4. **Branch Info** (for PR generation):
   ```go
   git log <base>..HEAD        // Commits on branch
   git diff --shortstat <base>...HEAD  // Stats
   ```

### Diff Truncation

- Branch diffs limited to 2000 lines to prevent overwhelming LLM
- Appends "... (truncated)" marker

### Commit Parsing

- Format: `%H|%an|%ar|%s|%b||END||`
- Captures: hash, author, date, subject, body
- Custom delimiter handles multi-line bodies

---

## 7. Reviewers and Instructions Management

### Reviewer Management

**Storage Location**: `~/.bark/reviewers/`

**Built-in Reviewers** (embedded):

- Uncle Bob (Clean Code principles)
- Linus Torvalds (Linux kernel style)
- Rob Pike (Go philosophy)
- Ricky Gervais (Brutally honest)
- Sun Tzu (Strategic analysis)
- Others...

**Each Reviewer File** (.md):

- Detailed system prompt defining review style
- Key concerns and focus areas
- Review format template
- Voice/personality guidelines

**Custom Reviewers**:

- Users can create new reviewers via `bark add "reviewer-name"`
- Opens editor for prompt creation
- Saved to `~/.bark/reviewers/reviewer-name.md`

### Instruction Management

**Storage Location**: `~/.bark/instructions/`

**Default Instructions**: None embedded (empty initially)

**Custom Instructions**:

- Created via `bark add "instruction-name"`
- Used to customize review focus (e.g., security, performance)
- Optional during review workflow (can skip with `-k` flag)

### Asset Operations

**Config**:

- Unpacks embedded files on first run
- Skips if already exist (unless `--reset` or `--hard` flag)

**Add**:

1. Create temp file
2. Open in user's editor
3. Validate content not empty
4. Move to storage with .md extension

**Delete**:

- Remove specific asset file
- Can be done CLI-based or via TUI selection

**Edit**:

- Open existing asset in editor
- Save changes in place

**Reset**:

- `--reviewers`: Restore default reviewers only
- `--instructions`: Restore default instructions only
- `--hard`: Remove all and re-extract defaults

---

## 8. Important Patterns & Architectural Decisions

### 1. **Interface-based LLM Design**

- Single `LLM` interface used by all providers
- Factory pattern for provider instantiation
- Supports adding new providers (Claude, OpenAI, etc.) without changing core logic

### 2. **Context-based Cancellation**

- Long-running operations use `context.Context`
- TUI captures `context.CancelFunc` for early termination
- Graceful shutdown on Ctrl+C

### 3. **Embedded Assets**

- Reviewer prompts and default instructions embedded in binary
- No external files needed for base installation
- Easy customization: users can override by creating files in storage

### 4. **Message-driven State Machine**

- Bubble Tea messages coordinate state transitions
- View enum determines what's rendered
- Loose coupling between sub-models

### 5. **Error Recovery**

- Review failed? Press 'r' to retry
- Commit generation failed? Ctrl+R to retry
- Most operations support retry without restarting flow

### 6. **Git Abstraction**

- All Git operations in single `pkg/git` package
- Exec-based (not libgit2) for portability
- Custom error types for user-friendly messaging

### 7. **Configuration Hierarchy**

- Config file `~/.bark/.config.toml`
- Environment variables for credentials
- Command-line flags override config
- Sensible defaults when missing

### 8. **Streaming vs. Blocking**

- Review uses streaming for real-time display (better UX)
- Commit/PR use blocking calls (simpler, sufficient latency)
- Both support context cancellation

### 9. **Two-phase TUI Initialization**

- No LLM call until explicit task selection
- Reduces latency for `bark` command (just shows menu)
- Lazy evaluation of resources

### 10. **Asset Type Polymorphism**

- `Asset` struct with Name and Prompt
- Both reviewers and instructions use same storage pattern
- Generic asset management in `internal/assets`

---

## Command Flow Examples

### bark review --as "uncle bob" --staged

```
1. Parse flags → ReviewOption: ReviewOptionStagedChanges
2. Initialize TUI with TaskReview
3. Skip to reviewer selection (--as provided)
4. Get staged changes: git diff --staged
5. Load "Uncle Bob" reviewer prompt
6. Skip instruction selection (no -i provided)
7. Stream review to display
8. Show result with retry option
```

### bark commit --hint "feature"

```
1. Initialize TUI with TaskCommit
2. Get staged changes: git diff --staged
3. Load commit instructions
4. Append hint to prompt
5. Generate commit message (blocking)
6. Show in editor-ready display
7. On approval, commit via git commit -m
```

### bark pr --branch develop

```
1. Initialize TUI with TaskPRDescription
2. Get branch info (commits, stats, diffs)
3. Load PR instructions
4. Generate description with branch context
5. Show in editor-ready display
6. Allow edits via editor
```

---

## File Organization Reference

### CLI Entry Points

- `main.go` → Entry point
- `cmd/root.go` → Root command definition
- `cmd/{review,commit,pr,config,reset,add,delete,edit}.go` → Subcommand handlers

### Core Logic

- `pkg/git/git.go` → Git operations
- `pkg/llm/llm.go` → Interface
- `pkg/llm/llm_factory/llm_factory.go` → Provider selection
- `pkg/llm/{genai,gemini,vertexai}/` → Provider implementations
- `pkg/reviewers/reviewers.go` → Reviewer management
- `pkg/instructions/instructions.go` → Instruction management

### Configuration

- `internal/config/config.go` → Config management
- `internal/config/{commit.md, pull_request_description.md, config.toml}` → Embedded defaults
- `internal/assets/assets.go` → Asset unpacking/loading

### TUI

- `tui/app.go` → Main model and state machine
- `tui/{tasks,review-options,commits,reviewers,instructions}.go` → Sub-models
- `tui/{review,commit-changes,pr}.go` → Result display models
- `tui/list.go` → Generic list component
- `tui/{loading,branch-input,assets}.go` → Supporting components
- `tui/format.md` → Formatting requirements for LLM responses

### Utilities

- `internal/utils/utils.go` → Helper functions (editor, message dispatch)
- `internal/version/version.go` → Version info

---

## Key Dependencies

- **Cobra**: CLI framework with flag parsing and command routing
- **Bubble Tea**: TUI framework (state machine, event loop, rendering)
- **Charmbracelet libraries**: Styling (lipgloss), markdown rendering, text input
- **Viper**: Config file management
- **Google GenAI SDK**: LLM API client for both Gemini and Vertex AI
- **Custom coffee libs** (ionut-t/coffee): Reusable UI components and styles

---

## Development Tips

1. **Adding a new reviewer**: Add .md file to `pkg/reviewers/prompts/`
2. **Adding new LLM provider**: Create adapter in `pkg/llm/{provider}/`, update factory
3. **Modifying TUI flow**: Update `app.go` view enum and message handlers
4. **Testing Git integration**: Uses actual `git` CLI, safe to test
5. **Config testing**: Uses real `~/.bark` directory; use separate BARK_HOME for tests

---

This architecture emphasizes:

- **Clarity**: Each package has a clear single responsibility
- **Extensibility**: LLM providers, reviewers, and instructions easily customizable
- **User experience**: Streaming responses, retry mechanics, humorous loading messages
- **Robustness**: Error handling, context cancellation, graceful degradation

