# Unit-backed Kubernetes application scale

## Summary

Use unit identities as the requested scale of a Kubernetes application, so scaling intent has one durable source instead of a parallel `application_scale` record.

## Problem Statement

Juju stored scale and ordinal metadata separately from units and pods. Those values could drift during asynchronous departure, pod registration, and Kubernetes reconciliation. The extra representation made interruption and recovery harder to reason about.

## Goals

- Express requested scale through intended units, including units awaiting pods.
- Preserve safe scale-down, scale-to-zero for applications other than the one hosting the Juju controller, ordinal identity, and recovery after worker restarts.
- Keep the user-facing scale contract and storage retention behaviour.

## Non-Goals

This does not change machine applications, stateless deployments, application deletion, Kubernetes storage policy, or the public scale command.

## Affected Domain

The change belongs primarily in Juju's StatefulSet-backed Kubernetes application domain, provisioner, and provider integration, including the controller application. The controller owns unit state and reconciliation; the pod's container agent introduces its unit through the authenticated application facade. Status, model migration, and scale watchers previously consumed the separate scale record and must use unit state instead. No direct change to JIMM or the Terraform provider is established by this analysis.

## Current State

Before this change, a scale request updated `application_scale.scale`. The provisioner persisted a target and ordinal range, then converged units and the StatefulSet. Pod introduction normally created the matching unit from the pod ordinal; on scale-down, Juju removed the lowest ordinals, retained the highest requested number, and waited for departure before reducing replicas. Scaling an application other than the one hosting the Juju controller to zero retained its application and StatefulSet at zero replicas. The scale path did not delete PVCs, and unit removal did not request storage destruction.

### Old table mappings

The removed fields should be understood as projections of unit state, not independent state to maintain:

| Field | Unit-backed projection |
| --- | --- |
| `scale` | Number of non-Dead units with a `k8s_pod` row. |
| `scale_target` | Number of non-Dead units, including those without a `k8s_pod` row. |
| `scaling` | Whether those two counts differ. This records a pod-registration gap, not full Kubernetes convergence. |
| `start_ordinal` | Lowest ordinal of a non-Dead unit, whether or not it has a `k8s_pod` row. It is undefined with no such unit; the sequence supplies the next ordinal. |

Requested capacity is a separate count: Alive units. Dying units remain in the old-field projections until removed, but are no longer requested. A `k8s_pod` row proves registration, not that Kubernetes is currently running the pod. Historical values need not match these projections because the old `scale` field was also written when a request arrived.

Relative scale requests are applied once. If concurrent membership changes make
the request stale, the request returns its conflict to the caller; Juju does
not retry `ChangeApplicationScale` because a retry could apply the relative
change to a different scale than the user requested. The user must retry.

### Places that can drift

- The old `scale` could record a request before unit or pod rows changed; `scale_target` and `scaling` recorded provisioner progress. Their stored values could therefore differ from the unit-backed projections above.
- `start_ordinal`, the unit rows, and the application sequence each describe part of ordinal history. Pod introduction can insert an explicit ordinal without advancing the sequence, so these records need separate reconciliation to avoid reusing an identity or targeting the wrong range.
- `k8s_pod` rows record introductions, while the StatefulSet specifies and runs pods independently. On scale-up today, a failed introduction can leave a pod without a unit row; a registration row does not prove its pod is still running. Neither row count proves Kubernetes has reached the requested scale.

## Evidence

- [Historical scale schema](domain/schema/model/4.0.15-model-release.ddl), [scale worker](internal/worker/caasapplicationprovisioner/ops.go), and [unit life watcher](domain/application/service/service.go).
- [Pod registration and termination](domain/application/service/provider.go), [assignment check](domain/application/state/unit.go), and [unit sequence](domain/application/state/unitstate.go).
- [Optional pod record](domain/application/state/unitstate.go), [pod information read](domain/application/state/unit.go), and [unit life watcher](domain/lifewatcher.go).
- [StatefulSet scaling](internal/provider/kubernetes/application/scale.go), [provider state](caas/application.go), [pod watcher](internal/provider/kubernetes/application/application.go), [provisioner event loop](internal/worker/caasapplicationprovisioner/application.go), [unit removal](internal/worker/caasapplicationprovisioner/provisioner_adapters.go), and [pod record deletion](domain/removal/state/model/unit.go).
- [Status read](domain/status/state/model/modelstate.go), [legacy application import](domain/application/modelmigration/import.go), [sequence import](domain/sequence/modelmigration/import.go), and [same-level export fields](domain/export/types/v4_1_0/model.go).
- [Machine cloud-instance schema](domain/schema/model/sql/0017-machine-cloud-instance.sql) keeps the provider instance ID optional until creation.

