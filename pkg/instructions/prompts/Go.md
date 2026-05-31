## Error Handling

- Flag ignored errors (`_` on error returns)
- Flag errors returned without context — wrap with `fmt.Errorf("context: %w", err)`
- Flag panics used as a substitute for proper error handling
- Flag errors checked with `==` instead of `errors.Is()` or `errors.As()`

## Concurrency

- Flag goroutines without clear ownership or a way to signal completion
- Flag shared mutable state accessed without synchronisation
- Flag channel sends or receives with no way to unblock if the other side exits
- Flag missing `context.Context` in functions that call external services or block

## API Design

- Flag exported identifiers with missing or unhelpful documentation
- Flag stuttering names — `users.UserService` should be `users.Service`
- Flag receiver inconsistency — if a type has any pointer receivers, all methods should use pointer receivers
- Flag interfaces defined at the implementation site; they belong at the usage site
- Flag large interfaces that could be split into smaller, more composable ones

## Code Quality

- Flag `init()` functions with side effects
- Flag global mutable state
- Flag functions longer than ~40 lines that could be meaningfully split
- Flag deeply nested logic that could be flattened with early returns
- Flag single-letter variable names outside of short-lived loop indices and receivers
