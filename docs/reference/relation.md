---
myst:
  html_meta:
    description: "Juju relation (integration) reference: application connections through endpoints. These include regular relations, peer relations, subordinate relations, and cross-model relations, and involve relation settings."
---

(relation)=
# Relation (integration)

```{ibnote}
See also: {ref}`manage-relations`
```

In Juju, a **relation** (**integration**) is a connection an {ref}`application <application>` supports by virtue of having a particular {ref}`endpoint <application-endpoint>`.

(the-relations-records)=
## The relation's records

### The relation's identity

In the model database, a relation is a record identified by a
**relation ID** (assigned automatically by Juju; expressed in
monotonically increasing numbers) or a **relation key** (derived from
the endpoints, format: `application1:[endpoint] application2:[endpoint]`).

### The relation in the data model

A relation's state is spread across ten stored tables in the model
database. The `relation` record carries the relation ID, its life,
its scope, and the suspended flag with its reason. The
`relation_endpoint` table links the relation to the application
endpoints that realize it (two rows for a provider/requirer
relation, one for a peer relation); `relation_unit` records the units
that have entered scope, per endpoint.

The relation's payload lives in four settings tables: unit-level
settings in `relation_unit_setting` and application-level settings in
`relation_application_setting`, each with a companion `sha256` hash
table that lets watchers detect a settings change without reading the
values. Both are scoped to the relation, not to the application
alone: the application-level table is keyed per relation endpoint, so
an application that participates in several relations keeps an
independent set of settings for each. When a unit leaves scope, its
settings are copied to `relation_unit_setting_archive`, where they
stay readable for the lifetime of the relation -- even after the unit
itself is gone; a read of a departed unit's settings is served from
the archive (`relation-get`, for example).

These settings are often called *relation databags* in the charm
community -- a name that stuck from the early charm-tooling days.
Juju's own code never uses it: the tables, the service methods, the
API facades and the hook context all say *settings* (`relation_unit_setting`,
`Settings()`, `ApplicationSettings()`), so this document does too.
The word is kept here as an alias so that readers arriving from charm
documentation find the same concept.

The relation's status is its own table pair: `relation_status` holds
the current status, message and update time; `relation_status_type`
is the status vocabulary (see {ref}`Relation status
<relation-status>`).

The services do not read these tables directly: they read them
through four derived views (`v_application_endpoint`,
`v_relation_endpoint`, `v_relation_endpoint_identifier`,
`v_relation_status`) that join the stored rows into the shapes the
domain speaks in. The views have no pointers of their own, so they do
not appear in the diagram.

```{ggarch}
:file: ../juju.ggarch
:view: Relation attributes
:alt: The relation tables as an entity-relationship slice: relation at the centre pointing to life and charm_relation_scope; relation_endpoint below it pointing back to relation and across to application_endpoint; relation_unit pointing to relation_endpoint and unit; the unit and application settings tables (with their sha256 hash columns) hanging under their owners; relation_status pointing to relation and relation_status_type; the settings archive pointing to relation. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The relation's ten stored tables and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The services read these tables through four derived views, which have no pointers of their own and are therefore not drawn.
```

(relation-settings)=
#### Relation settings

Creating a relation creates its settings: one unit settings record
per involved unit and one application settings record per involved
application. The settings can be unit-scoped or application-scoped,
and each unit involved in the relation gets a local copy of all the
settings for that relation.

##### Permissions around relation settings

```{ggarch}
:file: ../juju.ggarch
:view: Relation settings permissions
:no-legend:
:caption: Topology: Each unit reads + writes only its own settings (red); the leader also writes the application settings; all units read the other application's settings (green). Peer case: permissions turn inward -- every unit reads all of its own application's settings, application settings included.
:alt: App A's units (appA/leader, appA/1) and app B's units (appB/leader, appB/1) above one row of settings records; red arrows reading and writing the own records (own unit settings; the leader also the application settings), green arrows reading across to the other application's set; below, the peer panel: one application's units with red own/leader arrows and green reads of every record, the application settings included.
```

