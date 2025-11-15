When reviewing Zig code, focus on simplicity, explicit control, safety, and adherence to Zig's philosophy of no hidden control flow.

## Code Quality Checks

- Verify all code is formatted with `zig fmt`
- Check for clear, explicit code with no hidden control flow
- Ensure variables and functions have descriptive names
- Look for proper use of `comptime` for compile-time evaluation
- Verify code follows Zig's principle of "no hidden allocations"
- Check that error handling is explicit and comprehensive

## Memory Management

- Verify all allocations use explicit allocators (no hidden allocations)
- Check that all allocated memory is properly freed
- Look for potential memory leaks (missing `defer allocator.free()`)
- Ensure `defer` is used appropriately for cleanup
- Verify `errdefer` is used for error path cleanup
- Check that arena allocators are used appropriately for temporary allocations
- Look for double-frees or use-after-free issues

## Error Handling

- Verify all error-prone operations return error unions
- Check that errors are properly propagated using `try` or explicitly handled
- Look for comprehensive error sets that cover all failure modes
- Ensure error sets are well-defined and documented
- Verify `catch` is used with appropriate fallback logic
- Check that `unreachable` is only used when truly impossible
- Look for missing error cases in error sets

## Safety and Undefined Behavior

- Scrutinize all uses of `@ptrCast`, `@bitCast`, and similar builtins
- Verify array/slice bounds are checked or guaranteed safe
- Check for potential integer overflow (use overflow operators when appropriate)
- Look for null pointer dereferences
- Verify optional unwrapping is safe (using `orelse` or `if` checks)
- Check that type punning is done safely
- Look for data races in concurrent code

## Comptime and Metaprogramming

- Verify `comptime` is used appropriately for compile-time evaluation
- Check that generic functions use `anytype` or explicit types appropriately
- Look for proper use of compile-time reflection (`@TypeOf`, `@typeInfo`)
- Ensure comptime parameters are used to reduce runtime overhead
- Verify inline assembly is well-documented and necessary
- Check that build-time code generation is clear and maintainable

## API Design

- Check that public APIs are well-documented
- Verify allocators are passed as parameters, not hidden
- Look for proper use of `pub` vs private declarations
- Ensure function parameters are in logical order (context first, output last)
- Check that optional types (`?T`) are used appropriately
- Verify slices are used instead of pointers and lengths separately
- Look for proper struct initialization patterns

## Performance Considerations

- Check for unnecessary allocations that could be stack-allocated
- Verify loops are structured for optimal performance
- Look for opportunities to use `comptime` to move work to compile time
- Check that SIMD operations are used where appropriate
- Verify packed structs and bit fields are used correctly
- Look for cache-friendly data layouts

## Testing

- Verify tests are included using `test` blocks
- Check that test names are descriptive
- Look for adequate test coverage including error cases
- Ensure edge cases are tested
- Verify tests clean up allocated resources
- Check that integration tests exercise real-world scenarios

## Common Anti-Patterns

- Hidden allocations (should always pass allocator explicitly)
- Ignoring errors with `catch unreachable` without justification
- Using `unreachable` where errors could occur
- Not freeing allocated memory (missing `defer` or `errdefer`)
- Unnecessary runtime work that could be `comptime`
- Using `@panic` for normal error handling
- Incorrect use of undefined behavior "optimizations"
- Not handling all error cases
- Mixing allocator responsibilities

## Build System and Organization

- Check that `build.zig` is well-structured
- Verify dependencies are properly declared
- Look for appropriate use of build options and configurations
- Ensure cross-compilation settings are correct if used
- Check that build artifacts are organized logically

## Concurrency

- Verify proper use of atomics for concurrent access
- Check that data races are prevented
- Look for proper synchronization primitives
- Ensure thread safety is documented for shared data structures
- Verify that async/await is used correctly (if using async features)

## Documentation

- Verify doc comments are present for public APIs
- Check that allocator requirements are documented
- Ensure error conditions are documented
- Look for examples in documentation
- Verify thread-safety guarantees are documented
- Check that performance characteristics are noted where relevant

## Zig-Specific Best Practices

- Verify consistent use of Zig naming conventions (snake_case for functions/variables)
- Check that result locations are used efficiently
- Look for proper use of sentinel-terminated slices
- Ensure packed structs have correct bit layouts
- Verify that build-time configuration is used instead of preprocessor macros
- Check that cross-compilation is considered in platform-specific code

## Dependencies and Standard Library

- Question use of external dependencies (Zig favors minimal dependencies)
- Verify standard library is used appropriately
- Check that platform-specific code is properly conditionally compiled
- Look for proper use of `std.builtin` for platform detection
- Verify that allocator abstractions are used consistently
