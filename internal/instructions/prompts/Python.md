## Type Safety

- Flag missing type hints on function signatures and class attributes
- Flag use of `Any` where a more specific type is possible
- Flag `# type: ignore` comments without an explanation
- Flag untyped `**kwargs` in public APIs where the shape is known

## Error Handling

- Flag bare `except:` or `except Exception:` that swallows errors silently
- Flag exceptions caught and discarded with `pass` — at minimum log them
- Flag missing cleanup in error paths — prefer `with` statements and `finally`
- Flag re-raised exceptions that lose the original traceback — use `raise ... from ...`

## Common Bugs

- Flag mutable default arguments — `def f(x=[])` is a classic footgun
- Flag variables that shadow built-ins (`id`, `list`, `type`, `input`, etc.)
- Flag boolean comparisons using `==` instead of `is` for `None`, `True`, `False`
- Flag `except` clauses that catch more than they handle

## Security

- Flag `eval()` or `exec()` with any user-controlled input
- Flag `subprocess` calls constructed by string concatenation — use argument lists
- Flag hardcoded credentials or secrets

## Code Quality

- Flag global mutable state
- Flag functions longer than ~30 lines that could be meaningfully split
- Flag deeply nested logic that could be flattened with early returns or guard clauses
- Flag missing `__all__` in public modules
- Flag blocking I/O in async functions — use `asyncio`-compatible alternatives
