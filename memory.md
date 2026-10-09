# Unit-backed Kubernetes scale review progress

## Scope

- Review branch `unit-scale` at `08a734fc49` against the untracked `spec.md`.
- Branch base is `4606c096e84` (`4.1`); the current working tree contains the scale range fix and its regression test.
- Read root `AGENTS.md`, architectural and core-domain rules, and nested `domain/AGENTS.md` and `domain/schema/AGENTS.md`. Used Semble before code exploration.
- Updated the untracked `spec.md` during this review. It now states the current implementation assessment and the required invariants, and its test-oriented task list was removed as requested.
- Current follow-up: each new scale unit carries its required planned name, and `SetCAASApplicationUnitScale` has spacing and comments separating its transaction stages. `changeCAASApplicationUnitScale` now delegates plan construction, next-ordinal selection, and argument creation to service helpers. Focused and full affected race tests passed after the refactor, and concurrency stress completed with no failures.

## Remaining findings

- None identified against the reviewed scale contract.

## Resolved findings

- The service checks that the existing Alive range is contiguous and gives each proposed addition a required name. With no Alive units, it derives the first name from the sequence high-water mark. State compares each allocated name within the transaction; a mismatch returns retryable `ScalingStateInconsistent`, rolling back both unit and sequence writes. No unit-name parsing or new core unit usage is needed in state. The state regression rejects `{foo/0, foo/6}`, verifies the Alive set and sequence remain unchanged, and confirms `foo/6` is valid after scale-to-zero because no range remains to extend.
- The application-status fixtures now insert the singleton model row with the correct machine or Kubernetes type before creating applications (`domain/status/state/model/modelstate_test.go`). Normal model creation and both legacy and current import bootstrap establish this row before application data. The query's model-type join is retained. A new regression checks that Kubernetes status scale counts only Alive units and reports zero after all units become Dying; existing machine status tests still require a nil scale.
- Stale scale plans now pass the full expected Alive names into `SetCAASApplicationUnitScale`, which checks exact membership inside the transaction before any update. Equal counts alone cannot hide a departure and replacement. Relative requests retry a conflicting plan up to three attempts using a fresh Alive read. The state test exercises `{foo/0, foo/1}` changing to `{foo/1, foo/2}` before the stale removal runs, and the service test verifies replanning after a conflict. A comment at the state check explains why comparing counts is insufficient.
- The orphan StatefulSet replacement now explicitly creates at zero replicas, independent of `InitialScale`, until the provisioner reconciles its reserved unit range (`internal/provider/kubernetes/application/application.go:301-347`). The old ordinal range may no longer be intended; the provisioner first reserves current Alive units and then scales the replacement, so zero is a safe temporary size. The existing orphan replacement test asserts the zero value while using `InitialScale: 3` (`internal/provider/kubernetes/application/application_test.go:1036-1103`).
- The stale reservation path no longer inserts absent identities. `ReserveCAASUnits` now calls `CompleteCAASUnitStorage`, which transactionally requires the selected unit UUID to remain Alive in the application and has no unit-creation path (`domain/application/service/provider.go:440-510`, `domain/application/state/unit.go:556-586`).
- The model-status summary now counts only Alive units (`domain/status/state/model/modelstate.go:1998-2004`), with a Dying/Dead regression test (`domain/status/state/model/modelstate_test.go:2373-2395`).

## Invariants confirmed by source review

- Absolute and relative scale requests derive their current target from Alive units. The state transaction validates their full expected Alive identity set before applying the plan.
- Scale-down marks the lowest Alive ordinals Dying and retains the highest target range. A later scale-up extends that range; the state transaction verifies contiguity after allocating ordinals. With no retained Alive units, the next sequence ordinal can start a new range.
- The provisioner derives the provider's desired range from Alive membership, checks contiguity, uses the sequence above zero when no unit remains, verifies configured start ordinal and actual pod names, and reconciles when a worker starts (`internal/worker/caasapplicationprovisioner/ops.go:624-744`, `internal/worker/caasapplicationprovisioner/application.go:276-281`).
- Contiguity applies only to Alive intended membership. During a scale-down
  from `{0,1,2}` to `{2}`, asynchronous departure may temporarily leave
  `{0(Dying),2(Alive)}` if unit `1` is removed first. The provisioner does not
  inspect or change the provider range while any departure remains; after unit
  `0` is gone it reconciles the StatefulSet to `[2,3)`. This is covered by
  `TestEnsureScaleWaitsForEveryDepartureBeforeChangingRange` in
  `internal/worker/caasapplicationprovisioner/ops_test.go`.
- Pod registration requires an existing reserved unit and rejects an unreserved identity (`domain/application/service/provider.go:549-607`).
- StatefulSet state exposes configured start ordinal and `ScaleRange` patches both replicas and ordinal start (`internal/provider/kubernetes/application/application.go:1529-1583`, `internal/provider/kubernetes/application/scale.go:50-96`).
- Controller nonces are ensured before the provider range is increased on the normal reconciliation path (`internal/worker/caasapplicationprovisioner/ops.go:699-706`, `:1013-1028`).
- Legacy model migration rejects an accepted scale request that is not represented by imported units; imported unit and pod identities advance, but never lower, the sequence high-water mark (`domain/application/service/migration.go:253-328`). Same-level imports validate legacy `application_scale` rows against Alive units before writes (`domain/modelimport/validate.go:20-40`).
- New model exports omit `application_scale`; the type remains only to accept older payloads (`domain/export/types/v4_1_0/model.go:1633-1641`).

