## Memory & Ownership

- Flag unnecessary `.clone()` calls that could use borrowing instead
- Flag `unsafe` blocks without a comment explaining the invariants that make them sound
- Flag `unsafe` blocks that are larger than necessary — scope them tightly
- Flag `Rc<RefCell<T>>` cycles that could cause memory leaks
- Flag raw pointer usage where a safe abstraction exists

## Error Handling

- Flag `.unwrap()` and `.expect()` in non-test code paths — prefer `?` or explicit handling
- Flag errors silently discarded with `let _ = ...` or `.ok()`
- Flag overly broad `Box<dyn Error>` where a concrete error type would be clearer
- Flag missing error context when propagating errors across boundaries

## Concurrency & Async

- Flag blocking calls (`std::thread::sleep`, synchronous I/O) inside async functions — use `spawn_blocking` for CPU-bound work
- Flag shared state that uses `Mutex` where `RwLock` would be more appropriate
- Flag spawned tasks where a panic would be silently swallowed
- Flag missing `Send`/`Sync` bounds that could cause subtle threading issues

## API Design

- Flag public items missing doc comments, especially `# Errors`, `# Panics`, and `# Safety` sections where relevant
- Flag public enums that could grow but are missing `#[non_exhaustive]`
- Flag `String` parameters that should be `&str`
- Flag `Vec<T>` parameters that should be `&[T]`
- Flag `#[must_use]` missing on types or functions where ignoring the result is almost certainly a bug

## Performance

- Flag unnecessary heap allocations where stack allocation would suffice
- Flag missing `Vec::with_capacity()` when the size is known ahead of the loop
- Flag excessive cloning in hot paths
