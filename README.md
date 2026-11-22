# Bark - AI Code Reviewer

Bark is a command-line tool that uses AI to review code, generate commit messages, and create pull request descriptions.

![bark reviewing a staged change as Linus Torvalds](assets/demo.gif)

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
go install github.com/ionut-t/bark/v2@latest
```

## Usage

Bark provides three main commands: `review`, `commit`, and `pr`.

### Code Review

To review the changes, run `bark review`:

```bash
bark review
```

By default, `bark review` will analyse all uncommitted changes, including new files (gitignored files are skipped). To review only the staged changes, use the `--staged` or `-s` flag:

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

To provide custom instructions to the reviewer, use the `--instructions` or `-i` flag with the name of a saved instruction, a file path or the instructions as text. Repeat the flag to combine several; they're added in the order given:

```bash
bark review --instructions <instruction-name>
bark review -i security -i ./docs/review-rules.md -i "Keep it brief"
```

Without `-i`, the TUI lets you pick from your saved instructions: press `space` to mark several and `enter` to use them together, or just `enter` to use the highlighted one.

To review a GitHub pull request and post the review on it, use `--pr` with `--comment` (needs the [gh CLI](https://cli.github.com)):

```bash
bark review --pr 42 --comment
```

`--comment` turns on structured findings. Each finding on a line the pull request changes becomes an inline comment, and its fix becomes a suggestion that can be applied with one click. The reviewer's intro and verdict, and findings about other lines, go in the review body.

When Bark has posted a review on the pull request before, `--comment` reviews only the changes since the commit that review was made at, which each posted review records in a hidden comment. Bark diffs those commits locally, so it needs them in the repository (in CI, check out with `fetch-depth: 0`). It reviews the whole pull request instead, and says why on stderr, when:

- no earlier Bark review recorded the commit it reviewed;
- that commit is no longer in the pull request's history, or isn't in the local repository (after a rebase or force-push);
- the branch merged other commits since, such as the base branch;
- the pull request's base changed (retargeted at another branch);
- the last review didn't see all of its diff (cut to fit `max_diff_tokens`, or the model's output ended early), so the next one covers the same changes again.

Bark only trusts earlier reviews posted by a bot (GitHub Actions) or by someone with write access to the repository. If nothing changed since the last review, Bark posts nothing. To review the whole pull request anyway:

```bash
bark review --pr 42 --comment --full
```

A full review of a pull request whose head hasn't changed since the last review is shown but not posted when it raises nothing new (every finding repeats a point raised before or dismissed, or there are none). `--fail-on` still applies to it.

#### Checking findings

Before a structured review (`--structured` or `--comment`) is shown in full or posted, Bark checks its findings with a second model call. The check reads the files the findings point to (whole, or only the lines around each finding for files over 1,500 lines) and must quote the code each finding rests on. Three kinds of finding are set aside:

- **Findings that don't hold up** go in a "Doubted by Bark's check" section, each with the reason.
- **Findings already raised** (with `--pr` only) go in a "Raised before on this pull request" block. These repeat a point from Bark's earlier reviews of the same pull request, whether it was posted, doubted or resolved, unless Bark [resolved it as fixed](#resolving-fixed-threads). Bark finds those points in a hidden comment that each posted review leaves. They aren't posted again, but they're still in the code: the verdict still asks for them to be fixed, and `--fail-on` counts them.
- **Findings dismissed on the pull request** (with `--pr` only) go in a "Dismissed on this pull request" block. See [Dismissing a finding](#dismissing-a-finding).

In a posted review, these sections are collapsed and their findings get no inline comments.

The check adds:

- one call per review that has findings, or [open threads to check for fixes](#resolving-fixed-threads), with the diff and the files it reads as input;
- when the review isn't streamed (plain output, `--comment`) and findings are set aside, one more small call (no code, a few hundred tokens) that rewrites the reviewer's intro and verdict so they don't ask for fixes to those findings.

To turn the check off:

```bash
bark review --check=false            # for one review
bark config --check-findings=false   # for good
```

#### Dismissing a finding

To tell Bark you won't fix a finding, react 👎 to its inline comment or reply "won't fix". Later repeats of it aren't posted, and `--fail-on` doesn't count them.

- Only people with write access can dismiss (owners, members, collaborators). Bots can't.
- Resolving the thread doesn't dismiss: threads are also resolved after a fix (see [Resolving fixed threads](#resolving-fixed-threads)).
- The check decides what's a repeat. To help it, Bark points out findings on the same lines as a dismissed one, if those lines haven't changed since. This needs the earlier review's commit in the local repository (in CI, check out with `fetch-depth: 0`).

#### Resolving fixed threads

With `--comment`, Bark resolves the threads of its own inline comments once the code fixes them, and replies on each with the reason ("Fixed in `abc1234`: …").

It only looks at a thread when:

- nobody replied to it or [dismissed](#dismissing-a-finding) it;
- the lines it was posted on have changed since. In CI, check out with `fetch-depth: 0`.

Bark's check decides, and it must quote the new code that fixes the point. When it isn't sure, the thread stays open and the run's log says why. A thread is never resolved just because a later review didn't raise the point again. The check costs no extra call when the review has findings, and one small call when it has none.

If the problem comes back after Bark resolved it, Bark posts it as a new finding. If you unresolve a thread, Bark leaves it alone.

In GitHub Actions, resolving needs `contents: write` as well as `pull-requests: write`; without it, threads stay open and the run warns. To turn resolving off:

```bash
bark review --pr 42 --comment --no-resolve
```

#### Filtering by severity

To show only the more serious findings, pass `--min-severity` with `critical`, `high`, `medium` or `low`:

```bash
bark review --structured --min-severity high   # for one review
bark config --min-severity high                # for good
```

Less severe findings are dropped before the check, so they aren't checked, shown or posted. The model still writes them, though, so a threshold doesn't make the review cheaper.

#### Failing on findings

To make a review fail when it finds something serious, for example to block a merge in CI, pass `--fail-on` with `critical`, `high`, `medium` or `low`:

```bash
bark review --pr 42 --comment --fail-on high
bark config --fail-on high   # for good (plain reviews only)
```

If the review keeps a finding of that severity or higher, Bark exits with status 3 and lists those findings on stderr. Every other error exits 1, so a script can tell a blocked change from Bark failing. Findings Bark's check doubts don't count. When the check is off or fails, every finding counts.

A finding that repeats a point raised in an earlier review of the same pull request does count: it isn't posted again, but the model found it again in the code, so it's still there. Stderr marks it "raised before, still there".

`--fail-on` turns on structured findings and plain output, since the TUI has no exit status to give. A `fail_on` in a config file applies only when the review runs in plain mode anyway, and turns on structured findings there. With `--comment`, the review is posted before Bark exits, so it's on the pull request when the job fails. Findings below `min_severity` are dropped before they're counted, so a `fail_on` below it warns and works as `min_severity`.

In GitHub Actions, set the `fail-on` input of the `bark-review` action. It posts the review and adds the label, then fails the job.

With `--comment`, a pull request Bark reviewed before is reviewed only from the commits since its last review. A later push that doesn't touch the code an earlier finding is about won't find it again, so it can pass while the issue is still there. Pass `--full` (or set `full-review`) to review the whole pull request each time, so every unfixed finding fails the run again. To get past a finding you won't fix, [dismiss it](#dismissing-a-finding).

#### Approving pull requests

To have Bark approve a pull request when its review raises nothing that should stop it, add `--approve` to `--comment`:

```bash
bark review --pr 42 --comment --approve                       # approve when only low findings remain
bark review --pr 42 --comment --approve --approve-below low   # approve only a review with no findings
bark review --pr 42 --comment --approve --fail-on high        # also fail the job on high or critical findings
```

The review is posted as an approval when:

- it keeps no finding at or above `--approve-below` (`medium` by default), counted the way [`--fail-on`](#failing-on-findings) counts them. Findings below it stay on the approval as inline comments. A `fail_on` lower than `--approve-below` wins, since a review that fails doesn't approve;
- it saw the whole change: the diff wasn't cut to fit its limit, and the model's output didn't end early;
- the model gave a verdict, and Bark could read every finding;
- when it reviewed only the changes since the last review, every point raised in earlier reviews was [dismissed](#dismissing-a-finding), doubted by Bark's check, or resolved: by someone, or by Bark once its check found it [fixed](#resolving-fixed-threads). Pass `--full` (or set `full-review`) to check the rest again each time.
- the pull request doesn't change what Bark reads from its checkout to decide how to review it: anything in a `.bark` folder (config, instructions, reviewer), `.barkignore`, or, with [house rules](#house-rules) on, a house-rules file or the file one points to. Otherwise it could set its own bar, say with a higher `min_severity`. Bark lists the pull request's files from its local commits, so this needs `fetch-depth: 0` in CI.

A later review that doesn't approve dismisses Bark's earlier approvals, saying why: on GitHub, a later comment from the same reviewer doesn't replace an approval. Pushing new commits doesn't trigger Bark, so an approval stays on code Bark hasn't seen; to drop it on every push, turn on "Dismiss stale pull request approvals when new commits are pushed" in the branch's ruleset or protection.

#### House rules

Bark reviews a change against your repository's own rules. By default it reads `AGENTS.md` and `CLAUDE.md`. If your repository has neither, nothing changes.

It looks for each file at the top level and in the folders above the changed files. For a change to `pkg/api/handler.go`, it reads `pkg/api/AGENTS.md`, then `pkg/AGENTS.md`, then `AGENTS.md`.

To pick other files, list them in `.bark/config.toml`, most important first:

```toml
house_rules_files = ["AGENTS.md", "docs/STYLE.md", "CONTRIBUTING.md"]
```

- A name like `AGENTS.md` is looked for in the folders above the changed files, as above.
- A path like `docs/STYLE.md` is read from that place only. Use `./AGENTS.md` for the top level only.
- An empty list means the default files.

Good to know:

- The rules are capped at about 2,000 tokens. Files past the cap are cut or left out, and Bark warns you.
- Your review instructions win if they conflict with the rules.
- A pull request that changes a rule is reviewed under the new rule.

To turn house rules off:

```bash
bark review --house-rules=false   # for one review
bark config --house-rules=false   # for good
```

A repository can turn them off with `house_rules = false` in `.bark/config.toml`.

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

Bark uses a configuration file located at `$XDG_CONFIG_HOME/bark/config.toml` (`~/.config/bark/config.toml` by default, on macOS too). Run `bark config` with no flags to open it in your editor.

Bark keeps its files in the [XDG base directories](https://specifications.freedesktop.org/basedir-spec/latest/):

| Directory               | Default               | Contents                                                                                 |
| ----------------------- | --------------------- | ---------------------------------------------------------------------------------------- |
| `$XDG_CONFIG_HOME/bark` | `~/.config/bark`      | `config.toml`, `commit.md`, `pull_request_description.md`, `reviewers/`, `instructions/` |
| `$XDG_CACHE_HOME/bark`  | `~/.cache/bark`       | `prices.json`                                                                            |
| `$XDG_STATE_HOME/bark`  | `~/.local/state/bark` | `logs/` (`BARK_DEBUG` logs)                                                              |

On Windows the defaults are `%AppData%\bark` for config and `%LocalAppData%\bark` for the cache and logs.

To set your preferred editor, LLM provider, or model, use the `config` command with the appropriate flags:

```bash
bark config --editor nvim
bark config --provider gemini         # `bark config --help` lists the providers
bark config --model gemini-2.5-pro
```

To cap how much of a large diff is sent to the model, set a token limit (0, the default, sends the whole diff). Bark estimates tokens from the diff's size, at 2.5 characters a token, without calling the provider; the estimate errs high, so the diff is cut a little early rather than late. The diff is cut at a line end and marked as truncated. Pass `--max-diff-tokens` to `bark review` or `bark pr` for one run.

```bash
bark config --max-diff-tokens 50000
```

Setting the provider is optional. Bark infers it from the model name: `claude-*` is Anthropic, `gpt-*`, `o3`-style names, `chatgpt-*`, `codex-*` and `ft:` fine-tunes are OpenAI, and `gemini-*` is Gemini or Vertex AI (whichever you configured, otherwise whichever has credentials, Gemini first). Other names, such as Ollama models, use the configured provider, or the first provider whose credentials are set (`GEMINI_API_KEY`, `VERTEXAI_PROJECT_ID` and `VERTEXAI_LOCATION`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, then `OLLAMA_HOST`).

A model chosen at a higher level than the provider wins: with `llm_provider = "gemini"` in your config, `bark review -m claude-sonnet-5-5` uses Anthropic. A provider set at the same level as the model, or a higher one, always wins, so `--provider ollama -m gpt-oss:20b` stays on Ollama.

To use a different config file, pass `--config <path>` to any command. The file must already exist; `bark config --config <path>` reads and saves that file instead of the default:

```bash
bark review --config ~/work/bark.toml
```

### Per-repository config

A repository can share review settings in `.bark/config.toml` at its top level. Its values win over your config file and lose to command-line flags. It can set `llm_provider`, `llm_model`, `max_diff_tokens`, `context_enrichment`, `structured_findings`, `check_findings`, `min_severity`, `fail_on`, `effort`, `house_rules` and `house_rules_files`. Other keys are an error. That includes `editor` and `relative_number`, which stay personal (the editor is run as a command, so a cloned repository can't choose it).

```toml
structured_findings = true
check_findings = true
min_severity = "medium"
max_diff_tokens = 50000
```

If the repository's config picks a different provider or model from yours, Bark prints a warning (a repository's `llm_model` alone can switch your provider, as above) on every run that uses it, because the run uses your credentials for that provider. Pass `--provider`/`--model` to choose your own.

### Large diffs

When a diff is larger than `max_diff_tokens`, Bark keeps the most important files and leaves out the rest. It keeps them in this order:

1. Code
2. Tests, such as `*_test.go`, `*.spec.ts` or files in `tests/`
3. Docs, such as `*.md` or files in `docs/`
4. Fixtures, such as `*.snap` or files in `testdata/`
5. Generated files, marked with a `Code generated ... DO NOT EDIT.` or `@generated` comment

The review starts with a note listing the files it left out. On a pull request, the next run reviews those changes again.

### Ignoring files

Bark leaves lock files and checksums (`*.sum`, `*.lock`, `package-lock.json`, `npm-shrinkwrap.json`, `pnpm-lock.yaml`, `bun.lockb`), generated protobuf code (`*.pb.go`, `*.pb.gw.go`) and minified assets (`*.min.js`, `*.min.css`) out of every diff it sends to a model. The manifest that changed a lock file, such as `go.mod` or `package.json`, is still reviewed. List more paths in a `.barkignore` file at the repository's top level. It uses `.gitignore` syntax:

```gitignore
# Snapshots and generated code
*.snap
internal/gen/
/docs/api.md
testdata/**/*.golden

