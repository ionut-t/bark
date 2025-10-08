# Bark - AI Code Reviewer

Bark is a command-line tool that uses AI to review code. It can help identify potential issues, suggest improvements, and generate commit messages.

## Features

- **AI-powered code review:** Get feedback from an AI assistant.
- **Multiple reviewers:** Choose from a variety of "reviewers" with different personalities, such as Linus Torvalds, Uncle Bob, Guido van Rossum, or Yoda.
- **Custom instructions:** Provide custom instructions to the AI to tailor the review.
- **Review commits, branches, or working directory:** Analyse code at any stage of development.
- **Interactive TUI:** View the review in a user-friendly terminal interface.
- **Generate commit messages:** Automatically generate commit messages based on changes.

## Installation

To install Bark, use `go install`:

```bash
go install github.com/ionut-t/bark@latest
```

## Usage

To review the changes in the current working directory, simply run `bark`:

```bash
bark
```

By default, `bark` will review all changes in the working directory (including untracked). To review only the staged changes, use the `--staged` or `-s` flag:

```bash
bark --staged
```

To compare the current branch to a specific branch, use the `--branch` or `-b` flag:

```bash
bark --branch <branch-name>
```

To select a commit to review from a list of recent commits, use the `--commit` or `-t` flag:

```bash
bark --commit
```

To use a specific reviewer, use the `--as` flag:

```bash
bark --as linus
```

To provide custom instructions to the reviewer, use the `--instructions` or `-i` flag with the name of the instruction file:

```bash
bark --instructions <instruction-name>
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
- [Google Generative AI for Go](https://github.com/google/generative-ai-go)
- [goeditor](https://github.com/ionut-t/goeditor)

