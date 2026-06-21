## Memory Management

- Flag allocations without a corresponding `defer allocator.free(...)` or `defer obj.deinit()`
- Flag missing `errdefer` when cleanup is needed on error paths before the happy-path `defer` fires
- Flag functions that allocate internally without accepting an explicit allocator parameter — Zig avoids hidden allocations
- Flag arena allocator use where individual deallocation would be more appropriate, and vice versa

## Error Handling

- Flag `catch unreachable` used on operations that can legitimately fail — document why it is truly unreachable if used
- Flag ignored error unions — every `!T` return must be handled or explicitly propagated with `try`
- Flag overly broad error sets where a narrower set would improve caller clarity
- Flag `unreachable` in code paths that could be reached under unexpected input

## Safety & Undefined Behaviour

- Flag `@ptrCast` and `@bitCast` without a comment explaining the safety invariant
- Flag unchecked array or slice indexing in code paths that receive external input
- Flag integer arithmetic that could overflow without using the checked or wrapping operators (`+%`, `-%`, etc.)
- Flag optional unwrapping with `.?` where a null check or `orelse` fallback is more appropriate

## Comptime

- Flag runtime computation that could be moved to `comptime` without loss of clarity
- Flag `comptime` blocks with side effects that are hard to reason about
- Flag generic functions using `anytype` where a concrete type or interface would make intent clearer

## API Design

- Flag public functions that allocate without documenting the allocator contract
- Flag slice parameters passed as pointer + length instead of a proper slice type
- Flag missing doc comments on public declarations
