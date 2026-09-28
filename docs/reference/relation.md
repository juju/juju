---
myst:
  html_meta:
    description: "Juju relation (integration) reference: the connection between applications through endpoints. The relation record, relation settings, relation status, the four relation kinds, relation operations, and relation watchers."
---

(relation)=
# Relation (integration)

In Juju, a **relation** (or **integration**) is the connection between two
{ref}`applications <application>` through a pair of
{ref}`endpoints <application-endpoint>`, or of an application to itself
through a `peers` endpoint. It is one object across the three layers: an
integrate operation over endpoints declares it, the model database persists
its record with its settings and status, and the related units' agents
execute it, moving the settings across with their relation hooks.

## Relations in the declaration layer

- **Integration:** A relation is created through the integrate operation,
  which names two endpoint identifiers of the form
  `<application>[:<endpoint>]` over the controller API and infers the
  matching endpoint pair. The operation requires model {ref}`write access
  <user-access-model-write>`.
- **Peer relations:** Never created by a client. A `peers` endpoint
  relates the application to itself, and the relation is created
  automatically when the application is deployed.
- **Suspend, resume, remove:** The other relation operations, all over the
  controller API with model write access.

```{ibnote}
See also: {ref}`Juju | Manage relations <manage-relations>`, {ref}`Terraform Provider for Juju | Manage relations <tfjuju:manage-relations>`
```