# Review lock files again
!*.lock
```

A pattern without a slash matches at any depth, and a leading or middle slash anchors it at the top level. A trailing slash matches a directory. `*` and `?` stay within one directory, and `**` crosses them. `!` can only turn off one of the defaults above. `.barkignore` and `.bark/config.toml` are always reviewed, so a pattern that matches either (such as `.bark/`, `.*` or `*`) is not used. In CI a pull request's own copies apply to its review, and this keeps a pull request from hiding files or lowering the bar without the change showing up in the review. Bark prints a warning for lines it can't use and applies the rest. The file applies to reviews, commit messages and pull request descriptions, including `--pr` reviews. Diffs piped on stdin are used as they are.

### Masking secrets

Before anything is sent to the model provider, Bark masks secrets it recognises by their shape: AWS access keys (and secret keys next to their name), GitHub tokens, OpenAI, Anthropic and Google API keys, Google OAuth client secrets, Stripe secret keys and webhook secrets, Slack webhook URLs and tokens, passwords in database and broker URLs (`postgres://user:password@host`, and the same for MySQL, MongoDB, Redis, AMQP and others; not `${DB_PASSWORD}` and other placeholders), and private keys (`-----BEGIN … PRIVATE KEY-----` blocks). This covers every prompt: the diff, the files and related code the check reads, commit messages, pull request descriptions, house rules and instructions. It applies to every provider, Ollama included. It's a pattern match on the text; no model is called.

