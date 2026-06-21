## Type Safety

- Flag use of `any` where a proper type is possible — prefer `unknown` for truly unknown values
- Flag missing return types on exported functions
- Flag type assertions (`as T`) without a runtime check justifying them
- Flag `@ts-ignore` and `@ts-expect-error` comments without an explanation
- Flag non-null assertions (`!`) where a null check or optional chaining would be safer

## Error Handling

- Flag unhandled promise rejections — every `async` function or `.then()` chain needs a `.catch()` or try/catch
- Flag `async` functions that catch errors and swallow them silently
- Flag `try/catch` blocks with an empty or logging-only `catch` that discards the error

## Async & Concurrency

- Flag `await` inside loops where `Promise.all()` would run operations in parallel
- Flag promises created but not awaited or returned
- Flag mixing callbacks and promises in the same flow

## Security

- Flag user input passed to `eval()`, `new Function()`, or `innerHTML` without sanitisation
- Flag template literals used to build SQL queries or shell commands — use parameterised queries and argument arrays
- Flag sensitive data logged to the console or included in error responses

## Code Quality

- Flag functions longer than ~30 lines that could be meaningfully split
- Flag deeply nested callbacks or promise chains that could be flattened with `async/await`
- Flag barrel re-exports (`index.ts`) that create circular dependency risks
- Flag `console.log` left in production code paths
- Flag magic numbers and strings that should be named constants