While the relation is maintained,

- in a relation between two applications, whether regular or subordinate:
    - each unit can read and write to its own unit settings;
    - leader units can also read and write to the local application
      settings;
    - all units of an application can read all of the remote
      application's settings.
- in a peer relation:
    - each unit can read and write to its own unit settings;
    - leader units can also read and write to the application
      settings;
    - all units can read all of the application's settings. That is,
      whether leader or not, every unit can read its own unit
      settings as well as every other unit's unit settings as well as
      the application settings.

Note that, in peer relations, all permissions related to the remote
application are turned inwards and become permissions related to the
local application.

The application settings are gated on leadership: only the leader
unit may read or write them, and a non-leader that tries gets a
permission error. Writes happen through the unit agent's hook
machinery (`relation-set --app` commits application settings, for
example), and the commit that carries application settings is wrapped
in a leadership lease -- the unit committing them must hold
leadership.

### Relation states

A relation carries two orthogonal state machines: its **life** -- the
shared alive / dying / dead cycle every entity has -- and its
**relation status**, the relation-specific status the status domain
validates.

#### Life

A relation is created alive. It cannot stay alive if either of its
applications stops being alive, and it cannot be declared dead until
every relation unit has left scope. When the relation is removed, the
removal machinery takes over: the relation is marked dying, the units
in scope leave as their agents notice, and a scheduled removal job
finishes the job once nothing is left in scope.

(relation-status)=
#### Relation status

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

### Types of relation

```{ggarch}
:file: ../juju.ggarch
:view: Types of relation
:alt: The relation kinds as a star: Relation at the top, the four kinds in a row below, each connected to Relation by a straight is-a edge ending in a hollow triangle. The edges are labelled with the discriminating fact: the application relates to itself (Peer relation); one side subordinate (Subordinate relation); both principal, same model (Regular relation); different models (Cross-model relation).
:caption: Taxonomy star: A relation is a peer relation when the application relates to itself; otherwise it connects two applications, and it is a subordinate relation when one side is subordinate (always same-model), a cross-model relation when the applications live in different models, and a regular relation when two principal applications share a model.
```

Every relation is one of four kinds. The kind follows from two facts:
which applications the relation connects -- the same application, or
two distinct ones -- and, for two distinct applications, whether one
of them is subordinate and whether the two live in the same model.

(peer-relation)=
#### Peer relation

```{ggarch}
:file: ../juju.ggarch
:view: Peer relation shape
:alt: The application relating to itself: the peer relation has one endpoint, and the application's units fan into it -- every unit joins the same relation.
:caption: Topology: The peer relation's shape: the application relates to itself -- the relation has one endpoint, created automatically at deployment, and every unit the application ever has joins it as it starts.
```

A **peer** relation is a relation of an application to itself: its
units relate to one another by virtue of the application having a
`peers` endpoint. Juju creates the peer relation automatically at
deployment time -- it is never added by a user -- and every unit the
application ever has joins it as it starts.

This is what makes the peer relation the mechanism behind
{ref}`scaling <scaling>` and
{ref}`high availability <high-availability>`: scaling the application
never duplicates the relation. Every new unit enters the scope of the
one existing relation, and the relation gives the units both a shared
store of application settings and a notification channel (each
settings write wakes the other units). The units of the application
therefore keep coordinating with one another however many of them
there are -- which is what clustering an application, and thus
high availability, require.

Because every relation results in the creation of unit and
application settings in Juju's database, peer relations are also
sometimes used by charm authors as a way to persist charm data.

(subordinate-relation)=
#### Subordinate relation

```{ggarch}
:file: ../juju.ggarch
:view: Subordinate relation shape
:alt: A principal application and a subordinate application relating through a container-scoped relation; below, both the principal unit and the subordinate unit run on the principal unit's machine.
:caption: Topology: The subordinate relation's shape: a principal application and a subordinate charm relate through a container-scoped relation; the relation is what places the subordinate's unit on the principal unit's own machine.
```

