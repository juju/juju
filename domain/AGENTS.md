# Juju Domain Rules for Coding Agents

These rules apply to `domain/` and its sub-packages and supplement the
[root guidance](../AGENTS.md).

## Domain Service and State Rules

- Keep business logic in the service layer.
- Services depend on state interfaces/indirections, not concrete
  implementations.
- Do not leak transaction or database details out of state packages.
- Put logic in state only when it must execute inside a transaction.
- State sub-packages must use Sqlair for query and mutation.
- Prefer SQL queries that use `WITH` common table expressions instead of
  embedded `SELECT` subqueries.
- SQL queries must use explicit aliases for tables, CTEs, and projected values;
  use `AS` rather than relying on implicit aliasing.
- For changed queries and filters, identify which entities are included or
  excluded. Check missing join rows, empty or zero values, and entities created
  through alternative lifecycle paths. Trace excluded categories downstream
  and verify that exclusions preserve the caller's contract.
- State method arguments should be simple types (`string`, `int`, etc.) or
  types local to that domain.
- Types intended only for exchange between service and state layers must go in
  the domain sub-package's `internal` package (`domain/<name>/internal`).
- Each field transferred across a service and state boundary must have a
  demonstrated consumer. Keep implementation-specific fields with the layer
  that owns them instead of carrying them through shared types unnecessarily.
- Generate new UUIDs in the service layer, then pass them into state methods as
  strings. State should persist supplied UUIDs rather than creating them, so
  services can return created entity UUIDs directly when needed.
- When wrapping errors across layers, add identifying context such as entity
  UUIDs once at the highest useful layer. Keep state-layer `Errorf` messages
  generic to avoid repeated identifiers in the final error chain.
- UUID parameters accepted by service-layer methods should use their typed form
  (e.g. `coremodel.UUID`) whenever such a type exists, and the service method
  must validate them (e.g. `modelUUID.Validate()`) before use.
- State-layer methods must always receive UUIDs as plain `string`; the service
  converts typed UUIDs at the call boundary.
- Domain packages should generally avoid `github.com/juju/names`. Prefer
  converting Juju tags to primitive values at API, facade, worker, or command
  boundaries before calling domain services.
- Values populated inside a `db.Txn` closure MUST remain correct if the closure
  is retried. Reset mutable targets only when a previous attempt can leave
  partial data that would produce duplicate or inconsistent results. Do not add
  redundant resets for retry-consistent assignments or Sqlair `GetAll` targets,
  which Sqlair clears before populating.
- For workflows spanning transactions or databases, establish what remains
  committed if execution stops after each write, and how retry or
  reconciliation completes the operation. Do not assume atomicity across
  controller and model databases. Cover partial completion and recovery in
  tests.

## State Method Naming

- State method names must identify the persistence operation being performed.
- Prefer `Get` for retrieval, whether selecting one entity or a collection:
  `GetThing` and `GetThings`. Using `GetThings` in one state package and
  `ListThings` in another falsely implies different operations. Use `Get`
  consistently for equivalent queries; do not introduce `List` as a synonym.
- Methods that delete persisted data must use `Delete`, including deletions of
  association rows. A method that deletes a thing must be named `DeleteThing`,
  not `ClearThing`.
- Service methods may describe workflow intent, such as `ClearThing`. Their
  state methods must describe the actual persistence operation, so a service's
  `ClearThing` may call state's `DeleteThing`. Names need not match across the
  service and state boundary.
