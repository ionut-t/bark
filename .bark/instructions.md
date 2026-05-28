You are an expert in Go, writing idiomatic, performant, and maintainable code following Go best practices and community conventions.

## Go Latest Version

- 1.26+

## Go Language Best Practices

- Follow the [Effective Go](https://go.dev/doc/effective_go) guidelines
- Keep functions small and focused on a single task
- Prefer clarity over cleverness; write code that's easy to read
- Use meaningful variable names; avoid single-letter names except for short-lived loop indices and receivers

## Code Organization

- Organize code by domain/feature, not by type (avoid `models/`, `controllers/` directories)
- Keep package names short, lowercase, single-word when possible
- Avoid stuttering: `user.UserService` → `user.Service`
- Place related types and functions in the same file
- Use internal packages to prevent external imports of implementation details
- Limit package scope; expose only what's necessary

## Error Handling

- Always check errors; never ignore them with `_`
- Return errors as the last return value
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Use custom error types for errors that need programmatic inspection
- Handle errors at the appropriate level; don't just pass them up blindly
- Use `errors.Is()` and `errors.As()` for error type checking
- Avoid panic except for truly unrecoverable situations

## Naming Conventions

- Use `MixedCaps` or `mixedCaps` rather than underscores
- Exported names start with capital letter, unexported with lowercase
- Interface names: single-method interfaces end in "-er" (`Reader`, `Writer`)
- Use short receiver names (1-2 letters), be consistent within a type
- Getters don't use "Get" prefix: `obj.Name()` not `obj.GetName()`
- Setters use "Set" prefix: `obj.SetName(name)`

## Concurrency

- Don't communicate by sharing memory; share memory by communicating
- Use channels to coordinate goroutines, mutexes to protect shared state
- Always handle channel closure; use the comma-ok idiom or range
- Avoid goroutine leaks: ensure all goroutines can exit
- Use `context.Context` for cancellation and timeouts
- Use `sync.WaitGroup` to wait for goroutines to complete
- Prefer `sync.RWMutex` over `sync.Mutex` when reads vastly outnumber writes
- Keep critical sections (locked code) as small as possible

## Interfaces

- Accept interfaces, return structs
- Keep interfaces small; prefer many small interfaces over large ones
- Define interfaces where they're used, not where they're implemented
- Don't define interfaces before you need them; let them emerge
- Use `io.Reader`, `io.Writer`, and other standard interfaces when applicable

## Structs and Methods

- Use pointer receivers when the method modifies the receiver
- Use pointer receivers for large structs to avoid copying
- Be consistent: if some methods use pointer receivers, all should
- Initialize structs with composite literals or constructor functions
- Use field names in struct literals for clarity (except for very short structs)
- Prefer composition over inheritance; embed types when appropriate

## Testing

- Place tests in `*_test.go` files in the same package
- Use table-driven tests for multiple test cases
- Name tests descriptively: `TestFunctionName_Scenario`
- Use `t.Helper()` for test helper functions
- Prefer `testing.T` over assertion libraries
- Use subtests with `t.Run()` for better organization and parallel execution
- Mock external dependencies using interfaces
- Keep tests simple and focused; one assertion per test when practical

## Performance

- Don't optimize prematurely; measure first with profiling
- Use `sync.Pool` for frequently allocated objects
- Preallocate slices when size is known: `make([]T, 0, size)`
- Use `strings.Builder` for concatenating strings in loops
- Be mindful of allocations in hot paths

## Dependencies

- Keep dependencies minimal; evaluate before adding
- Review `go.sum` changes in code reviews

## Common Patterns

- Use functional options pattern for configuration
- Use context for request-scoped values and cancellation
- Implement `String()` method for debugging (Stringer interface)
- Use `defer` for cleanup (closing files, unlocking mutexes)
- Handle initialization in `init()` sparingly; prefer explicit initialization
- Use zero values effectively; design structs to be useful when zero-valued

## CLI UX

- Error messages must be actionable: tell the user what went wrong and how to fix it. "reviewer not found" is bad; "reviewer 'foo' not found — run bark reviewer list to see available reviewers" is good.
- Mutually exclusive flags must be enforced with `MarkFlagsMutuallyExclusive`, not in runtime logic.
- Flags that require other flags should use `MarkFlagRequired` or validate early with a clear message.
- Default values should reflect the most common use case. Require explicit opt-in for destructive or expensive operations.
- Long-running operations must give the user feedback (spinner, progress) — never silently hang.

## Bubble Tea / TUI

- Never block the `Update` method — offload I/O and computation to commands (`tea.Cmd`).
- Messages (`tea.Msg`) must be immutable value types; do not share pointers between components.
- Component state should be self-contained; avoid reaching into parent or sibling models.
- Clean up subscriptions and background commands in the final `Update` before returning `tea.Quit`.
- Prefer returning `tea.Batch` over chaining updates to avoid dropped messages.

## LLM Integration

- Always apply a timeout to LLM API calls — a hanging request must not hang the process indefinitely.
- Streaming responses must handle partial chunks and EOF cleanly; do not assume chunks are complete sentences or valid markdown.
- LLM API errors (rate limits, quota exceeded, invalid key) must produce clear user-facing messages, not raw API error strings.
- Prompts that include user-controlled content (diff, custom instructions) must not allow that content to override the system-level reviewer persona.
- Token limits: truncate or summarise diffs before sending; never silently send an oversized payload.

## Security

- API keys must never appear in logs, error messages, or `--help` output.
- Validate that required secrets are present before doing any work — fail fast at startup, not mid-operation.
- When shelling out (git, gh CLI), avoid constructing commands by string concatenation; use `exec.Command` with separate arguments to prevent injection.
- Do not store or cache API keys beyond the lifetime of the process.
