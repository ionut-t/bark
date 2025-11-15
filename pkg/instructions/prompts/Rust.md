When reviewing Rust code, focus on idiomatic patterns, safety, performance, and adherence to Rust best practices.

## Code Quality Checks

- Verify all code is formatted with `rustfmt`
- Check that `clippy` warnings are addressed (especially `clippy::all` and `clippy::pedantic`)
- Look for unnecessary mutability (`mut` should only be used when actually needed)
- Ensure variables have meaningful, descriptive names
- Check for proper use of the type system to prevent errors

## Ownership and Memory Safety

- Verify proper ownership transfer vs. borrowing patterns
- Check for unnecessary `.clone()` calls that could be replaced with borrowing
- Look for potential lifetime issues or overly complex lifetime annotations
- Ensure smart pointers (`Box`, `Rc`, `Arc`, `RefCell`) are used appropriately
- Watch for potential memory leaks with reference cycles in `Rc`/`Arc`
- Verify mutable borrows don't violate borrowing rules

## Error Handling

- Check that errors use `Result<T, E>` instead of panicking
- Verify `?` operator is used for error propagation instead of `.unwrap()`
- Look for `.unwrap()` or `.expect()` in production code paths
- Ensure `.expect()` messages are descriptive when used
- Check that custom error types are well-designed and informative
- Verify error context is preserved when propagating errors
- Look for unhandled error cases in `match` statements

## Safety and Unsafe Code

- Scrutinize all `unsafe` blocks carefully
- Verify safety invariants are documented for `unsafe` code
- Check that `unsafe` blocks are minimally scoped
- Ensure `unsafe` code doesn't leak unsafety through public APIs
- Look for alternatives to `unsafe` code using safe abstractions
- Verify that raw pointer dereferencing is sound

## Concurrency and Async

- Check for proper use of `Send` and `Sync` bounds
- Verify thread-safe sharing uses `Arc<Mutex<T>>` or `Arc<RwLock<T>>`
- Look for potential deadlocks or race conditions
- Ensure async functions don't block the executor
- Check that `spawn_blocking` is used for CPU-bound work in async code
- Verify proper use of channels for message passing
- Look for unhandled panics in spawned tasks

## API Design

- Check that public APIs are minimal and well-documented
- Verify trait implementations for standard traits (`Debug`, `Clone`, `PartialEq`, etc.)
- Look for proper use of `pub`, `pub(crate)`, and private visibility
- Check that function signatures use borrowing (`&T`) appropriately
- Verify builder patterns are used for complex constructors
- Ensure `#[non_exhaustive]` is used for public enums that may grow

## Performance Concerns

- Look for unnecessary allocations in hot paths
- Check for `Vec::with_capacity()` when size is known
- Verify iterators are used instead of manual loops where appropriate
- Look for inefficient string concatenation (should use `String` methods or `format!`)
- Check for excessive cloning that impacts performance
- Verify that borrowed strings (`&str`) are used for parameters instead of `String`

## Testing

- Verify adequate test coverage for new functionality
- Check that tests are well-named and descriptive
- Look for missing edge cases or error path testing
- Ensure `#[should_panic]` tests specify expected panic messages
- Verify integration tests are in appropriate locations
- Check that doc tests compile and run correctly

## Common Anti-Patterns

- Calling `.unwrap()` or `.expect()` without justification
- Using `panic!` for error handling instead of `Result`
- Unnecessary `.clone()` calls due to misunderstanding borrowing
- Ignoring errors with `.ok()` or `let _ = ...`
- Complex lifetime annotations that could be simplified
- Exposing implementation details in public APIs
- Using `unsafe` without proper documentation or justification
- Blocking calls in async functions
- Not matching exhaustively on enums

## Documentation

- Verify all public items have doc comments (`///`)
- Check that doc comments include examples
- Ensure `# Panics`, `# Errors`, and `# Safety` sections are present where appropriate
- Verify links to other items in docs are correct
- Check that module-level docs (`//!`) explain the module's purpose

## Dependencies

- Question unnecessary dependencies
- Check for security advisories using `cargo audit`
- Verify dependency versions are appropriately constrained
- Look for outdated or unmaintained dependencies
- Check that feature flags are used appropriately to keep dependencies optional
