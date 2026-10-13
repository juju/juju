# Unit Scale-Down Investigation


## Observed Failure

During `grafana-source-relation-departed`, the charm receives:

```text
ops.model.ModelError: ERROR permission denied (unauthorized access)
```

The failing command is:

```text
relation-get -r 2 - grafana --app --format=json
```

The likely sequence is:

1. `relation-get` observes stale leadership state and selects the leader-only
   local application-settings path.
2. `RemoveUnit` has already revoked the unit's leadership lease.
3. `GetRelationApplicationSettingsWithLeader` returns
   `corelease.ErrNotHeld`.
4. `apiserver/facades/agent/uniter/uniter.go` maps that error to
   `permission denied`.

The race appears to exist in Juju 4.1, but the `unit-scale` branch makes it
easier to trigger:

- Scale mutation marks units Dying before provisioner removal.
- Departure hooks can begin before `RemoveUnit` revokes leadership.
- The provisioner sends both Alive and Dying units through `DestroyUnits`,
  potentially repeating leadership revocation.

The departure-hook contract in
`domain/deployment/charm/hooks/hooks.go` says relation settings should remain
accessible during departure.

## Comparison With Issue #23326

Issue #23326 is related, but it is not the same defect.

### Issue #23326

- A peer `relation_unit` row remains after unit removal.
- `MarkUnitAsDeadWithNoEntities` treats the row as a blocker.
- Removal therefore never reaches `leaveAllRelationScopes`.
- Surviving units continue to see the removed peer and never receive the peer
  `relation-departed` event.

### This Failure

- The `grafana-source-relation-departed` hook is dispatched.
- The hook fails while reading the peer application data bag.
- `relation-get` selects the leader-only path using stale leadership state.
- The API finds that the leadership lease has already been revoked and returns
  `permission denied`.

The defects can interact. If the authorization race causes the departure hook
to fail before the unit leaves its peer scope, issue #23326 can then prevent the
removal backstop from clearing that scope:

1. The leadership authorization race causes the hook to fail.
2. The peer scope remains.

Issue #23326 may therefore become a secondary symptom, but it does not explain
the initial `permission denied` error. The initial error is a separate
leadership/departure ordering race.

## Relevant Files

- `domain/deployment/charm/hooks/hooks.go`: documents relation settings access
  during `relation-departed`.
- `internal/worker/uniter/runner/jujuc/relation-get.go`: chooses between the
  local leader-only and controller settings paths.
- `internal/worker/uniter/runner/context/leader.go`: determines hook leadership
  through `ClaimLeader`.
- `apiserver/facades/agent/uniter/uniter.go`: reads local application settings
  and maps `corelease.ErrNotHeld` to `apiservererrors.ErrPerm`.
- `domain/relation/service/relation.go`: requires the leadership lease in
  `GetRelationApplicationSettingsWithLeader`.
- `domain/removal/service/unit.go`: `RemoveUnit` marks the unit non-Alive and
  revokes leadership.
- `domain/application/state/application.go`: scale mutation marks leaving units
  Dying and contains the synthetic-application scaling guard.
- `domain/application/state/application_test.go`: contains the synthetic
  scaling regression test.
- `internal/worker/caasapplicationprovisioner/ops.go`: sends leaving Alive and
  Dying units through `DestroyUnits`.
- `internal/worker/caasapplicationprovisioner/provisioner_adapters.go`:
  `DestroyUnits` delegates to `removalSvc.RemoveUnit`.

## Earlier Branch Work

- Added a transactional synthetic-application scaling guard in
  `SetCAASApplicationUnitScale`; application-life behavior was deliberately
  unchanged.
- Added `TestSetCAASApplicationUnitScaleRejectsSyntheticApplication`.
- Verified with `go test ./domain/application/state`, `git diff --check`, and
  `gci` on the touched Go files.
- The investigation itself made no code changes.
- `spec.md` was left untouched.
