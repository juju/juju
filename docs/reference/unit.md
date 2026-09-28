---
myst:
  html_meta:
    description: "Juju unit reference: unit declaration, persistence (the record and its satellites, the leader and subordinate kinds), execution (creation, leadership, hook execution and resolution, removal, watchers), and rules."
---

(unit)=
# Unit

In Juju, a **unit** is a deployed {ref}`charm <charm>`: one running instance of an {ref}`application <application>`. An application consists of one or more units, and its units occupy {ref}`machines <machine>` (containers and pods included). A unit is named on the pattern `<application>/<unit number>` (`mysql/0`); the number is the stored, static identity, and the `leader` keyword is a dynamic alias: it names, in operations only, whichever unit currently holds the leadership lease, never in the stored record.

(the-units-declaration)=
## Units in the declaration layer

- **Explicit creation:** The add-unit operation; it requires model {ref}`write access <user-access-model-write>`.
- **Implicit creation:** Deploying an application creates its units; on Kubernetes the application's scale target drives unit creation, and the provisioner brings the pods into being (see {ref}`Application scaling <the-application-scaling>`).
- **Placement:** On machine clouds an added unit can name the machine or container it runs on (the {ref}`machine <machine>` designations); on Kubernetes the count is what the scale target carries.
- **Removal and resolution:** Removing a unit and resolving its hook errors are controller-API operations; both require model write access.

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>`, {ref}`Terraform Provider for Juju | Manage units <tfjuju:manage-units>`
```