## Checks performed

- At `2f07fcf16d1`, `go test -race ./domain/application/... ./domain/modelimport/... ./internal/worker/caasapplicationprovisioner/...` passed.
- `go test -c -race -o /tmp/caasapplicationprovisioner-review.test ./internal/worker/caasapplicationprovisioner` passed.
- `timeout 31 stress -timeout 30s /tmp/caasapplicationprovisioner-review.test` ran 705 iterations with 0 failures; process exit 124 was the outer timeout.
- `git diff --check 4606c096e84...HEAD` passed.
- At `e6835267838`, `go test -race ./domain/application/... ./domain/status/state/model ./internal/worker/caasapplicationprovisioner/...` failed: application and worker packages passed, but 13 existing `TestGetApplicationAndUnitStatuses*` status cases failed because the query returned no applications. The status test suite's common setup does not insert a model row; its separate model-info tests do so explicitly.
- `go test -race -run 'TestModelStateSuite/TestGetApplicationAndUnitStatuses' ./domain/status/state/model -count=1` reproduced the failures. `go test -race -run 'TestModelStateSuite/TestGetApplicationAndUnitModelStatusesExcludesNonAliveUnits$' ./domain/status/state/model -count=1` passed, confirming the former issue 3 fix.
- The orphan replacement test and full `internal/provider/kubernetes/application` package passed with `go test -race` after the change. `gci` ran on both touched Go files.
- The stale-plan regression, relative replan, and existing scale tests passed under `go test -race` in `domain/application/state` and `domain/application/service`; both full packages also passed. The generated service mock was refreshed for the state interface change.
- `timeout 31 stress -timeout 30s /tmp/juju-application-state-scale-review.test -test.run 'TestApplicationStateSuite/TestSetCAASApplicationUnitScaleSerializesConcurrentRequests$'` completed 801 runs with 0 failures. Exit 124 came from the outer timeout.
- `go test -race ./domain/application/...` completed successfully across all packages. The user requested an explicit outer timeout for future test commands; apply it to every subsequent test run. This broad run had already finished before the request arrived.
- `timeout 240s go test -race -run 'TestModelStateSuite/TestGetApplicationAndUnitStatuses' ./domain/status/state/model -count=1 -timeout 3m` passed, including the new Alive-only scale regression.
- `timeout 300s go test -race ./domain/status/state/model -count=1 -timeout 4m` passed.
- `timeout 240s go test -race -run 'TestApplicationStateSuite/TestSetCAASApplicationUnitScale' ./domain/application/state -count=1 -timeout 3m` passed with the new range regression.
- `timeout 360s go test -race ./domain/application/state ./domain/application/service ./internal/worker/caasapplicationprovisioner -count=1 -timeout 5m` passed.
- After the service refactor and required-name plan, `timeout 300s go test -race -run 'TestApplication(State|Service)Suite/(TestSetCAASApplicationUnitScale|TestSetApplicationScale|TestChangeApplicationScale)' ./domain/application/state ./domain/application/service -count=1 -timeout 4m` passed.
- After the refactor, `timeout 360s go test -race ./domain/application/state ./domain/application/service ./internal/worker/caasapplicationprovisioner -count=1 -timeout 5m` passed.
- `timeout 360s go test -race ./internal/worker/caasapplicationprovisioner -count=1 -timeout 5m` passed after adding the asynchronous departure-order regression.
- `timeout 180s go test -race -run 'TestApplicationServiceSuite/TestSetApplicationScale' ./domain/application/service -count=1 -timeout 2m` passed after adding an assertion for the planned name following an existing Alive range.
- `timeout 240s go test -c -race -o /tmp/juju-application-state-scale-plan.test ./domain/application/state` passed. `timeout 31s stress -timeout 30s /tmp/juju-application-state-scale-plan.test -test.run 'TestApplicationStateSuite/TestSetCAASApplicationUnitScaleSerializesConcurrentRequests$'` ran 779 iterations with 0 failures; exit 124 was the outer timeout.
- `timeout 240s go test -c -race -o /tmp/juju-application-state-range.test ./domain/application/state` passed. `timeout 31s stress -timeout 30s /tmp/juju-application-state-range.test -test.run 'TestApplicationStateSuite/TestSetCAASApplicationUnitScaleSerializesConcurrentRequests$'` ran 804 iterations with 0 failures; exit 124 was the outer timeout.
- `timeout 31 stress -timeout 30s /tmp/kubernetes-application-orphan-review.test -test.run 'TestApplicationSuite/TestEnsureStatefulDeletesOrphanedStatefulSet$'` ran 883 iterations with 0 failures; exit 124 was the outer timeout.
- `make pre-check` skipped static analysis by default. An explicit static-analysis run stopped at a sandbox `yq` runtime-directory error; the user said no further pre-check run is needed.
- `gci` has run on every touched non-generated Go source file and `git diff --check` currently passes, including the scale range fix.

## Review limits

- Do not report a potential dead-application import failure without a supported import path. Model migration precheck requires applications and units to be Alive, so the reviewed migration path cannot supply that state.
- Every future test command needs an outer timeout.
