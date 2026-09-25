---
myst:
  html_meta:
    description: "Juju unit reference: the unit record, unit kinds, the unit in the data model, unit states, operations, watchers, and rules."
---

(unit)=
# Unit

```{ibnote}
See also: {ref}`manage-units`
```

In Juju, a **unit** is a deployed {ref}`charm <charm>`: one running instance of an {ref}`application <application>`. An application's units occupy {ref}`machines <machine>`.

Simple applications may be deployed with a single unit, but it is possible for an individual application to have multiple units running on different machines. For example, one may deploy a single MongoDB application, and specify that it should run three units (with one machine per unit) so that the replica set is resilient to failures.

A unit is always named on the pattern `<application>/<unit ID>`, where `<application>` is the name of the application and the `<unit ID>` is its ID number or, for the leader unit, the keyword `leader`. For example, `mysql/0` or `mysql/leader`. Note: the number designation is a static reference to a unique entity whereas the `leader` designation is a dynamic reference to whichever unit happens to be elected by Juju to be the leader.

(the-units-records)=
## The unit's records

(the-unit-record)=
### The unit's identity

In the model database, a unit is a record identified by its **name** --
unique per model, the application's name plus `/` and the unit number
-- carrying the unit's life (see {ref}`Unit states <the-unit-states>`),
the application it belongs to, the network identity it shares with
its machine (its net node), and the charm revision it runs: a unit
pins its charm, so a {ref}`refresh <the-application-refresh>` leaves
existing units on the old revision until they are individually
refreshed.

