# Python Best Practices Guide

You are an expert in Python, writing clean, idiomatic, and maintainable code following Python best practices and PEP conventions.

## Python Language Best Practices

- Follow [PEP 8](https://peps.python.org/pep-0008/) style guide
- Use Python 3.10+ features when available (pattern matching, union types, etc.)
- Write Pythonic code; embrace "There should be one obvious way to do it"
- Use list/dict/set comprehensions for simple transformations
- Prefer built-in functions and standard library over custom implementations
- Use `pathlib` for file path operations instead of `os.path`
- Format code with `black` or `ruff format`
- Lint with `ruff` or `pylint`

## Type Hints

- Use type hints for all function signatures and class attributes
- Use `from __future__ import annotations` for forward references (Python 3.7-3.9)
- Use modern syntax: `list[str]` instead of `List[str]` (Python 3.9+)
- Use `|` for union types instead of `Union` (Python 3.10+)
- Use `typing.Protocol` for structural subtyping
- Run `mypy` in strict mode for type checking
- Use `TypeAlias` for complex type definitions
- Prefer `collections.abc` types for function parameters

```python
from collections.abc import Sequence, Mapping

def process_items(items: Sequence[str]) -> list[str]:
    """Accept any sequence, return specific list."""
    return [item.upper() for item in items]
```

## Code Organization

- Organize by feature/domain, not by type
- Keep modules focused; one main concept per module
- Use packages to group related modules
- Limit module length to ~500 lines; split when larger
- Order imports: standard library, third-party, local (separated by blank lines)
- Use absolute imports; avoid relative imports except within packages
- Keep `__init__.py` minimal; avoid complex initialization logic

## Naming Conventions

- `snake_case` for functions, variables, methods, modules, packages
- `PascalCase` for classes and exceptions
- `SCREAMING_SNAKE_CASE` for constants
- Prefix private attributes/methods with single underscore: `_private_method`
- Use double underscore for name mangling only when truly necessary
- Avoid single-letter names except for counters and iterators (`i`, `j`, `k`)
- Use descriptive names; clarity over brevity

## Error Handling

- Use specific exception types; avoid bare `except:`
- Create custom exceptions for domain-specific errors
- Use `except Exception as e:` to catch and handle errors
- Re-raise exceptions with context: `raise NewError() from original_error`
- Use `else` clause in try-except for code that runs only if no exception
- Use `finally` for cleanup code that must always run
- Prefer EAFP (Easier to Ask for Forgiveness than Permission) over LBYL

```python
# Good: EAFP
try:
    value = my_dict[key]
except KeyError:
    value = default_value

# Avoid: LBYL
if key in my_dict:
    value = my_dict[key]
else:
    value = default_value
```

## Functions

- Keep functions short and focused; aim for <20 lines
- Use default arguments carefully; never use mutable defaults
- Use `*args` and `**kwargs` judiciously; prefer explicit parameters
- Use keyword-only arguments for clarity: `def func(*, name: str, age: int)`
- Return early to reduce nesting
- Use generators for large sequences or infinite streams
- Document with docstrings following Google or NumPy style

```python
def calculate_total(
    items: list[float],
    *,
    tax_rate: float = 0.0,
    discount: float = 0.0,
) -> float:
    """Calculate total with tax and discount.

    Args:
        items: List of item prices
        tax_rate: Tax rate as decimal (e.g., 0.08 for 8%)
        discount: Discount as decimal (e.g., 0.10 for 10%)

    Returns:
        Final total after tax and discount
    """
    subtotal = sum(items)
    total = subtotal * (1 + tax_rate) * (1 - discount)
    return total
```

## Classes and OOP

- Use dataclasses for simple data containers
- Use `@property` for computed attributes and encapsulation
- Implement `__str__` for user-friendly output, `__repr__` for debugging
- Use `@classmethod` for alternative constructors
- Use `@staticmethod` sparingly; often better as module-level functions
- Prefer composition over inheritance
- Use ABC (Abstract Base Classes) to define interfaces
- Implement context managers with `__enter__` and `__exit__`
- Use slots (`__slots__`) for memory optimization in frequently-instantiated classes

```python
from dataclasses import dataclass
from typing import Protocol

@dataclass
class User:
    """User data container."""
    name: str
    email: str
    age: int

    @classmethod
    def from_dict(cls, data: dict[str, any]) -> "User":
        """Create User from dictionary."""
        return cls(**data)

    @property
    def is_adult(self) -> bool:
        """Check if user is adult."""
        return self.age >= 18


class Storage(Protocol):
    """Storage interface."""
    def save(self, data: str) -> None: ...
    def load(self) -> str: ...
```

## Testing

- Use `pytest` for testing framework
- Place tests in `tests/` directory mirroring source structure
- Name test files `test_*.py` and test functions `test_*`
- Use fixtures for setup and teardown
- Use parametrize for multiple test cases
- Mock external dependencies with `unittest.mock` or `pytest-mock`
- Aim for high test coverage but focus on critical paths
- Use type checking as a form of testing

```python
import pytest
from myapp.calculator import add

@pytest.mark.parametrize("a,b,expected", [
    (1, 2, 3),
    (0, 0, 0),
    (-1, 1, 0),
])
def test_add(a: int, b: int, expected: int) -> None:
    """Test add function with various inputs."""
    assert add(a, b) == expected
```

## Project Structure

```
myproject/
├── src/
│   └── myproject/
│       ├── __init__.py
│       ├── main.py
│       ├── domain/
│       │   ├── __init__.py
│       │   └── models.py
│       ├── services/
│       │   ├── __init__.py
│       │   └── user_service.py
│       └── storage/
│           ├── __init__.py
│           └── database.py
├── tests/
│   ├── __init__.py
│   ├── test_models.py
│   └── test_user_service.py
├── pyproject.toml
├── README.md
└── .gitignore
```

## Dependency Management

- Use `pyproject.toml` for project configuration (PEP 621)
- Use `uv`, `poetry`, or `pip-tools` for dependency management
- Pin exact versions in production: `package==1.2.3`
- Use version ranges for libraries: `package>=1.2,<2.0`
- Keep dependencies minimal; evaluate before adding
- Use virtual environments for all projects
- Document system dependencies in README

## Performance

- Don't optimize prematurely; profile first with `cProfile` or `line_profiler`
- Use built-in functions (often implemented in C): `sum()`, `map()`, `filter()`
- Use sets for membership testing: `item in my_set` vs `item in my_list`
- Use generators for memory efficiency with large datasets
- Use `itertools` for efficient iteration patterns
- Cache expensive computations with `@lru_cache` or `@cache`
- Use `__slots__` for classes with many instances
- Consider NumPy/Pandas for numerical operations

```python
from functools import lru_cache

@lru_cache(maxsize=128)
def fibonacci(n: int) -> int:
    """Calculate nth Fibonacci number with caching."""
    if n < 2:
        return n
    return fibonacci(n - 1) + fibonacci(n - 2)
```

## Common Patterns

### Context Managers

```python
from contextlib import contextmanager

@contextmanager
def timer(name: str):
    """Context manager for timing operations."""
    start = time.time()
    try:
        yield
    finally:
        print(f"{name}: {time.time() - start:.2f}s")

with timer("Processing"):
    process_data()
```

### Enums for Constants

```python
from enum import Enum, auto

class Status(Enum):
    """Order status enumeration."""
    PENDING = auto()
    PROCESSING = auto()
    COMPLETED = auto()
    CANCELLED = auto()
```

### Descriptors for Validation

```python
class PositiveNumber:
    """Descriptor that ensures positive numbers."""

    def __set_name__(self, owner, name):
        self.name = name

    def __get__(self, obj, objtype=None):
        return obj.__dict__.get(self.name)

    def __set__(self, obj, value):
        if value <= 0:
            raise ValueError(f"{self.name} must be positive")
        obj.__dict__[self.name] = value
```

## Best Practices Summary

- **Readability counts**: Write code for humans first, computers second
- **Explicit is better than implicit**: Make intentions clear
- **Simple is better than complex**: Prefer straightforward solutions
- **Use the standard library**: It's comprehensive and well-tested
- **Type everything**: Type hints catch bugs and improve documentation
- **Test comprehensively**: Tests are documentation that never lies
- **Handle errors gracefully**: Fail fast but fail informatively
- **Keep it DRY**: Don't Repeat Yourself, but don't over-abstract

