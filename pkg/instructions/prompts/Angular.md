## Components

- Flag components missing `changeDetection: ChangeDetectionStrategy.OnPush` — it should be the default
- Flag `standalone: true` in component decorators — standalone is the default in modern Angular
- Flag `@HostBinding` and `@HostListener` decorators — use the `host` object in `@Component`/`@Directive` instead
- Flag `ngClass` usage — use class bindings (`[class.foo]="condition"`) instead
- Flag `ngStyle` usage — use style bindings (`[style.color]="value"`) instead
- Flag components with too many responsibilities — suggest splitting into container and presentational components

## Templates

- Flag `*ngIf`, `*ngFor`, `*ngSwitch` — use the native control flow (`@if`, `@for`, `@switch`) instead
- Flag complex expressions in templates — move them to component methods or computed signals
- Flag `<img>` tags not using `NgOptimizedImage` for static images

## State & Reactivity

- Flag constructor injection — use `inject()` instead
- Flag `@Input()` and `@Output()` decorators — use `input()` and `output()` signal functions instead
- Flag `.mutate()` on signals — use `.set()` or `.update()` instead
- Flag derived state computed manually in `ngOnInit` — use `computed()` instead
- Flag component state stored outside signals for components that use `OnPush`

## Subscriptions & Memory Leaks

- Flag subscriptions in components not cleaned up in `ngOnDestroy` — prefer `takeUntilDestroyed()` or the `async` pipe
- Flag nested `.subscribe()` calls — use `switchMap`, `mergeMap`, or `concatMap` instead
- Flag subjects exposed directly from services — expose as `Observable` via `.asObservable()`

## Forms

- Flag template-driven forms where reactive forms would give better control and testability
- Flag form controls accessed directly from the template without typed form groups

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
- Flag `console.log` left in production code paths
- Flag magic numbers and strings that should be named constants
