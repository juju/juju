---
myst:
  html_meta:
    description: "Juju relation (integration) reference: application connections through endpoints. These include regular relations, peer relations, subordinate relations, and cross-model relations, and involve relation databags."
---

(relation)=
# Relation (integration)

```{ibnote}
See also: {ref}`manage-relations`
```

In Juju, a **relation** (**integration**) is a connection an {ref}`application <application>` supports by virtue of having a particular {ref}`endpoint <application-endpoint>`.

## Types of relation

```{ggarch}
:file: ../juju.ggarch
:view: Types of relation
:alt: The relation kinds as a tree: Relation splits into Peer relation and Non-peer relation; Non-peer relation splits into Subordinate relation and Non-subordinate relation; Non-subordinate relation splits into Cross-model relation and Non-cross-model relation. Each edge is labelled with the discriminator that separates the two branches, and ends in a hollow triangle on the parent kind.
:caption: Taxonomy tree: Every relation is a peer relation (the application relates to itself) or a non-peer relation (two distinct applications); a non-peer relation is subordinate when one side is subordinate, and a non-subordinate relation is cross-model when the two applications live in different models.
```

(peer-relation)=
### Peer relation

A **peer** relation is a relation that an application has to itself (i.e., its units respond to one another) automatically by virtue of having a `peers` endpoint.

Because every relation results in the creation of unit and application databags in Juju's database, peer relations are sometimes used by charm authors as a way to persist charm data. When the application has multiple units, peer relations are also the mechanism behind {ref}`high availability <high-availability>`.

(non-peer-relation)=
### Non-peer relation

A **non-peer** relation is a relation from one application to another, where the applications support the same endpoint interface and have opposite `provides` / `requires` endpoint roles.

(subordinate-relation)=
#### Subordinate relation

A **subordinate** relation is a {ref}`non-peer <non-peer-relation>` relation where one application is principal and the other subordinate.

A subordinate charm is by definition a charm deployed on the same machine as the principal charm it is intended to accompany. When you deploy a subordinate charm, it appears in your Juju model as an application with no unit. The subordinate relation helps the subordinate application acquire a unit. The subordinate application then scales automatically when the principal application does, by virtue of this relation.

(the-implicit-juju-info-relation-endpoint)=
##### The implicit `juju-info` relation endpoint

Every application in a model implicitly provides an extra endpoint named `juju-info`, with role `provides`, interface `juju-info`, and global scope. The endpoint is supplied by Juju itself: it does not need to be (and cannot be) declared in the charm's `metadata.yaml` / `charmcraft.yaml`, and it does not appear in `juju info <charm>` or on the charm's Charmhub page. It is supported on every kind of charm, but is only useful for {ref}`machine charms <machine-charm>`, since it exists to allow {ref}`subordinate charms <subordinate-charm>` to attach to a principal that does not otherwise expose a suitable interface.

The `juju-info` endpoint is intended to be consumed by a {ref}`subordinate charm <subordinate-charm>` whose `metadata.yaml` / `charmcraft.yaml` declares an explicit `requires` endpoint with interface `juju-info` (typically with name `juju-info` and `scope: container`). The container scope is what causes the subordinate's unit to be co-located on the same machine as the principal's unit.