A masked secret becomes a placeholder that names its kind, such as `[REDACTED:github-token]`, so the review can still point out a hard-coded key. A prompt with masked secrets ends with a note telling the model that the code holds the real secret, not the placeholder. Bark prints a warning on stderr, once for each secret, naming the kind and the file:

```
bark: warning: masked 1 secret(s) before sending them to the model provider: aws-access-key-id (config/prod.env)
```

The TUI prints these warnings when it exits.

In a review with structured findings (on with `--comment`, `fail_on` or `structured_findings`), a secret on a line the change adds is also a critical `security` finding, which Bark makes without asking the model. It comes first in the review (in the TUI too), counts for `--fail-on`, stops `--approve`, and is posted on the pull request like any other finding. The model is told not to report the placeholder itself, so there's no duplicate, but to report a hard-coded secret Bark didn't recognise, such as a password in a connection string. On a pull request, a secret Bark has already reported isn't posted again but still counts, and one you dismiss (👎 or "won't fix", for a fake key in a test, say) is set aside. Secrets on removed or unchanged lines are only warned about.

A suggestion that would put a placeholder into the code isn't offered as one to apply on GitHub. `BARK_DEBUG` logs show the prompts masked, as they were sent. Masking is always on. It catches the shapes above, not every secret, so it doesn't replace a secret scanner.