A **subordinate** relation is a relation between a principal
application and a {ref}`subordinate <subordinate-charm>`
application. It is always a same-model relation: the relation is
container-scoped, and the container scope is what places the
subordinate's unit on the principal unit's own machine.

A subordinate charm is by definition a charm deployed on the same
machine as the principal charm it is intended to accompany. When you
deploy a subordinate charm, it appears in your Juju model as an
application with no unit. The subordinate relation helps the
subordinate application acquire a unit. The subordinate application
then scales automatically when the principal application does, by
virtue of this relation.

(the-implicit-juju-info-relation-endpoint)=
##### The implicit `juju-info` relation endpoint

Every application in a model implicitly provides an extra endpoint named `juju-info`, with role `provides`, interface `juju-info`, and global scope. The endpoint is supplied by Juju itself: it does not need to be (and cannot be) declared in the charm's `metadata.yaml` / `charmcraft.yaml`, and it does not appear in `juju info <charm>` or on the charm's Charmhub page. It is supported on every kind of charm, but is only useful for {ref}`machine charms <machine-charm>`, since it exists to allow {ref}`subordinate charms <subordinate-charm>` to attach to a principal that does not otherwise expose a suitable interface.

The `juju-info` endpoint is intended to be consumed by a {ref}`subordinate charm <subordinate-charm>` whose `metadata.yaml` / `charmcraft.yaml` declares an explicit `requires` endpoint with interface `juju-info` (typically with name `juju-info` and `scope: container`). The container scope is what causes the subordinate's unit to be co-located on the same machine as the principal's unit.

