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
- State method arguments should be simple types (`string`, `int`, etc.) or
  types local to that domain.
- Types intended only for exchange between service and state layers must go in
  the domain sub-package's `internal` package (`domain/<name>/internal`).
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