## Validated Assumptions

- **Confirmed:** `k8s_pod` records identify registered pods; they do not establish current Kubernetes replica count. The provider's StatefulSet state is needed to confirm convergence.
- **Confirmed:** Scale-to-zero can leave no unit records while retaining the application, StatefulSet, and storage not requested for destruction.
- **Confirmed:** Unit creation permits no `k8s_pod` row, pod registration can complete an existing unit, and pod-information reads return missing fields as zero values. This establishes a pending-unit state, not universal consumer compatibility.
- **Confirmed:** Previously, scale-up could start pods before their unit rows existed, so failed pod introduction could leave a temporary gap. Reserving all intended units first removes that failure mode.
- **Confirmed:** The unit life watcher can report deletion of a known unit, but a worker starting after the last deletion cannot infer unfinished scaling from that event. The old persisted `scaling` flag supplied the restart cue; reconciliation now needs to run on worker start.
- **Confirmed:** Scale-down retains the highest ordinals. `{0,1,2}` scaled to one becomes `{2}`; a later scale-up becomes `{2,3}`, not `{0,3}`.
- **Confirmed:** Contiguity applies to the Alive intended range, not to the
  combined set of Alive and departing units. When `{0,1,2}` scales to `{2}`,
  both `0` and `1` become Dying. If `1` is removed first, the transient state
  `{0(Dying),2(Alive)}` is valid. The provisioner keeps the existing
  StatefulSet range until `0` also departs, then reconciles it to `[2,3)`.
- **Confirmed:** Only the application hosting the Juju controller is barred from scaling to zero. Provider state must expose replica count, pod names, and the configured StatefulSet start ordinal.
- **Confirmed:** Model import carries sequences; legacy application import also carries old scale and provisioning state. Older same-level payloads contain `application_scale`, units, and `k8s_pod` rows. Unit and pod rows provide the old-field projections, while the sequence preserves allocated ordinals.
- **Partially supported:** The imported sequence alone may lag explicitly registered or imported unit ordinals. Reconcile it with imported unit/pod identities before using it as the high-water mark.

## Domain Model and Terminology

An **intended unit** is an Alive unit selected to exist, whether or not its pod has registered. A `k8s_pod` row records that registration. A Dying unit is no longer intended but still counts as non-Dead until cleanup completes; a Dead unit does not count in the old-field projections. The StatefulSet's replica count and ordinal range describe Kubernetes intent; its observed replicas describe provider progress. The sequence records ordinals allocated through it, not scale.

## Key Constraints and Invariants

- A pod may claim only an intended unit identity; existing authentication and assignment checks must remain effective.
- A unit without a pod is pending, not a running replica; status and other consumers must preserve that distinction.
- Old-field projections exclude Dead units. Requested capacity counts only Alive units, so Dying departures cannot inflate it.
- Commit every intended unit and its sequence allocation before increasing the provider's ordinal range. Pod introduction attaches to a reserved unit; it does not create scaling intent.
- StatefulSet ordinals must form a valid contiguous range without reusing an identity that still has a pod or storage.
- Only Alive intended units must form a contiguous range. Dying units may
  temporarily leave gaps in the combined unit set because their removal order
  is asynchronous; the provider range must not change until every departure
  outside the intended range has completed.
- Scale-down removes the lowest ordinals and retains the highest requested number, including when some units have not yet registered pods. Subsequent scale-up allocates above the retained range.
- Once a unit is Dying, a later scale request cannot revive it. New capacity requires a fresh ordinal.
- The controller application's per-ordinal nonce must be available before its pod starts.
- Termination and restart decisions must use intended unit membership and the provider's ordinal range, not compare an ordinal with a replica count.
- Departure completes before its pod is stopped; reconciliation must survive retries, restarts, and the disappearance of the last unit record without a persisted `scaling` flag.
- Scale-to-zero must retain current StatefulSet and storage semantics for other applications; the application hosting the Juju controller remains barred from zero, and application removal remains a distinct lifecycle.

