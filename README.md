# Bark - AI Code Reviewer

Bark is a command-line tool that uses AI to review code, generate commit messages, and create pull request descriptions.

## Features

- **AI-powered code review:** Get feedback on your code from an AI assistant.
- **Commit message generation:** Automatically generate descriptive commit messages.
- **Pull request descriptions:** Create detailed pull request descriptions from your branch changes.
- **Multiple reviewers:** Choose from a variety of "reviewers" with different personalities, such as Linus Torvalds, Uncle Bob, or Yoda.
- **Custom instructions:** Provide custom instructions to the AI to tailor the review.
- **Review commits, branches, or current changes:** Analyse code at any stage of development.
- **Interactive TUI:** A user-friendly terminal interface for all tasks.

## Installation

To install Bark, use `go install`:

```bash
go install github.com/ionut-t/bark@latest
```

## Usage

Bark provides three main commands: `review`, `commit`, and `pr`.

### Code Review

To review the changes, run `bark review`:

```bash
bark review
```

By default, `bark review` will analyse all tracked changes. To review only the staged changes, use the `--staged` or `-s` flag:

```bash
bark review --staged
```

To compare the current branch to a specific branch, use the `--branch` or `-b` flag:

```bash
bark review --branch <branch-name>
```

To select a commit to review from a list of recent commits, use the `--commit` or `-t` flag:

```bash
bark review --commit
```

To use a specific reviewer, use the `--as` flag:

```bash
bark review --as linus
```

To provide custom instructions to the reviewer, use the `--instructions` or `-i` flag with the name of the instruction file:

```bash
bark review --instructions <instruction-name>
```

### Commit Message Generation

To generate a commit message for the current staged changes, run `bark commit`:

```bash
bark commit
```

### Pull Request Description Generation

To generate a pull request description for the current branch, run `bark pr`:

```bash
bark pr
```

## Configuration

Bark uses a configuration file located at `$HOME/.bark/config.toml`. You can edit this file directly or use the `config` command to manage your settings.

To set your preferred editor, LLM provider, or model, use the `config` command with the appropriate flags:

```bash
bark config --editor nvim
bark config --provider gemini
bark config --model gemini-2.5-pro
```

## Reset

To reset the reviewers and instructions to their default state use the `reset` command.

To reset only the reviewers, use the `--reviewers` or `-r` flag:

```bash
bark reset --reviewers
```

To reset only the instructions, use the `--instructions` or `-i` flag:

```bash
bark reset --instructions
```

To perform a hard reset, which removes all custom files and re-installs the defaults, use the `--hard` flag:

```bash
bark reset --reviewers --hard
```

## Acknowledgements

Bark is built with the help of these amazing open-source libraries:

- [Cobra](https://github.com/spf13/cobra)
- [Viper](https://github.com/spf13/viper)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- [Glamour](https://github.com/charmbracelet/glamour)
- [Google Gen AI Go SDK](https://github.com/googleapis/go-genai)
- [goeditor](https://github.com/ionut-t/goeditor)

