# Juju Architectural Rules for Coding Agents

Juju is a long-lived distributed systems project.
Prioritise correctness, stability, and architectural boundaries over novelty.

## Architectural Priorities

- Strict architectural layering
- Testability

## Core Rule

Respect Juju layering. Never create new cross-layer dependencies.

## Layer Roles

- `domain/`: Business workflows in services; persistence behind state abstractions.
- `apiserver/`: RPC facade implementations. Keep these thin and free of business logic.
- `api/`: Client code for calling RPC facades.
- `internal/worker/`: Workers that converge real-world state towards declared state.
- `cmd/`: CLI commands and utilities.
- `core/`: Shared internal logic. Add here only when functionality is truly cross-cutting.

## Import Rules

- `apiserver` -> may import `domain` and `core`; must not depend on `cmd`.
- `internal/worker` -> may import `domain` and `api`; must not depend on `cmd`.
- `domain` -> must not import `apiserver`, `cmd`, or `internal/worker`.
- `core` -> should only import other `core` sub-packages or external packages; not other Juju packages.

## Concurrency and Goroutine Rules

### Required

- Always propagate `context.Context` and honour cancellation.
- Use existing worker abstractions (`worker.Worker`, `worker.Runner`, etc.).
- For background work and polling loops, use established worker lifecycle patterns.

### Prohibited

- Unmanaged goroutines, especially without cancellation.
- Starting goroutines inside API handlers.
- Blocking operations in hot paths.
- Client, cross-controller, or third-party connections without deterministic closure.

## API Facade Rules

- Keep facades to thin orchestration.
- Facades handle auth, wire encoding/decoding, and domain service calls.
- Do not place business logic in facades.

## Domain Service and State Rules

- Keep business logic in the service layer.
- Services depend on state interfaces/indirections, not concrete implementations.
- Do not leak transaction or database details out of state packages.
- Put logic in state only when it must execute inside a transaction.
- State sub-packages must use Sqlair for query and mutation.
- State method arguments should be simple types (`string`, `int`, etc.) or types local to that domain.

## Worker Boundaries

- Workers must be restartable, deterministic, and cancellation-aware.
- Manage worker main loops with `tomb.Tomb` or `worker.Catacomb`.

## Change Discipline

- Follow existing local conventions before adding abstractions.
- When modifying a subsystem, read neighbouring files for consistency.
- Prefer minimal diffs that improve safety, clarity, and correctness.
- Avoid new global state.
- Avoid new dependencies unless clearly justified.
- Do not introduce new patterns or abstractions unless clearly necessary.
