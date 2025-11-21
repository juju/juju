# Guidelines for Coding Agents Working on the Juju Codebase

Juju is a large, long-lived distributed systems project.  

Correctness, stability, and architectural boundaries are more important than novelty.

## What Juju Is

Juju is an orchestration engine for deploying, managing, scaling and integrating infrastructure and applications.  

Juju code must prioritize:
- **Correctness under concurrency**
- **Eventual consistency where required**
- **Strict architectural layering**
- **Testability**

Avoid introducing new patterns or abstractions unless absolutely necessary.

## Architectural Boundaries

### Core rule: **Respect Juju’s layering. Do not create new cross-layer dependencies.**

Major layers include:
- `domain/` — Application logic consisting of services that utilise states for persistence.
- `apiserver/` — RPC facade implementations; should not contain business logic.
- `api/` — Client functionality for calling RPC facades.
- `internal/worker/` — Actors that converge real world to declared state.
- `cmd/` — CLI commands and utilities.
- `core/` — Shared internal logic; avoid adding functionality here unless truly cross-cutting.

### Import rules
- `apiserver` → may import `domain` and `core` packages, but must not depend on `cmd`.
- `internal/worker` → may depend on `domain` and `api`, but not on `cmd`.
- `domain` → must not import `apiserver`, `cmd` or `internal/worker`.
- `core` → should only import other sub-packages in `core` or external packages; not other Juju packages.

## Concurrency & Goroutine Rules

Juju uses high concurrency and distributed coordination.  

### Required patterns
- Always propagate `context.Context`; never ignore cancellation.
- Use existing worker framework abstractions (`worker.Worker`, `worker.Runner`, etc.).
- For background operations, prefer existing worker patterns instead of ad-hoc goroutines.
- For loops/polling, follow the established worker lifecycle patterns.

### Prohibited patterns
- Unmanaged goroutines or goroutines without cancellation.
- Starting goroutines from inside API handlers.
- Introducing blocking operations in hot paths.
- Client, cross-controller or third-party connections without corresponding closure.

## Juju-Specific Patterns to Respect

### API facades
- These contain *thin orchestration*, not business logic.
- Primary responsibilities are auth, encoding/decoding wire data and calling domain services.

### Domain services
- Domain services are grouped by workflow concern.
- There is a strict layering of service and state.
- Services depend on state indirections; not implementations.
- No transaction or database details leak out of state packages.
- Business logic should be in the service layer, unless strictly required within a transaction, in which case it may
  reside in the state layer.
- State sub-packages must use Sqlair to query and modify data.
- State method arguments should be simple types (string, int, etc.) or types internal to the domain they reside in.

### Workers
- Each worker must be restartable, deterministic, and observe cancellation.
- A worker's main loop must be managed by a `tomb.Tomb` or `worker.Catacomb`.

## Making Changes

### When editing code
- Follow existing local conventions before introducing new abstractions.
- When modifying a subsystem, examine all files in the directory to maintain consistency.
- Prefer minimal diffs that improve clarity, safety, and/or correctness.

### When generating new code
- Use patterns already in that directory.
- Avoid adding new dependencies unless necessary.
- Avoid new global state.