The `juju-info` endpoint is the standard mechanism used by general-purpose machine subordinates that need to ride along on every machine of a principal but do not have any application-specific data to exchange. For example, monitoring, logging, or system-administration agents such as [`ntp`](https://charmhub.io/ntp), [`telegraf`](https://charmhub.io/telegraf), [`nrpe`](https://charmhub.io/nrpe), and [`canonical-livepatch`](https://charmhub.io/canonical-livepatch).

The convention is for a subordinate to name its own `requires` endpoint `juju-info`, but any name can be used (for example: `info`, `general-info`); it just needs to use interface `juju-info`.

The implicit endpoint is integrated against the same way as any other endpoint, for example:

```text
juju integrate <subordinate> <principal>
```

If the subordinate also has explicit endpoints whose interfaces match endpoints on the principal, those explicit endpoints take precedence over the implicit `juju-info` one when Juju resolves the relation. To force the implicit endpoint, name it explicitly on either side:

```text
juju integrate <subordinate>:<requires-endpoint> <principal>:juju-info
```

(regular-relation)=
#### Regular relation

```{ggarch}
:file: ../juju.ggarch
:view: Regular relation shape
:alt: Two principal applications -- one provides, one requires -- relating through a two-endpoint relation in the same model.
:caption: Topology: The regular relation's shape: two principal applications in the same model relate through a two-endpoint relation -- opposite provides/requires roles on the same interface.
```

A **regular** relation is a relation between two principal
applications that live in the same model. Both sides of the relation
support the same endpoint interface and have opposite `provides` /
`requires` endpoint roles.

(cross-model-relation)=
#### Cross-model relation

```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation shape
:alt: Two model containers -- consuming model A with its application, offering model B with the saas synthetic application -- joined by a cross-model relation edge.
:caption: Topology: The cross-model relation's shape: the two applications live in different models, each holding its own half; the consuming model integrates through a synthetic application (the saas) that stands in for the offered application.
```

```{ibnote}
See also: {ref}`manage-relations`
```

A **cross-model** relation (aka 'CMR') is a relation between
applications that live in different models (+/- different
controllers, +/- different clouds). Each model holds its own half of
the relation: the consuming model connects through a synthetic
application that stands in for the offered application in the
offering model.

Cross-model relations enable, for example, scenarios where  your databases are hosted on bare metal, to take advantage of I/O performance, and your applications live within Kubernetes, to take advantage of scalability and application density.

If the network topology is anything other than flat, the Juju controllers will need to be bootstrapped with `--controller-external-ips`, `--controller-external-name`, or both, so that the controllers are able to communicate. Note that these config values can only be set at bootstrap time, and are read-only thereafter.

A cross-model relation has two sides: the offering side (aka "offerer") and the consume side (aka 'saas'). It does not make a difference which side of the relation (provider or requirer) is the offerer and which is the saas - the two are interchangeable. However, the endpoint type does influence on how juju sets up firewall rules: it is assumed that a requirer is the client and the provider is the server, so ports are opened on the provider side.

Note that application names are obfuscated (anonymised) to the offerer side:
- Applications that relate to the saas appear to the offerer as remote + token, e.g. `remote-76cd96ab50f146b284912afd1cc13a0e`.
- For the consumer, the remote app names is the saas name, e.g. `prometheus`.

(the-relations-machinery)=
## The relation's machinery

A relation has no machinery of its own -- its units' agents execute it:
each side's units run their relation hooks in lockstep, and the model
side is record bookkeeping (creating, suspending, resuming, removing).

### Relation operations

#### Relation creation

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:alt: User calls juju integrate A B. Controller writes relation record and fires watchers to both unit agents. Each runs relation-created, relation-joined, relation-changed hooks, writing relation data; each data write wakes the other side's watcher for a further relation-changed.
:caption: Sequence diagram: Integrating two applications. The controller writes the relation record; the two units' hooks run in lockstep, each data write waking the other side for another relation-changed.
```


A relation is created by the integrate operation -- for example,
`juju integrate A B`. Creating the relation writes a relation record
and wakes both sides: each unit's watcher fires, and the units run
their relation hooks in lockstep -- `relation-created`, then
`relation-joined` and `relation-changed` -- exchanging settings as
they go.

(relation-removal)=
#### Relation removal

Removal is initiated through the remove-relation operation (for
example, `juju remove-relation 0`). Removal follows the same
cooperative pattern as every entity removal: the relation is marked
dying, the units in scope leave as their agents notice, and the
removal machinery schedules a removal job that finishes once nothing
is left in scope. A forced removal (`--force`) skips the wait: the
relation and its units' scope entries are torn down without waiting
for the agents to leave gracefully.

Cross-model relations are not removed by this path: the removal
pre-check rejects them, because the local record is only the local
half of the relation.

(suspending-relations)=
#### Suspending and resuming

Suspending pauses the data flow across a relation to an application
offer; the resume operation restores it (the `juju suspend-relation`
and `juju resume-relation` commands, for example). The relation
records the suspended state and its reason, and the controller's
firewaller closes the provider side's ingress for the suspended
relation. On a cross-model relation the suspension is propagated to
the remote model as well, so both halves agree on the suspended
state.

### Relation watchers

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
  turns a settings write on one side into a `relation-changed` on the
  other.
- **The units in a given relation** -- notifies of changes to the
  units in the relation in the local model.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change:
the watcher is woken by the change log, the agent fetches the current
state and reconciles. When a relation is removed, key-based consumers
receive one final change carrying the relation key, so they can clean
up the state they hold under that key.

## Relation rules and errors

The relation domain encodes its rules as a typed error taxonomy; each
error names the rule it enforces. The rules matter to charm
developers -- they are the errors a relation hook can meet -- and to
Juju developers, who maintain them as the domain's validation law.

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

## Entities related to the relation

- **Status** owns the relation status vocabulary and validates every
  transition (see {ref}`Relation status <relation-status>`).
- **Removal** owns the scheduled teardown that takes a dying relation
  to dead (see {ref}`Relation removal <relation-removal>`).
- **Cross-model relations** are the cross-model half of the story:
  the offerer's and consumer's models each hold their own records,
  and the suspension state propagates across the two (see
  {ref}`cross-model relation <cross-model-relation>`).
