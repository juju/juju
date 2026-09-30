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

(the-relations-declaration-rules)=
## Relations in the declaration layer

- **Integration:** A relation is created through the integrate operation, which names two endpoint identifiers of the form `<application>[:<endpoint>]` over the controller API and infers the matching endpoint pair. The operation requires model {ref}`write access <user-access-model-write>`.
  - *Rule:* Both applications must be alive.
  - *Rule:* The endpoints must infer to exactly one compatible pair: same interface, counterpart `provides` / `requires` roles, two distinct applications.
  - *Rule:* If either endpoint is container-scoped, one of the two applications must be subordinate, and the two applications' bases must be compatible.
  - *Rule:* Both sides cannot be remote, cross-model applications.
  - *Rule:* The endpoint's relation limit, the charm's per-endpoint quota, must not be exceeded.
  - *Rule:* No relation may already connect the same pair of endpoints.
  - *Related errors:*
    - **`no compatible endpoints found between applications`**: *Trigger:* No endpoint pair matches the two applications. *Remediation:* Check the two charms' declared interfaces and roles.
    - **`ambiguous relation`**: *Trigger:* Several endpoint pairs match. *Remediation:* Name the endpoints explicitly in the identifiers.
    - **`already exists`**: *Trigger:* The same pair of endpoints is already related. *Remediation:* None; the relation is in place.
    - **`quota limit exceeded`**: *Trigger:* The endpoint's relation limit is reached. *Remediation:* Remove one of the endpoint's relations or raise the charm's endpoint limit.
    - **`both endpoints are remote applications`**: *Trigger:* Both sides are cross-model remote applications. *Remediation:* Relate through a local application.
    - **`relation endpoint not found`**: *Trigger:* An identifier names an application with no candidate endpoint. *Remediation:* Check the application's endpoint names.
- **Peer relations:** Never created by a client. A `peers` endpoint relates the application to itself, and the relation is created automatically when the application is deployed.
- **Suspend, resume, remove:** The other relation operations, all over the controller API with model write access.

```{ibnote}
See also: {ref}`Juju | Manage relations <manage-relations>`, {ref}`Terraform Provider for Juju | Manage relations <tfjuju:manage-relations>`
```

(the-relations-persistence-rules)=
## Relations in the persistence layer

In the {ref}`model database <database>`, a relation is a **native record**,
distributed across a decoupled set of records. The record set, in
prose:

- **Primary entry:** A single record containing the relation's essential identifiers: the UUID, the join handle the other records point at, and the **relation ID**, a monotonically increasing number drawn from the model's sequence, never reused in the model's lifetime, the number the client sees. The life and the scope are stored vocabularies (the shared alive / dying / dead cycle; global, or container when either endpoint is); the suspended flag with its reason rides the record. The **relation key** is the endpoint-derived string (`application:endpoint application:endpoint`, requirer first; a peer relation's key is its single endpoint). The endpoint pair is the relation's natural key: no two relations in a model may connect the same pair, and the relation service checks the pair before it writes the record.
  - *Rule:* The relation is created alive. It cannot stay alive if either of its applications stops being alive, and it cannot be dead until every relation unit has left scope.
  - *Related errors:*
    - **`relation not found`**: *Trigger:* The named relation does not exist. *Remediation:* Check the relation ID or key.
    - **`relation is not alive`**: *Trigger:* Operating on a relation whose life is not alive. *Remediation:* None; the relation is being torn down.
- **Endpoint links:** Two link records for a provider and a requirer, one for a peer relation; each names the application's endpoint (the charm endpoint with its optional space binding).
  - *Related error:*
    - **`application endpoint not found`**: *Trigger:* The named application endpoint does not exist. *Remediation:* Check the application's endpoint names.
- **Relation units:** A membership record, one per unit that has entered the relation's scope, per endpoint. The unit itself is the neighbour the pointer reaches.
  - *Related errors:*
    - **`relation unit not found`**: *Trigger:* The named relation unit does not exist. *Remediation:* Verify the unit has entered the relation's scope.
    - **`unit not in relation`**: *Trigger:* The named unit is not a member of the relation. *Remediation:* Check the relation's units in scope.
    - **`unit is dead`**: *Trigger:* Operating on a unit whose life is dead. *Remediation:* None; the unit's lifecycle has ended.
- **Settings records:** Two levels. One unit settings record per involved unit (a key/value record per key, keys non-empty), and one application settings record per involved application, keyed per relation endpoint, so an application in several relations keeps an independent set per relation. Each settings level carries a digest record, a SHA-256 hash of the whole set, so watchers detect a settings change without reading the values.
  - *Rule:* Settings keys must be non-empty, and empty values are dropped rather than written.