## Proposed High-Level Approach

Treat the set of Alive Kubernetes units as requested capacity. Commit the complete contiguous set of new unit reservations and sequence allocations before scaling the StatefulSet; failed pod introduction then leaves a pending unit, not a missing identity. On scale-down, make the lowest ordinals Dying so the highest requested number remain, then remove departing units after cleanup. A newer request changes the Alive set but cannot revive Dying units; replacements use new ordinals. Reconcile the StatefulSet only to a contiguous intended range after departures are safe, and verify its configured range and observed pods against Kubernetes rather than `k8s_pod` row counts. Reconcile on worker start, unit creation/life/deletion, and provider pod changes; retry incomplete transitions even if an event is missed. At zero, the intended set is empty, while the sequence retains the ordinal high-water mark. On import, use the imported units, `k8s_pod` rows, and sequence rather than creating units from old scale counts. A legacy migration with a scale request not yet reflected in its units must wait until that request settles or be rejected; otherwise dropping the old row would silently lose it.

## Domain Fit

Units already carry identity and lifecycle, while the provisioner owns convergence with Kubernetes. Keeping requested capacity with those identities aligns scaling with the entities whose departure must be protected. This follows the machine pattern: a machine has an identity before `machine_cloud_instance` has a provider instance ID. Likewise, an intended unit exists before its `k8s_pod` is registered. The sequence can retain ordinal history when no units remain.

## User-Visible Behavior

The scale command should still report the requested count and converge to that many pods. Scale-down retains the highest ordinals, so `{0,1,2} → {2} → {2,3}` across a down-then-up sequence. Status must continue to report requested capacity accurately, without treating pod registration as proof of running replicas. Scaling any application other than the one hosting the Juju controller to zero should stop pods while leaving the application, StatefulSet, and retained storage available for later scale-up; only the controller-hosting application remains barred from zero.

## Risks and Tradeoffs

Pre-created units may affect consumers that equate unit existence with a registered pod; those paths need an explicit pending-unit contract. Dropping `application_scale` changes the source for watchers, termination decisions, status, and model export/import. Legacy migration must detect an unmaterialised scale request instead of treating an old scalar as a unit count or silently discarding it. Reordering reservation after provider scale would recreate the old gap. Provider state must expose the configured start ordinal to verify the range. A missed wake-up or unreconciled sequence could leave replicas running or reuse an old identity. No new credential or trust boundary is intended, but pod assignment remains security-sensitive.

## Open Questions

None for the proposed scaling contract. Failed pod introduction leaves a reserved unit pending; import establishes intended units before reconciliation.

## Suggested Next Decisions

Audit pending-unit consumers and validate that legacy migration requests have settled before discarding old scale metadata. Retain imported sequences, enforce reservation-before-provider ordering, make startup reconciliation independent of a scale flag, and keep requested scale observable through Alive units.

## Tasks

1. Establish the pending-unit contract: commit Alive reservations before provider scale, attach pod registration to reserved identities, and audit consumers that assume every unit has a pod. Test failed introduction of a middle pod while a higher pod registers: all intended units remain, with the failed one pending.
2. Make the sequence authoritative: preserve imported sequences and reconcile their high-water marks with explicit unit/pod ordinals, including at zero. Use imported units rather than synthesising them from old scale counts; test legacy and same-level imports with missing registrations, restart, and non-reuse after zero.
3. Move scale requests to atomic changes of the Alive unit set, retaining the highest ordinals on scale-down while preserving Dying departures and relative-scale concurrency. Test overlapping requests, `{0} → {0,1,2} → {2} → {2,3}`, retries, zero for an ordinary application, and zero rejection only for the controller-hosting application.
4. Derive and reconcile the StatefulSet range from units, expose its configured start ordinal, and replace ordinal-versus-count termination checks with membership and range checks. Never scale the provider before reservations commit; wait for safe departure before stopping pods. Test ordering failures, shifted ranges, interrupted transitions, last-unit deletion, controller nonces, and retained PVCs.
5. Move scale watchers, status, and model export/import to unit-backed intent. Derive the old-field projections from non-Dead units and pod rows, but derive requested capacity from Alive units; reject legacy migration while an accepted scale request is absent from unit state. Test API compatibility, pending/departing status, both import paths and export round trips, and Kubernetes scale-up/down/zero behaviour.