The `juju-info` endpoint is the standard mechanism used by general-purpose machine subordinates that need to ride along on every machine of a principal but do not have any application-specific data to exchange. For example, monitoring, logging, or system-administration agents such as [`ntp`](https://charmhub.io/ntp), [`telegraf`](https://charmhub.io/telegraf), [`nrpe`](https://charmhub.io/nrpe), and [`canonical-livepatch`](https://charmhub.io/canonical-livepatch).

The convention is for a subordinate to name its own `requires` endpoint `juju-info`, but any name can be used (for example: `info`, `general-info`); it just needs to use interface `juju-info`.

You integrate against the implicit endpoint the same way as any other endpoint:

```text
juju integrate <subordinate> <principal>
```

If the subordinate also has explicit endpoints whose interfaces match endpoints on the principal, those explicit endpoints take precedence over the implicit `juju-info` one when Juju resolves the relation. To force the implicit endpoint, name it explicitly on either side:

```text
juju integrate <subordinate>:<requires-endpoint> <principal>:juju-info
```

(non-subordinate-relation)=
#### Non-subordinate relation

A **non-subordinate** relation (aka 'regular') is a {ref}`non-peer <non-peer-relation>` relation where the applications are both principal.

(cross-model-relation)=
##### Cross-model relation

```{ibnote}
See also: {ref}`manage-relations`
```

A **cross-model** relation (aka 'CMR') is a {ref}`non-subordinate <non-subordinate-relation>` relation where the applications are on different models (+/- different controllers, +/- different clouds).

Cross-model relations enable, for example, scenarios where  your databases are hosted on bare metal, to take advantage of I/O performance, and your applications live within Kubernetes, to take advantage of scalability and application density.

If the network topology is anything other than flat, the Juju controllers will need to be bootstrapped with `--controller-external-ips`, `--controller-external-name`, or both, so that the controllers are able to communicate. Note that these config values can only be set at bootstrap time, and are read-only thereafter.

A cross-model relation has two sides: the offering side (aka "offerer") and the consume side (aka 'saas'). It does not make a difference which side of the relation (provider or requirer) is the offerer and which is the saas - the two are interchangeable. However, the endpoint type does influence on how juju sets up firewall rules: it is assumed that a requirer is the client and the provider is the server, so ports are opened on the provider side.

Note that application names are obfuscated (anonymised) to the offerer side:
- Applications that relate to the saas appear to the offerer as remote + token, e.g. `remote-76cd96ab50f146b284912afd1cc13a0e`.
- For the consumer, the remote app names is the saas name, e.g. `prometheus`.
##### Non-cross-model relation

A **non-cross-model** relation is a {ref}`non-subordinate <non-subordinate-relation>` relation where the applications are on  the same model.

## Relation identification

A relation is identified by a **relation ID** (assigned automatically by Juju; expressed in monotonically increasing numbers) or a **relation key** (derived from the endpoints, format: `application1:[endpoint] application2:[endpoint]`).

## Relation states and transitions

A relation carries two orthogonal state machines: its **life** -- the
shared alive / dying / dead cycle every entity has -- and its
**relation status**, the relation-specific status the status domain
validates.

### Life

A relation is created alive. It cannot stay alive if either of its
applications stops being alive, and it cannot be declared dead until
every relation unit has left scope. When the relation is removed, the
removal machinery takes over: the relation is marked dying, the units
in scope leave as their agents notice, and a scheduled removal job
finishes the job once nothing is left in scope.

(relation-status)=
### Relation status

The `relation_status` table records one of six status values:
`joining`, `joined`, `suspending`, `suspended`, `broken`, `error`
(defined in the model schema's `relation_status_type`).

- A new relation starts as `joining`; the status row is written when
  the relation record is created.
- `joined` is set by the **leader unit's agent**: when a unit enters
  the relation's scope, only the leader reports the relation as
  joined.
- `suspending` and `suspended` record a suspended relation (see
  {ref}`Suspending and resuming <suspending-relations>`); the
  controller's firewaller also writes relation status as it opens and
  closes the provider's ingress.
- `error` requires a status message.

Each write goes through the status domain's `SetRelationStatus`, which
validates the transition from the current status: every status may
transition to `broken`; `joining` cannot follow `joined` or `broken`;
`suspending` cannot follow `broken` or `suspended`; `joined` and
`suspended` cannot follow `broken`; and `error` cannot be set without
a message.

## Working with relations

### Relation creation

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:alt: User calls juju integrate A B. Controller writes relation record and fires watchers to both unit agents. Each runs relation-created, relation-joined, relation-changed hooks, writing relation data; each data write wakes the other side's watcher for a further relation-changed.
:caption: Sequence diagram: Integrating two applications. The controller writes the relation record; the two units' hooks run in lockstep, each data write waking the other side for another relation-changed.
```


A relation is created by `juju integrate`. Creating the relation
writes a relation record and wakes both sides: each unit's watcher
fires, and the units run their relation hooks in lockstep --
`relation-created`, then `relation-joined` and `relation-changed` --
exchanging data through the databags as they go.

### Relation databag

When you create a relation between two applications, this results in the creation of relation databags. Databags are per relation and per application, and can be application-scoped or unit-scoped. Each unit involved in a relation gets a local copy of all the databags for that relation.

#### Permissions around relation databags

```{ggarch}
:file: ../juju.ggarch
:view: Databag permissions
:no-legend:
:caption: Topology: Each unit reads + writes only its own bag (red); the leader also writes the application bag; all units read the other application's bags (green). Peer case: permissions turn inward -- every unit reads every bag of its own application, application bag included.
:alt: App A's units (appA/leader, appA/1) and app B's units (appB/leader, appB/1) above one row of databags; red arrows reading and writing the own bags (own unit bag; the leader also the application databag), green arrows reading across to the other application's set; below, the peer panel: one application's units with red own/leader arrows and green reads of every bag, the application databag included.
```

While the relation is maintained,

- in a non-peer relation, whether regular or subordinate:
    - each unit can read and write to its own databag;
    - leader units can also read and write to the local application databag;
    - all units of an application can read all of the remote application's databags.
- in a peer relation:
    - each unit can read and write to its own databag;
    - leader units can also read and write to the application databag;
    - all units can read all of the application's databags. That is, whether leader or not, every unit can read its own unit databag as well as every other unit's unit databag as well as the application databag.

Note that, in peer relations, all permissions related to the remote application are turned inwards and become permissions related to the local application.

(relation-removal)=
### Relation removal

A relation is removed with `juju remove-relation`. Removal follows the
same cooperative pattern as every entity removal: the relation is
marked dying, the units in scope leave as their agents notice, and the
removal machinery schedules a removal job that finishes once nothing
is left in scope. A forced removal (`--force`) skips the wait: the
relation and its units' scope entries are torn down without waiting
for the agents to leave gracefully.

Cross-model relations are not removed by this path: the removal
pre-check rejects them, because the local record is only the local
half of the relation.

(suspending-relations)=
### Suspending and resuming

Suspending pauses the data flow across a relation to an application
offer (`juju suspend-relation`; `juju resume-relation` restores it).
The relation records the suspended state and its reason, and the
controller's firewaller closes the provider side's ingress for the
suspended relation. On a cross-model relation the suspension is
propagated to the remote model as well, so both halves agree on the
suspended state.

## Watching relations

Nothing about a relation is polled: agents and clients watch it. The
relation domain's watchable service exposes five watch surfaces:

- **One relation's life and suspended state** -- notifies on changes
  to the given relation's life or suspended status.
- **A unit's relations' life and suspended state** -- notifies of
  changes to the life or suspended status of any relation the unit's
  application is part of (a subordinate watches through its principal
  application). Notifications carry relation keys.
- **An application's relations' life and suspended state** -- the same
  surface per application; notifications carry relation UUIDs.
- **A unit's counterparts in one relation** -- notifies of changes to
  the other units in the relation. This watcher watches three
  change-log namespaces at once: the relation's unit rows, the
  unit-side settings and the application-side settings, which is what
  turns a databag write on one side into a `relation-changed` on the
  other.
- **The units in a given relation** -- notifies of changes to the
  units in the relation in the local model.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change:
the watcher is woken by the change log, the agent fetches the current
state and reconciles. When a relation is removed, key-based consumers
receive one final change carrying the relation key, so they can clean
up the state they hold under that key.

## Rules and errors

The relation domain encodes its rules as a typed error taxonomy; each
error names the rule it enforces.

The rules a **new relation** must satisfy:

- both applications must be alive;
- the endpoints must be compatible: same interface, opposite
  `provides` / `requires` roles;
- if one endpoint is container-scoped, so must the other be, and then
  one of the applications must be a subordinate;
- the per-endpoint relation quota (`MaxRelationLimit`) must not be
  exceeded.

The errors that encode them:

- *Endpoint matching*: `no compatible endpoints found between
  applications` (nothing matched), `ambiguous relation` (several
  endpoint pairs matched -- name the endpoints explicitly),
  `already exists` (the same relation is already in place).
- *Quota*: `quota limit exceeded`.
- *Entering scope*: `cannot enter scope, unit or relation not alive`,
  `cannot enter scope, subordinate unit exists but is not alive`,
  `relation unit already exists` (entering scope is idempotent --
  entering twice is this error, not a second membership).
- *Lifecycle guards*: `relation is not alive`, `unit is dead`, and the
  existence errors (`relation not found`, `relation unit not found`,
  `unit not in relation`, `application endpoint not found`).

## Related domains

- **Status** owns the relation status vocabulary and validates every
  transition (see {ref}`Relation status <relation-status>`).
- **Removal** owns the scheduled teardown that takes a dying relation
  to dead (see {ref}`Relation removal <relation-removal>`).
- **Cross-model relations** are the cross-model half of the story:
  the offerer's and consumer's models each hold their own records,
  and the suspension state propagates across the two (see
  {ref}`cross-model relation <cross-model-relation>`).
