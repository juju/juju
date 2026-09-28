---
myst:
  html_meta:
    description: "Juju unit reference: unit declaration, persistence (the record and its satellites, states, types), execution (creation, leadership, hook resolution, removal, watchers), and rules."
---

(unit)=
# Unit

In Juju, a **unit** is a deployed {ref}`charm <charm>`: one running instance of an {ref}`application <application>`. An application's units occupy {ref}`machines <machine>`.

Simple applications may be deployed with a single unit, but it is possible for an individual application to have multiple units running on different machines. For example, one may deploy a single MongoDB application, and specify that it should run three units (with one machine per unit) so that the replica set is resilient to failures.

A unit is always named on the pattern `<application>/<unit ID>`, where `<application>` is the name of the application and the `<unit ID>` is its ID number or, for the leader unit, the keyword `leader`. For example, `mysql/0` or `mysql/leader`. Note: the number designation is a static reference to a unique entity whereas the `leader` designation is a dynamic reference to whichever unit happens to be elected by Juju to be the leader.

(the-units-declaration)=
## Units in the declaration layer

You add a unit to an application explicitly (`juju add-unit` --
deploying an application creates its units implicitly), and you
resolve a unit's hook errors or remove it through the same clients;
adding or removing a unit requires {ref}`model write access
<user-access-model-write>`.

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>`, {ref}`Terraform Provider for Juju | Manage units <tfjuju:manage-units>`
```

(the-units-persistence)=
(the-unit-record)=
(the-unit-in-the-data-model)=
(the-unit-states)=
## Units in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Unit attributes
:alt: The unit's stored tables as an entity-relationship slice: the unit record at the centre with its uuid, name, life, application pointer, net-node pointer and pinned charm revision; the application record it joins west with the shared net node below it; the agent and workload status records east; the subordinate co-location pair south. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory (the password hash, drawn nullable, is a field, not a pointer).
:caption: Entity relationship diagram: The unit's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The unit belongs to its application and shares its machine's net node (that shared identity is what "runs on" means in the data model); the subordinate pair is a record of two unit pointers; the two status records -- the agent's and the workload's -- hang off the unit.
```

In the model database a unit is a **native record** (the DDL:
`0020-unit.sql`): one running instance of an application, created by
the deployment or add-unit machinery. Rather than one monolithic row,
the unit's story is distributed across the record and the satellites
that point at it:

* **the identity pair (`unit.uuid` + `unit.name`):** a unique, hidden
  id (`uuid`) is the anchor the other records point at, while the
  natural key (`name`, UNIQUE) is the user-facing identity the client
  types -- the application's name plus `/` and the unit number (`0`
  or a positive integer without leading zeros: `mysql/0`, never
  `mysql/01`). The creation service rejects a duplicate with `unit
  already exists`.
* **the security token (`unit.password_hash`):** a UNIQUE field
  preventing any unit from impersonating another; because NULLs count
  as distinct, units created without a password break no uniqueness.
* **the pinned charm revision (`unit.charm_uuid`):** a unit pins its
  charm pointer directly onto its own row, so a
  {ref}`refresh <the-application-refresh>` rewrites the
  {ref}`application's <application>` pointer and leaves existing
  units on the revision they were deployed from until they are
  individually refreshed.
* **the parent application (`unit.application_uuid`):** the mandatory
  pointer connecting the unit back to its parent application, from
  which it derives its context.
* **the shared net node (`unit.net_node_uuid`):** the mandatory
  pointer to the network identity the unit shares with its machine or
  pod. Sharing this identity is the data model's literal definition
  of what it means to "run on" something.
* **the life (`unit.life_id`):** the shared alive / dying / dead
  cycle. A unit is created alive; the removal machinery marks it
  dying (guarded one-way) and declares it dead only once no relation
  scopes and no storage attachments are left -- a scheduled removal
  job then deletes the records (see {ref}`Unit removal
  <the-unit-removal>`).
* **the agent status (`unit_agent_status`):** written by the **unit
  agent** (its own loop states: `idle`, `executing`, ...), but it
  cannot set `lost` or `allocating`, and an `error` write must carry
  a message.
* **the workload status (`unit_workload_status`):** written by the
  **workload** (the charm, through its hooks: `active`,
  `maintenance`, ...). The **controller** writes the initial statuses
  at unit creation (`allocating` / `waiting`) and computes the
  display overrides: an agent not seen recently reads as `lost`, and
  a lost agent's workload reads as `unknown` unless the workload
  itself is in error or terminated. The pair `juju status` shows as
  `<workload status>/<agent status>` (the vocabularies:
  {ref}`unit status <unit-status>`; the who-writes story across all
  five status domains: the {ref}`Status domains <status>` view).
* **the subordinate pair (`unit_principal`):** two pointers into the
  unit table -- the subordinate's (`unit_uuid`, the pair's primary
  key) and the principal's (`principal_uuid`). Subordinate helpers
  are regular units; this record is what pairs them (see
  {ref}`Subordinate unit <subordinate-unit>`).

Not drawn above, and asserted by the schema just the same: the
charm-claimed state triple (`unit_state` / `unit_state_charm` /
`unit_state_relation` -- the state the unit's charm has claimed
through its hooks), the resolution mode recorded when a hook error is
cleared (`unit_resolved`), presence (the controller updates it as the
unit agent logs in and out, see {ref}`the unit agent <unit-agent>`),
and the workload/agent versions. None of the statuses is
transition-validated: every status write is a membership check (the
value must be known) followed by writer gates -- what constrains a
unit is who writes which value, not a transition matrix.