(the-unit-in-the-data-model)=
### The unit in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Unit attributes
:alt: The unit's stored tables as an entity-relationship slice: the unit record at the centre; the application it belongs to west with the shared net node below it; the agent and workload status records east; the subordinate co-location pair south. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The unit's stored records and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The unit belongs to its application and shares its machine's net node (that shared identity is what "runs on" means in the data model); the subordinate pair is a record of two unit pointers; the two status records -- the agent's and the workload's -- hang off the unit.
```

The unit's state is spread across a handful of stored tables. The
`unit` record carries the name, the life, the application, the net
node, the pinned charm revision, and the unit's password (unique
across units -- a unit cannot impersonate another). The
`unit_principal` record is the subordinate co-location pair (see
{ref}`Subordinate unit <subordinate-unit>`); `unit_agent_status` and
`unit_workload_status` are the two status records (see
{ref}`Unit states <the-unit-states>`); the `unit_state`,
`unit_state_charm` and `unit_state_relation` records are the state the
unit's charm has claimed through its hooks; `unit_resolved` records
the resolution mode when a hook error is cleared; and presence is a
separate record the controller updates as the unit agent logs in and
out (see {ref}`the unit agent <unit-agent>`).

(the-unit-states)=
### Unit states

A unit carries orthogonal state machines: its **life** -- the shared
alive / dying / dead cycle every entity has -- and two status
vocabularies, the unit agent's own status and the workload's status,
the pair `juju status` shows as `<workload status>/<agent status>`.
Like the application's, none of these is transition-validated: every
status write is a membership check (the value must be known) followed
by writer gates -- what constrains a unit is who writes which value,
not a transition matrix.

#### Life

A unit is created alive. The removal machinery marks it dying (guarded
one-way) when the unit is removed, and declares it dead only once no
relation scopes and no storage attachments are left on it; a scheduled
removal job then deletes the records (see
{ref}`Unit removal <the-unit-removal>`).

#### Status

The value vocabularies are documented in
{ref}`unit status <unit-status>`; the who-writes story across all five
status domains is the {ref}`Status domains <status>` view. The short
version, as it constrains the unit:

- the **unit agent** writes its own status (`idle`, `executing`,
  ...) -- but it cannot set `lost` or `allocating`, and an `error`
  write must carry a message;
- the **workload** (the charm, through its hooks) writes the workload
  status (`active`, `maintenance`, ...);
- the **controller** writes the initial statuses at unit creation
  (`allocating` / `waiting`) and computes the display overrides: an
  agent not seen recently reads as `lost`, and a lost agent's workload
  reads as `unknown` unless the workload itself is in error or
  terminated.

(types-of-unit)=
### Types of unit

The unit table has no type column: a unit's kind is derived, and the
kinds are not mutually exclusive -- the leader is also just one of the
application's units. Most units are **regular units**: one charm
instance, running its hooks on its machine or pod.

(leader-unit)=
#### Leader unit

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

#### Subordinate unit

A **subordinate unit** is a unit of a
{ref}`subordinate charm <subordinate-relation>`: it runs co-located
with a principal unit on the same machine, and the pair is recorded in
a dedicated principal record. Like the application's
subordinate-ness, this is a role, not a type -- it is derived from the
charm's metadata and the co-location record.

(the-units-machinery)=
## The unit's machinery

A unit has machinery of its own: the unit agent runs on it -- claiming
leadership and running the charm -- while in the controller, creation,
hook-error resolution and removal act on the unit's records.

(the-unit-operations)=
### Unit operations

Operations on units split by owner: the controller creates units and
resolves their hook errors; the unit agents claim leadership and run
the charm; the removal machinery tears units down.

#### Unit creation

Units are created implicitly by deploying an application or explicitly
by adding units (for example, `juju add-unit mysql -n 2`): the
controller validates the application is alive, writes the unit records
with their initial statuses, and asks for the compute the placement
asks for (see {ref}`Application deployment
<the-application-deployment>` and {ref}`machine
<machine>`). On Kubernetes, a unit the user registers directly (a
manually started pod) is registered into the model rather than
scheduled.

#### Leadership

A unit agent claims the application's leadership lease when it wants
to lead and renews it while it holds it; the lease expires if the unit
stops renewing, and it is revoked when the unit is removed. Holding
the lease is what gates the leader's writes: the application status
write, the peer relation settings read, and secret access all check
leadership, and fail with a not-leader error if the unit no longer
holds it.

#### Hook results and resolution

The unit agent is the only writer of the unit's hook-claimed state
(see {ref}`the unit agent <unit-agent>` and
{ref}`hook execution <hook-execution>`): each hook's changes commit
transactionally, with a leadership caveat where the change requires
the leader. When a hook errors, the user (or the charm, on the
leader) resolves it -- the resolution mode is recorded on the unit and
honoured by the agent on the next run.

(the-unit-removal)=
#### Unit removal

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

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

(the-unit-rules-and-errors)=
## Unit rules and errors

The unit domain encodes its rules as a typed error taxonomy; each
error names the rule it enforces. The rules matter to charm authors
operating units and to Juju developers, who maintain them as the
domain's validation law.

The rules a **unit name** must satisfy:

- the application's name, a `/`, and the unit number -- `0` or a
  positive integer without leading zeros (`mysql/0`, never
  `mysql/01`);
- the name is unique per model; the unit's password hash is unique
  across all units.

The rules a **unit mutation** must satisfy:

- units are added only to an alive application, and a unit cannot be
  updated or refreshed once dead;
- a dying unit can only be declared dead once no relation scopes and
  no storage attachments are left on it (`unit has subordinates`,
  `unit has storage attachments` -- see
  {ref}`Unit removal <the-unit-removal>`);
- leader-gated writes fail when the unit no longer holds the lease
  (`unit is not the leader`).

The errors that encode them:

- *Existence and life*: `unit not found`, `unit already exists`,
  `unit is alive`, `unit not alive`, `unit is dead`,
  `unit not assigned`.
- *Leadership*: `claim denied`, `claim not held`,
  `unit is not the leader`.
- *Composition*: `unit has subordinates`,
  `unit has storage attachments`,
  `unit already has subordinate`.

(related-entities-unit)=
## Entities related to the unit

- **Applications** own their units -- one or more, sharing the charm,
  the configuration and the relations (see
  {ref}`application <application>`).
- **Machines** host units: the unit and the machine share the
  machine's net node -- that shared network identity is how Juju knows
  a unit "runs on" its machine (see {ref}`machine <machine>`).
- **The unit agent** is the unit's worker: it claims leadership, runs
  the charm's hooks, commits their results, and watches its unit's
  life (see {ref}`unit agent <unit-agent>`).
- **Leadership** gates the leader's writes through the application's
  lease (see {ref}`Unit operations <the-unit-operations>`).
- **Status** owns the two unit vocabularies and the display overrides
  (see {ref}`Unit states <the-unit-states>`).
- **Removal** owns the teardown that takes a dying unit to dead,
  machine and leadership consequences included (see
  {ref}`Unit removal <the-unit-removal>`).
- **Subordinates** co-locate with their principal through the
  principal pair record (see
  {ref}`Subordinate unit <subordinate-unit>`).