(the-relations-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - Both applications must be alive.
  - The endpoints must infer to exactly one compatible pair: same
    interface, counterpart `provides` / `requires` roles, two distinct
    applications.
  - If either endpoint is container-scoped, one of the two applications
    must be subordinate, and the two applications' bases must be
    compatible.
  - Both sides cannot be remote, cross-model applications.
  - The endpoint's relation limit, the charm's per-endpoint quota, must
    not be exceeded.
  - No relation may already connect the same pair of endpoints.
- **Errors:**
  - **`no compatible endpoints found between applications`:** Triggered
    when no endpoint pair matches the two applications. Remediation:
    check the two charms' declared interfaces and roles.
  - **`ambiguous relation`:** Triggered when several endpoint pairs match.
    Remediation: name the endpoints explicitly in the identifiers.
  - **`already exists`:** Triggered when the same pair of endpoints is
    already related. Remediation: none; the relation is in place.
  - **`quota limit exceeded`:** Triggered when the endpoint's relation
    limit is reached. Remediation: remove one of the endpoint's relations
    or raise the charm's endpoint limit.
  - **`both endpoints are remote applications`:** Triggered when both
    sides are cross-model remote applications. Remediation: relate through
    a local application.
  - **`relation endpoint not found`:** Triggered when an identifier names
    an application with no candidate endpoint. Remediation: check the
    application's endpoint names.

## Relations in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Relation attributes
:alt: The relation records as an entity-relationship slice: relation at the centre pointing to life and charm_relation_scope; relation_endpoint below it pointing back to relation and across to application_endpoint; relation_unit pointing to relation_endpoint and unit; the unit and application settings records (with their sha256 hash columns) hanging under their owners; relation_status pointing to relation and relation_status_type; the settings archive pointing to relation. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory.
:caption: Entity relationship diagram: The relation's ten stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the record may be absent). The services read these records through four derived views, which have no pointers of their own and are therefore not drawn.
```

In the model database, a relation is a **native record** (`0024-relation.sql`):

- **`relation`:** The core record: the UUID, the join handle; the
  **relation ID**, a monotonically increasing number drawn from the
  model's sequence, never reused in the model's lifetime, the number the
  client sees; the life pointer; the scope; and the suspended flag with
  its reason.
- **`relation_endpoint`:** The link records to the endpoints: two for a
  provider and requirer, one for a peer relation.
- **`relation_unit`:** One record per unit that has entered the relation's
  scope, per endpoint.
- **`relation_unit_setting`:** The unit settings: key/value pairs keyed
  per relation unit; keys must be non-empty.
- **`relation_unit_settings_hash`:** A SHA-256 digest of each unit's
  settings, so watchers detect a settings change without reading the
  values.
- **`relation_application_setting`:** The application settings: key/value
  pairs keyed per relation endpoint, so an application in several
  relations keeps an independent set per relation.
- **`relation_application_settings_hash`:** The digest of each endpoint's
  application settings, the change signal watchers see.
- **`relation_unit_setting_archive`:** A unit's settings, copied here when
  it leaves scope. They stay readable for the lifetime of the relation,
  even after the unit itself is gone.
- **`relation_status` and `relation_status_type`:** The status record and
  its vocabulary: `joining`, `joined`, `broken`, `suspending`,
  `suspended`, `error`.

The identity pair: the UUID is the join handle; the relation ID is the
user-facing number; the **relation key** is the endpoint-derived string
(`application:endpoint application:endpoint`, requirer first; a peer
relation's key is its single endpoint). The endpoint pair is the
relation's natural key: no two relations in a model may connect the same
pair, and the relation service checks the pair before it writes the
record.

Writers: the relation service inserts the relation with its endpoint
records
and its initial settings; the removal service carries the teardown; the
status domain validates and writes the status. Every settings write
updates the matching hash record. The services do not read the stored
records
directly: they read the four derived views (`v_application_endpoint`,
`v_relation_endpoint`, `v_relation_endpoint_identifier`,
`v_relation_status`) that join the records into the shapes the domain
speaks
in.

Two projections with writers of their own:

- **Life:** The shared alive / dying / dead cycle. A relation is created
  alive. It cannot stay alive if either of its applications stops being
  alive, and it cannot be dead until every relation unit has left scope.
- **`relation_status`:** The status domain's record, written through
  `SetRelationStatus`: a new relation starts as `joining`, the status
  record written when the relation record is created; the leader unit's agent
  reports `joined`; `suspending` and `suspended` record a suspended
  relation; `error` requires a status message.

### Relation settings

Creating a relation creates its settings: one unit settings record per
involved unit and one application settings record per involved
application. Each unit involved in the relation gets a local copy of all
the settings for that relation. Charm documentation often calls these
*relation databags*; Juju's code and hook commands all say
*settings*.

```{ggarch}
:file: ../juju.ggarch
:view: Relation settings permissions
:no-legend:
:caption: Topology: Each unit reads + writes only its own settings (red); the leader also writes the application settings; all units read the other application's settings (green). Peer case: permissions turn inward -- every unit reads all of its own application's settings, application settings included.
:alt: App A's units (appA/leader, appA/1) and app B's units (appB/leader, appB/1) above one set of settings records; red arrows reading and writing the own records (own unit settings; the leader also the application settings), green arrows reading across to the other application's set; below, the peer panel: one application's units with red own/leader arrows and green reads of every record, the application settings included.
```

While the relation is maintained:

- In a relation between two applications, regular or subordinate:
  - Each unit reads and writes its own unit settings.
  - The leader unit also reads and writes the local application
    settings.
  - All of an application's units read the remote application's
    settings.
- In a peer relation the permissions turn inward:
  - Each unit reads and writes its own unit settings.
  - The leader unit also reads and writes the application settings.
  - Every unit reads all of its own application's settings: the other
    units' unit settings and the application settings included.

The application settings are gated on leadership at both ends: only the
leader unit may read or write them, and the hook commit that carries
application settings is wrapped in a leadership lease, so the unit
committing them must hold leadership. A read of a departed unit's
settings is served from the archive.

### Types of relation

```{ggarch}
:file: ../juju.ggarch
:view: Types of relation
:alt: The relation kinds as a star: Relation at the top, the four kinds in a row below, each connected to Relation by a straight is-a edge ending in a hollow triangle. The edges are labelled with the discriminating fact: the application relates to itself (Peer relation); one side subordinate (Subordinate relation); both principal, same model (Regular relation); different models (Cross-model relation).
:caption: Taxonomy star: A relation is a peer relation when the application relates to itself; otherwise it connects two applications, and it is a subordinate relation when one side is subordinate (always same-model), a cross-model relation when the applications live in different models, and a regular relation when two principal applications share a model.
```

Every relation is one of four kinds. The kind is not a stored
discriminator: it follows from the relation's own records, which
applications it connects, the same application or two distinct ones, and,
for two distinct applications, whether one of them is subordinate and
whether the two live in the same model.

(peer-relation)=
#### Peer relation

```{ggarch}
:file: ../juju.ggarch
:view: Peer relation shape
:alt: The application relating to itself: the peer relation has one endpoint, and the application's units fan into it -- every unit joins the same relation.
:caption: Topology: The peer relation's shape: the application relates to itself -- the relation has one endpoint, created automatically at deployment, and every unit the application ever has joins it as it starts.
```

A **peer** relation relates an application to itself: its units relate to
one another by virtue of the application having a `peers` endpoint.

- Created automatically at deployment time; never added by a client.
- Every unit the application ever has joins the one relation as it
  starts; scaling the application never duplicates it.
- The relation gives the units a shared store of application settings and
  a notification channel: each settings write wakes the other units'
  watchers. This is the mechanism behind {ref}`scaling <scaling>` and
  {ref}`high availability <high-availability>`.

(subordinate-relation)=
#### Subordinate relation

```{ggarch}
:file: ../juju.ggarch
:view: Subordinate relation shape
:alt: A principal application and a subordinate application relating through a container-scoped relation; below, both the principal unit and the subordinate unit run on the principal unit's machine.
:caption: Topology: The subordinate relation's shape: a principal application and a subordinate charm relate through a container-scoped relation; the relation is what places the subordinate's unit on the principal unit's own machine.
```

A **subordinate** relation is a relation between a principal application
and a {ref}`subordinate <subordinate-charm>` application.

- Always a same-model relation: the relation is container-scoped, and the
  container scope is what places the subordinate's unit on the principal
  unit's own machine.
- A subordinate deploys as an application with no unit. The relation gives
  it its units, one per principal unit, so it scales automatically with
  the principal.
- When a unit enters a container-scoped relation, the subordinate unit is
  created and placed on the principal unit's machine.

(the-implicit-juju-info-relation-endpoint)=
##### The implicit `juju-info` endpoint

Every application in a model implicitly provides an extra endpoint named
`juju-info`, with role `provides`, interface `juju-info`, and global
scope. The endpoint is supplied by Juju itself: if the charm author has
not declared a relation named `juju-info`, Juju adds this one, and every
charm implements it.

- It exists to let {ref}`subordinate charms <subordinate-charm>` attach
  to a principal that does not otherwise expose a suitable interface, so
  it matters on {ref}`machine charms <machine-charm>`: subordinate
  relations are machine-model relations.
- The subordinate side declares a `requires` endpoint with interface
  `juju-info`, typically named `juju-info`, any name works, and declared
  with container scope, which is what co-locates the subordinate's unit
  with the principal's.
- It is the standard mechanism for general-purpose subordinates that ride
  along on every machine of a principal without exchanging
  application-specific data, such as monitoring or logging agents.
- It is integrated against like any other endpoint. Naming endpoints in
  the identifiers disambiguates when several pairs match.

(regular-relation)=
#### Regular relation

```{ggarch}
:file: ../juju.ggarch
:view: Regular relation shape
:alt: Two principal applications -- one provides, one requires -- relating through a two-endpoint relation in the same model.
:caption: Topology: The regular relation's shape: two principal applications in the same model relate through a two-endpoint relation -- opposite provides/requires roles on the same interface.
```

A **regular** relation connects two principal applications that live in
the same model. Both sides support the same endpoint interface and have
opposite `provides` / `requires` endpoint roles.

(cross-model-relation)=
#### Cross-model relation

```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation shape
:alt: Two model containers -- consuming model A with its application, offering model B with the saas synthetic application -- joined by a cross-model relation edge.
:caption: Topology: The cross-model relation's shape: the two applications live in different models, each holding its own half; the consuming model integrates through a synthetic application (the saas) that stands in for the offered application.
```

A **cross-model** relation relates applications that live in different
models, on different controllers or clouds. Each model holds its own
half: the consuming model connects through a synthetic application that
stands in for the offered application in the offering model.

- The synthetic application's name is the saas name locally; the
  offering model sees the remote side as `remote-` plus a UUID, so local
  application names never leak across.
- The offering side records the consuming side's egress CIDRs as the
  relation's allowed ingress.
- Suspension propagates to the remote half, so both models agree on the
  suspended state.
- For the controllers to communicate, the addresses they advertise, the
  bootstrap configuration keys `controller-external-ips` and
  `controller-external-name`, must be resolvable from the other
  controllers.

(the-relations-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - The status vocabulary is validated on every write: every status may
    transition to `broken`; `joining` cannot follow `joined` or `broken`;
    `suspending` cannot follow `broken` or `suspended`; `joined` and
    `suspended` cannot follow `broken`; `error` requires a message; and a
    `suspended` write with an empty message retains the reason recorded
    when the relation was suspending.
  - Settings keys must be non-empty, and empty values are dropped rather
    than written.
- **Errors:**
  - **`relation not found`:** Triggered when the named relation does not
    exist. Remediation: check the relation ID or key.
  - **`relation is not alive`:** Triggered when operating on a relation
    whose life is not alive. Remediation: none; the relation is being
    torn down.
  - **`relation unit not found`:** Triggered when the named relation unit
    does not exist. Remediation: verify the unit has entered the
    relation's scope.
  - **`unit is dead`:** Triggered when operating on a unit whose life is
    dead. Remediation: none; the unit's lifecycle has ended.
  - **`unit not in relation`:** Triggered when the named unit is not a
    member of the relation. Remediation: check the relation's units in
    scope.
  - **`application endpoint not found`:** Triggered when the named
    application endpoint does not exist. Remediation: check the
    application's endpoint names.

## Relations in the execution layer

A relation has no machinery of its own: its units' agents execute it.
Each side's units run their relation hooks in lockstep; the model side is
record bookkeeping: creating, suspending, resuming, removing.

(the-relations-operations)=
### Relation operations

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:alt: User calls juju integrate A B. Controller writes relation record and fires watchers to both unit agents. Each runs relation-created, relation-joined, relation-changed hooks, writing relation data; each data write wakes the other side's watcher for a further relation-changed.
:caption: Sequence diagram: Integrating two applications. The controller writes the relation record; the two units' hooks run in lockstep, each data write waking the other side for another relation-changed.
```

- **Creation:** The integrate operation writes the relation record and
  wakes both sides: each unit's watcher fires, and the units run their
  relation hooks in lockstep, `relation-created`, then `relation-joined`
  and `relation-changed`, exchanging settings as they go. Each settings
  write wakes the other side for a further `relation-changed`.
- **Removal:** The remove-relation operation follows the cooperative
  pattern: the relation is marked dying, the units in scope leave as
  their agents notice, and a scheduled removal job deletes the records
  once nothing is left in scope. A forced removal tears the relation and
  its scope entries down without waiting for the agents. Cross-model
  relations are rejected by the removal pre-check: the local record is
  only the local half of the relation.
- **Suspending and resuming:** Suspension pauses the data flow across a
  cross-model relation. The record keeps the suspended flag and its
  reason, the status moves `suspending`, then `suspended`, and the change
  propagates to the remote half. Resuming restores the flow and the
  status returns to `joining`.

### Relation hooks and hook commands

The relation's runtime surface is the {ref}`hook machinery
<hook-execution>`: the {ref}`relation hooks <relation-hooks>` fire on the
unit agents, and the relation hook commands read and write the settings.
The command pages carry the flags and the environment (see
{ref}`list of hook commands <list-of-hook-commands>`.

- **Hooks:** `<endpoint>-relation-created` runs once when the relation is
  first created; `<endpoint>-relation-joined` once when a related unit is
  first seen; `<endpoint>-relation-changed` on creation and whenever a
  related unit's settings change; `<endpoint>-relation-departed` when a
  related unit leaves; `<endpoint>-relation-broken` when the relation is
  torn down.
- **Commands:** `relation-get` reads a unit's or an application's
  settings, serving a departed unit's settings from the archive;
  `relation-set` writes the unit's own unit settings, and the application
  settings under leadership; `relation-ids` lists the relation IDs for an
  endpoint; `relation-list` lists the units in scope.

(the-relations-execution-rules)=
### Execution rules and errors

- **Rules:**
  - Entering scope is idempotent: a second enter is a no-op and the
    settings are not rewritten; even a dying unit or relation may be
    re-entered if the unit is already in scope, so the teardown hook flow
    can proceed. The state layer's `relation unit already exists` guard
    is folded into this no-op.
  - Relation status is written only by the leader unit: the write is
    leadership-gated, and the leader unit's agent reports the relation as
    `joined`.
  - A unit entering a container-scoped relation gets its subordinate
    unit created, placed on the principal unit's machine.
- **Errors:**
  - **`cannot enter scope, unit or relation not alive`:** Triggered when
    entering scope with a dead or dying unit or relation. Remediation:
    none; the relation or unit is being torn down.
  - **`cannot enter scope, subordinate unit exists but is not alive`:**
    Triggered when a container-scoped relation's subordinate unit exists
    but is not alive. Remediation: remove the stale subordinate unit.
  - **`entering scope for peer relation` and `entering scope, unit
    application is a subordinate`:** The remote-relation guards. Remote
    units do not enter local peer relations, and subordinate
    applications do not enter remote scope.

(the-relations-watchers)=
### Relation watchers

Nothing about a relation is polled: agents and clients watch it. The
relation domain's watchable service exposes five surfaces:

- **One relation's life and suspended state:** Changes to the given
  relation.
- **A unit's relations' life and suspended state:** Any relation the
  unit's application is part of; a subordinate watches through its
  principal application. Notifications carry relation keys.
- **An application's relations' life and suspended state:** The same
  surface per application; notifications carry relation UUIDs.
- **A unit's counterparts in one relation:** The other units in the
  relation. The watcher watches three change-log namespaces at once: the
  relation's unit records, the unit-side settings hashes, and the
  application-side settings hashes, which is what turns a settings write
  on one side into a `relation-changed` on the other.
- **The units in a given relation:** Membership changes in the local
  model.

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`). When a relation is removed,
key-based consumers receive one final change carrying the relation key,
so they can clean up the state they hold under that key.