The unit table has no type column: a unit's kind is derived, and the
kinds are not mutually exclusive -- the leader is also just one of the
application's units. Leadership, likewise, is not stored on the unit:
it is a lease in the controller database (see {ref}`Leader unit
<leader-unit>`). Most units are **regular units**: one charm
instance, running its hooks on its machine or pod.

(types-of-unit)=
(leader-unit)=
### Leader unit

In Juju, a **leader** (or {ref}`application <application>` leader) is the application {ref}`unit <unit>` that is the authoritative source for an application's status and configuration.

All units for a given application share the same charm code, the same relations, and the same user-provided configuration but the leader unit is different in that it is responsible for managing the lifecycle of the application as a whole.

Every application is guaranteed to have at most one leader at any given time. {ref}`Unit agents <unit-agent>` will each seek to acquire leadership, and maintain it while they have it or wait for the current leader to drop out.

Internally, even though the replica set shares the same user-provided configuration, each unit may be performing different roles within the replica set, as defined by the {ref}`charm <charm>`.

The leader is denoted by an asterisk in the output to `juju status`.

Leadership is not stored on the unit: it is a **lease** in the
controller database, one per application, held by a unit until it
expires or is revoked. The unit agents claim and renew the lease; the
controller validates it on every leader-gated operation -- the
application status write, the peer relation settings, secret access
(see {ref}`Unit operations <the-unit-operations>` and
{ref}`the unit agent <unit-agent>`).

(subordinate-unit)=
### Subordinate unit

A **subordinate unit** is a unit of a
{ref}`subordinate charm <subordinate-relation>`: it runs co-located
with a principal unit on the same machine, and the pair is recorded in
a dedicated principal record. Like the application's
subordinate-ness, this is a role, not a type -- it is derived from the
charm's metadata and the co-location record.

(the-units-execution)=
(the-unit-operations)=
## Units in the execution layer

By the time the command returns, the unit's records exist -- and the
compute it asks for may still be provisioning. A unit has machinery
of its own: the unit agent runs on it -- claiming leadership and
running the charm -- while in the controller, creation, hook-error
resolution and removal act on the unit's records. Operations on units
split by owner: the controller creates units and resolves their hook
errors; the unit agents claim leadership and run the charm; the
removal machinery tears units down.

### Unit creation

Units are created implicitly by deploying an application or explicitly
by adding units (for example, `juju add-unit mysql -n 2`): the
controller validates the application is alive, writes the unit records
with their initial statuses, and asks for the compute the placement
asks for (see {ref}`Application deployment
<the-application-deployment>` and {ref}`machine
<machine>`). On Kubernetes, a unit the user registers directly (a
manually started pod) is registered into the model rather than
scheduled.

### Leadership

A unit agent claims the application's leadership lease when it wants
to lead and renews it while it holds it; the lease expires if the unit
stops renewing, and it is revoked when the unit is removed. Holding
the lease is what gates the leader's writes: the application status
write, the peer relation settings read, and secret access all check
leadership, and fail with a not-leader error if the unit no longer
holds it.

### Hook results and resolution

The unit agent is the only writer of the unit's hook-claimed state
(see {ref}`the unit agent <unit-agent>` and
{ref}`hook execution <hook-execution>`): each hook's changes commit
transactionally, with a leadership caveat where the change requires
the leader. When a hook errors, the user (or the charm, on the
leader) resolves it -- the resolution mode is recorded on the unit and
honoured by the agent on the next run.

(the-unit-removal)=
### Unit removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Unit removal
:alt: User calls juju remove-unit. Controller marks unit Dying and fires watcher to unit agent. Unit agent runs stop, teardown, and remove hooks, then marks unit Dead. Controller releases machine and deletes unit records.
:caption: Sequence diagram: Removal is a cooperative shutdown. The controller only marks the entity Dying; the agent that owns it runs its teardown work and only then reports itself Dead. A hook error in that teardown is what the `--force` option overrides.
```

Removing a unit (`juju remove-unit`) is a cooperative shutdown, not a
kill: the controller only marks the unit Dying; the unit's agent runs
its teardown work and only then reports itself Dead. A hook error in
that teardown is what the `--force` and `--no-wait` options of
{ref}`removing things <removing-things>` override. When the last unit
of an application on a machine leaves, the machine is removed with it;
the unit's leadership lease is revoked as part of the teardown.

(the-unit-watchers)=
### Unit watchers

Nothing about a unit is polled by the things that act on it: they
watch it. The unit's watch surfaces are exposed by the application
domain's watchable service -- what a watcher fires on, not who
consumes it (see {ref}`the unit agent <unit-agent>`):

- **One unit's life** -- the unit agent's own shutdown watcher: it is
  how the agent learns its unit is dying and starts the teardown
  (see {ref}`Unit removal <the-unit-removal>`).
- **An application's units' life** -- the application-scoped life
  surface.
- **The unit's addresses** and **the units added or removed on a
  machine** -- the address and machine-scoped surfaces.
- **One unit for the legacy uniter** -- the unit, principal and
  resolution state, for the uniter's own reconciliation.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-unit-rules-and-errors)=
## Unit rules and errors

The unit domain encodes its rules as a typed error taxonomy; each
error names the rule it enforces. The rules themselves are stated
where they belong: the name grammar and the password uniqueness in
the persistence layer, the mutation gates in the execution layer's
operations and states. The errors matter to charm authors operating
units and to Juju developers, who maintain them as the domain's
validation law.

The errors that encode them:

- *Existence and life*: `unit not found`, `unit already exists`,
  `unit is alive`, `unit not alive`, `unit is dead`,
  `unit not assigned`.
- *Leadership*: `claim denied`, `claim not held`,
  `unit is not the leader`.
- *Composition*: `unit has subordinates`,
  `unit has storage attachments`,
  `unit already has subordinate`.
