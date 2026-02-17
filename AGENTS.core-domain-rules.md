# Juju Core Domain Rules for Coding Agents

## Core Priorities

- Correctness under concurrency
- Eventual consistency where required

## Life

- Treat life as a first-class value for lifecycle-managed model entities.
- Baseline lifecycle-managed entities include machines, units, applications, and relations.
- Use canonical life types and values only: `core/life.Value` or `domain/life.Life` with `Alive`, `Dying`, and `Dead`.
- Do not introduce extra lifecycle states, magic numbers, or ad-hoc string comparisons.
- Lifecycle progression is monotonic: `Alive -> Dying -> Dead`; never transition backwards.
- `Alive` means the entity participates normally in model workflows.
- Transitioning to `Dying` starts departure work (for example relation departure, storage clean-up, and cloud instance clean-up).
- Make `Alive -> Dying` transitions idempotent. Repeated removal requests for already non-`Alive` entities should generally be no-ops unless the API contract says otherwise.
- Move an entity to `Dead` only after required teardown and dependent clean-up is complete; otherwise return explicit blocking errors.
- Delete entities when `Dead`. Exception: some entities may become `Dead` and be removed in one operation, so `Dead` may not be observed externally.
- Forced removal is an exception path and may bypass parts of normal `Dying` workflow; only use it when explicitly required by the API contract.
- Keep lifecycle cascades transactional where possible so related entities do not split into inconsistent states.
- Keep life lookup mappings consistent across code and schema (`alive`, `dying`, `dead`).

## Watchers and Notifications

- Watch one thing with `NotifyWatcher` (`struct{}` notifications).
- Watch collections with `StringsWatcher` (`[]string` changed identifiers).
- Notifications indicate change, not state. Do not emit full entity state in watcher payloads.
- For collection watchers, emit initial collection members, then deltas.
- Emit changed identifiers only; include only entities relevant to the watcher concern.
- Coalesced notifications are expected: multiple changes between reads may collapse to one notification or one changed identifier.
- Consumers must re-query current state for changed entities before acting.
- Mapper/filter logic may drop events; emitting no identifiers is valid and should not be treated as an error.