- **Settings archive:** Keeps a departed unit's settings readable. They are copied there when the unit leaves scope and stay readable for the lifetime of the relation, even after the unit itself is gone.
- **Status record:** The relation has a status record with a stored vocabulary. A new relation starts as `joining` (the status record is written when the relation record is created); the vocabulary is `joining`, `joined`, `broken`, `suspending`, `suspended`, `error`. The status domain writes it through `SetRelationStatus`: the leader unit's agent reports `joined`; `suspending` and `suspended` record a suspended relation.
  - *Rule:* The vocabulary is validated on every write: every status may transition to `broken`; `joining` cannot follow `joined` or `broken`; `suspending` cannot follow `broken` or `suspended`; `joined` and `suspended` cannot follow `broken`; `error` requires a message; and a `suspended` write with an empty message retains the reason recorded when the relation was suspending.

**Writers:** The relation service inserts the relation with its endpoint records and its initial settings; the removal service carries the teardown; the status domain validates and writes the status. Every settings write updates the matching digest record. The services do not read the stored records directly: they read the four derived views that join the records into the shapes the domain speaks in.

### Relation settings

Creating a relation creates its settings: one unit settings record per
involved unit and one application settings record per involved
application. Each unit involved in the relation gets a local copy of all
the settings for that relation. Charm documentation often calls these
*relation databags*; Juju's code and hook commands all say
*settings*.

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

A relation is a peer relation when the application relates to itself; otherwise it connects two applications, and it is a subordinate relation when one side is subordinate (always same-model), a cross-model relation when the applications live in different models, and a regular relation when two principal applications share a model.

Every relation is one of four kinds. The kind is not a stored
discriminator: it follows from the relation's own records, which
applications it connects, the same application or two distinct ones, and,
for two distinct applications, whether one of them is subordinate and
whether the two live in the same model.

(peer-relation)=
#### Peer relation

A **peer** relation relates an application to itself: its units relate to
one another by virtue of the application having a `peers` endpoint.

- Created automatically at deployment time; never added by a client. The relation has one endpoint.
- Every unit the application ever has joins the one relation as it
  starts; scaling the application never duplicates it.
- The relation gives the units a shared store of application settings and
  a notification channel: each settings write wakes the other units'
  watchers. This is the mechanism behind {ref}`scaling <scaling>` and
  {ref}`high availability <high-availability>`.

(subordinate-relation)=
#### Subordinate relation

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

A **regular** relation connects two principal applications that live in
the same model. Both sides support the same endpoint interface and have
opposite `provides` / `requires` endpoint roles.

(cross-model-relation)=
#### Cross-model relation

A **cross-model** relation relates applications that live in different
models, on different controllers or clouds. Each model holds its own
half: the consuming model connects through a synthetic application (the
saas) that stands in for the offered application in the offering model.

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

## Relations in the execution layer

A relation has no machinery of its own: its units' agents execute it.
Each side's units run their relation hooks in lockstep; the model side is
record bookkeeping: creating, suspending, resuming, removing.

(the-relations-execution-rules)=
(the-relations-operations)=
### Relation operations

- **Creation:** The integrate operation writes the relation record and wakes both sides: each unit's watcher fires, and the units run their relation hooks in lockstep, `relation-created`, then `relation-joined` and `relation-changed`, exchanging settings as they go. Each settings write wakes the other side for a further `relation-changed`.
  - *Rule:* Entering scope is idempotent: a second enter is a no-op and the settings are not rewritten; even a dying unit or relation may be re-entered if the unit is already in scope, so the teardown hook flow can proceed. The state layer's `relation unit already exists` guard is folded into this no-op.
  - *Rule:* A unit entering a container-scoped relation gets its subordinate unit created, placed on the principal unit's machine.
  - *Related errors:*
    - **`cannot enter scope, unit or relation not alive`**: *Trigger:* Entering scope with a dead or dying unit or relation. *Remediation:* None; the relation or unit is being torn down.
    - **`cannot enter scope, subordinate unit exists but is not alive`**: *Trigger:* A container-scoped relation's subordinate unit exists but is not alive. *Remediation:* Remove the stale subordinate unit.
- **Remote-relation guards:** Remote units enter scope through the remote-relation path.
  - *Related errors:*
    - **`entering scope for peer relation`**: *Trigger:* A remote unit enters a local peer relation. *Remediation:* None; remote units do not enter local peer relations.
    - **`entering scope, unit application is a subordinate`**: *Trigger:* A subordinate application enters remote scope. *Remediation:* None; subordinate applications do not enter remote scope.
- **Status:** Relation status is written only by the leader unit: the write is leadership-gated, and the leader unit's agent reports the relation as `joined`.
- **Removal:** The remove-relation operation follows the cooperative pattern: the relation is marked dying, the units in scope leave as their agents notice, and a scheduled removal job deletes the records once nothing is left in scope. A forced removal tears the relation and its scope entries down without waiting for the agents. Cross-model relations are rejected by the removal pre-check: the local record is only the local half of the relation.
- **Suspending and resuming:** Suspension pauses the data flow across a cross-model relation. The record keeps the suspended flag and its reason, the status moves `suspending`, then `suspended`, and the change propagates to the remote half. Resuming restores the flow and the status returns to `joining`.

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