(the-units-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - The name is `<application>/<unit number>`: the application's name plus `/` and a number that is `0` or a positive integer without leading zeros (`mysql/0`, never `mysql/01`).
  - The `leader` keyword is the operations alias for the current leader; the stored name is always the number.
  - The name is unique per model; the creation path mints the next number from the application's sequence.
  - Every adding, removing and resolving operation requires model write access.
- **Errors:**
  - **`unit already exists`:** Triggered when the operation names a unit name the model already uses. Remediation: let the creation path mint the next number, or name an unused one.
  - **`invalid unit name`:** Triggered when a name fails the grammar. Remediation: use the application's name, `/`, and a number without leading zeros.

(the-units-persistence)=
## Units in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Unit attributes
:alt: The unit's stored tables as an entity-relationship slice: the unit record at the centre with its uuid, name, life, application pointer, net-node pointer and pinned charm revision; the application record it joins west with the shared net node below it; the agent and workload status records east; the subordinate co-location pair south. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory (the password hash, drawn nullable, is a field, not a pointer).
:caption: Entity relationship diagram: The unit's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The unit belongs to its application and shares its machine's net node (that shared identity is what "runs on" means in the data model); the subordinate pair is a record of two unit pointers; the two status records -- the agent's and the workload's -- hang off the unit.
```

In the model database a unit is a native record (the DDL: `0020-unit.sql`), created by the deployment, add-unit or scale machinery. Its story is distributed across the record and the satellites that point at it:

- **`unit`:** The core row: the identity pair (the `uuid` the other records point at, the `name` the client types, unique per model), the life, the parent application pointer, the shared net node pointer, the pinned charm pointer, and the password hash (unique across all units, so no unit can impersonate another; NULLs count as distinct, so units created without one break no uniqueness).
- **`unit_principal`:** The subordinate co-location pair: two pointers into the unit table, the subordinate's (the pair's primary key) and the principal's.
- **`unit_workload_version`:** The workload version the unit reports.
- **`unit_agent_version`:** The unit agent's reported version and architecture; `v_unit_target_agent_version` joins the model's target agent version onto it.
- **`k8s_pod`, `k8s_pod_port`, `k8s_pod_status`:** The Kubernetes pod a CAAS unit runs in: the provider ID (the pod's name), the ports the pod exposes, and the pod's provisioning status.
- **`unit_agent_status`:** The unit agent's health report, its message and data (the agent vocabulary: `allocating`, `executing`, `idle`, `error`, `failed`, `lost`, `rebooting`).
- **`unit_workload_status`:** The workload's health, written by the charm through its hooks (the {ref}`workload vocabulary <workload--charm-status>`), its message and data.
- **`unit_agent_presence`:** The agent's logins: the controller updates `last_seen` on every API connection; the status views join it in, and the display overrides read it.
- **`unit_resolved` + `resolve_mode`:** The resolution mode recorded when a hook error is cleared (`retry-hooks`, `no-hooks`).
- **`unit_state`, `unit_state_charm`, `unit_state_relation`:** The charm's claimed local state, committed with the hook's transaction.

The two status satellites are the pair the status display reads. The agent's status is written by the **unit agent**; the workload's by the **workload**, the charm through its hooks. Writer gates, not transitions, are what constrain them: the agent cannot write `lost` or `allocating`, and an `error` write must carry a message. At read time, an agent with no presence row displays as `lost`, and a lost agent's workload displays as `unknown` unless the workload itself is in `error` or `terminated` (the vocabularies: {ref}`unit status <unit-status>`; the who-writes story across all five status domains: the {ref}`Status domains <status>` view).

The unit table has no type column: a unit's kinds are derived from the records around it, and they are not mutually exclusive; most units are **regular units**: one charm instance, running its hooks on its machine or pod.

### Leader unit

In Juju, a **leader** (or {ref}`application <application>` leader) is the application {ref}`unit <unit>` that is the authoritative source for the application's status and its peer relation settings. Every application has at most one leader at a time. Leadership is not stored on the unit: it is a **lease** in the controller database, one per application, held by a unit until it expires or is revoked. The unit agents claim and renew the lease; the controller validates it on every leader-gated operation (see {ref}`Leadership <the-units-leadership>`).

### Subordinate unit

A **subordinate unit** is a unit of a {ref}`subordinate charm <subordinate-relation>`: it runs co-located with a principal unit on the same machine, and the pair is recorded in the `unit_principal` record. Like the application's subordinate-ness, this is a role, not a type: it is derived from the charm's metadata and the co-location record.

(the-units-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - Every satellite's foreign key points at the unit UUID; the co-location pair's two pointers both land in the unit table.
  - The charm pointer is pinned per unit: a {ref}`refresh <the-application-refresh>` rewrites the {ref}`application's <application>` pointer and leaves existing units on the revision they were deployed from until they are individually refreshed.
  - **Life:** Created alive; marked dying one-way by the removal machinery; dead only once no relation scopes and no storage attachments are left; the scheduled removal job then deletes the records (see {ref}`Unit removal <the-unit-removal>`).
  - The statuses are upserts gated by writer, never transitions; the display overrides are computed from presence at read time.
- **Errors:**
  - **`unit not found`:** Triggered when an operation names a unit the model does not have. Remediation: check the unit name and the model.
  - **`unit is not alive`:** Triggered when a storage-attachment operation targets a unit that is dying or dead. Remediation: none; the unit is being removed.
  - **`unit is dead`:** Triggered when a status or relation operation targets a unit whose life is dead. Remediation: none; the records are being deleted.
  - **`unit status not found`:** Triggered when reading a unit's agent or workload status before its first write. Remediation: wait for the first status write.

(the-units-execution)=
## Units in the execution layer

By the time the operation returns, the unit's records exist, and the compute it asks for may still be provisioning. Operations on units split by owner: the controller creates units and resolves their hook errors; the unit agents claim leadership and run the charm; the removal machinery tears units down.

### Unit creation

Units are created implicitly by deploying an application or explicitly by adding units: the controller checks the application is alive, writes the unit records, and asks for the compute the placement asks for (see {ref}`Application deployment <the-application-deployment>`). On machine clouds the next unit number is minted from the application's sequence; on Kubernetes the provisioner reconciles the pod count to the scale target, and a pod the model did not ask for is refused at registration.

(the-units-leadership)=
### Leadership

A unit agent claims the application's leadership lease when it wants to lead, and renews it while it holds it; the lease expires when the unit stops renewing, and lapses when the unit is removed. The lease lives in the controller database, one per application. Holding it is what gates the leader's writes: the application status write, the peer relation settings read, and secret access all check leadership and refuse with `unit is not the leader` when the unit no longer holds it.

### Hook execution and resolution

The unit agent is the only writer of the unit's hook-claimed state (see {ref}`hook execution <hook-execution>`): each hook's changes commit transactionally, against a life precondition (a commit when the unit's life has moved on fails with `unit life predicate failed`) and a leadership caveat where the change requires the leader. Over a unit's lifetime the agent runs, in order: `install`, once before any other hook; `config-changed`, immediately after it and again on every configuration change, after an upgrade, and when recovering from transient agent errors; `start`, immediately after the first `config-changed` (and again on Kubernetes pod churn); `leader-elected` or `leader-deposed` when leadership moves; the {ref}`relation <relation>` and secret hooks as their changes arrive; and on teardown the relation hooks, then `stop`, the last hook of a started unit, then `remove`. The flags and environment of each hook command live on the {ref}`hook command <hook-command>` pages.

When a hook errors, the unit's agent status reads `error`; a client resolves it, requiring model write access, by recording a mode: `retry-hooks` re-runs the failed hook on the agent's next pass, `no-hooks` skips it. Only a unit in error can be resolved, and the marker is cleared once the agent has honoured it.

(the-unit-removal)=
### Unit removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Unit removal
:alt: User calls juju remove-unit. Controller marks unit Dying and fires watcher to unit agent. Unit agent runs stop, teardown, and remove hooks, then marks unit Dead. Controller releases machine and deletes unit records.
:caption: Sequence diagram: Removal is a cooperative shutdown. The controller only marks the entity Dying; the agent that owns it runs its teardown work and only then reports itself Dead. A hook error in that teardown is what the `--force` option overrides.
```

Removing a unit is a cooperative shutdown, not a kill: the removal machinery marks the unit dying, departs its relation scopes and schedules the storage teardown; the unit's agent runs its stop and remove hooks and only then reports itself dead; the scheduled removal job deletes the records, and the unit goes dead only once no relation scopes and no storage attachments are left. The force flag skips that wait: the job departs the scopes and deletes the records without waiting for the agent's teardown (see {ref}`removing things <removing-things>`). When the removed unit was the last one on its machine, the machine is marked dying and scheduled for removal in the same cascade.

### Unit watchers

Nothing about a unit is polled by the things that act on it: they watch it. The application domain's watchable service exposes the unit's surfaces, what a watcher fires on, not who consumes it (see {ref}`the unit agent <unit-agent>` for the consumers' side):

- **One unit's life:** The unit agent's own shutdown watcher: how the agent learns its unit is dying and starts the teardown (see {ref}`Unit removal <the-unit-removal>`).
- **An application's units' life:** The application-scoped life surface.
- **One unit's addresses and their hash:** The address changes, and the compact address-hash signal.
- **The units added or removed on a machine:** The machine-scoped surface.
- **One unit for the legacy uniter:** The unit, principal and resolution state, for the uniter's own reconciliation.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change (see {ref}`the watcher pattern <watchers>`).

(the-units-execution-rules)=
### Execution rules and errors

- **Rules:**
  - Every adding, removing and resolving operation requires model write access; the leader owns the application status write, the peer relation settings read and secret access.
  - Creating a unit requires an alive application; on Kubernetes the pod count reconciles to the scale target, and unrequested pods are refused at registration.
  - Only a unit whose agent status is `error` can be resolved, and the resolution mode is honoured once, then cleared.
  - Removal is cooperative: the machinery marks dying, the agent tears down, and the job deletes the records only once no relation scopes and no storage attachments remain; the force flag skips the wait.
- **Errors:**
  - **`unit is not the leader`:** Triggered when a leader-gated operation runs on a unit that no longer holds the lease. Remediation: none; wait for leadership to settle, or run the operation on the current leader.
  - **`leadership claim denied`:** Triggered when a unit claims a lease another unit holds. Remediation: none; the agent retries until the lease frees up.
  - **`leadership lease not held`:** Triggered when an operation acts on a lease the unit has already lost. Remediation: none; re-acquire the lease first.
  - **`unit is not in error state`:** Triggered when resolving a unit whose agent status is not `error`. Remediation: resolve only units in error.
  - **`unit not resolved`:** Triggered when clearing a resolve marker the unit does not have. Remediation: none; there is nothing to clear.
  - **`unit agent status not found`:** Triggered when resolving a unit that has no agent status yet. Remediation: wait for the first status write.
  - **`unit not assigned`:** Triggered when a Kubernetes pod the infrastructure announced is not one the model asked for (its ordinal is beyond the scale target, or no scale is in progress). Remediation: let the provisioner settle the pod count to the scale target.
  - **`unit life predicate failed`:** Triggered when a hook-state commit expects a life the unit no longer has. Remediation: none; the agent re-runs once the unit's life settles.