## Costs

Bark can show what each run's model calls cost. It needs a prices file, which `bark prices` downloads from [models.dev](https://models.dev) and saves to `$XDG_CACHE_HOME/bark/prices.json` (`~/.cache/bark/prices.json` by default):

```bash
bark prices
```

With it, `review`, `commit` and `pr` in plain mode (`--plain`, or when the output is piped, as in CI) print the cost on stderr. A review that checks its findings shows each call's part: the review, the check and, when the check sets findings aside (without `--stream`), the rewritten verdict:

```text
bark: cost $0.0234: review $0.0123, check $0.0101, verdict rewrite $0.0010 (prices from 2026-11-09)
```

In the TUI, the usage overlay (`ctrl+t`) shows the cost of the last task: for a review, the review and its check with each part (the TUI doesn't rewrite the verdict). `BARK_DEBUG=1` logs show each call's cost. Prices are in USD, for the standard paid tier: cached input, thinking and long-prompt prices are counted, but not batch, regional or free-tier pricing. Bark never downloads prices by itself; run `bark prices` again to update them, as providers change their prices. Without a prices file, for a model it doesn't list, and for Ollama, no cost is shown.

Bark reads the repo's `.bark/prices.json` before the one in its cache directory. To show costs in CI, save the prices to the repo from its top-level directory and commit them:

```bash
bark prices --repo
```

`--output <path>` saves them anywhere else.

### Thinking effort

`--effort` sets how hard the model thinks before answering: `low`, `medium` or `high`. Lower is faster and cheaper, but may miss findings; the default leaves it to the model.

```bash
bark review --effort low     # for one run (also bark commit and bark pr)
bark config --effort low     # for good, for every command
```

It applies to every model call in a run, the check included. Bark maps it to each provider's setting:

- **Gemini and Vertex AI:** the thinking level on Gemini 3 and later; a thinking budget of 1,024, 8,192 or 24,576 tokens on Gemini 2.5.
- **OpenAI:** the reasoning effort. Only reasoning models (gpt-5, o-series) accept it.
- **Anthropic:** the effort, with adaptive thinking turned on. Models that don't think by default (Opus 4.6–4.8) think when it's set, so `high` can cost more than leaving it unset.
- **Ollama:** ignored.

A model that can't think may reject the request; Bark doesn't check model names first.

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
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- [Glamour](https://github.com/charmbracelet/glamour)
- [Google Gen AI Go SDK](https://github.com/googleapis/go-genai)
- [goeditor](https://github.com/ionut-t/goeditor)
- [gotreesitter](https://github.com/odvcencio/gotreesitter) - Pure-Go tree-sitter runtime
