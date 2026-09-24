---
myst:
  html_meta:
    description: "Juju application reference: the application record, application kinds, the application in the data model, application states, operations, watchers, and rules."
---

(application)=
# Application

```{ibnote}
See also: {ref}`manage-applications`
```

In Juju, an **application** is a running abstraction of a {ref}`charm <charm>` in the Juju {ref}`model <model>`: the software the charm defines, deployed and managed as one record. This could correspond to a traditional software package but it could also be less or more.

An application consists of one or more {ref}`units <unit>`, and it can have {ref}`resources <charm-resource>`, {ref}`configuration <application-configuration>`, {ref}`relations <relation>` through its {ref}`endpoints <application-endpoint>`, and {ref}`actions <action>`.

(the-application-record)=
## The application record

An application is a record in the model database. It is identified by
its **name** -- unique per model, lowercase letters, digits and
hyphens -- and it carries the application's life (see
{ref}`Application states <the-application-states>`), the
{ref}`charm <charm>` it was deployed from (stored as the charm's UUID,
not its URL), the charm revision the application is pinned to, and the
{ref}`space <space>` its endpoints bind to by default.

(types-of-application)=
## Types of application

The application table has no type column: an application's kind is
derived from the records around it, and the kinds are not mutually
exclusive -- the controller application is also just an application.
Most applications are **regular applications**: a {ref}`charm <charm>`
deployed into the model, with its units and their machines or pods.

### The controller application

The application that runs Juju itself in the
{ref}`controller model <the-controller-model>` is marked by a dedicated
singleton record -- one row, in one application, enforced by the
schema. It is the application whose units host the controller's
workers (see {ref}`the controller agent <controller-agent>`).

### The remote-offerer application

The far half of a {ref}`cross-model relation <cross-model-relation>`
is an application record the consuming model synthesises to stand for
an application it cannot see: it pairs with a remote-offerer record
carrying the far model's identity and the offer URL. Nothing is
deployed behind it -- the units live in the offering model (see
{ref}`offer <offer>`).

(the-application-in-the-data-model)=
## The application in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Application attributes
:alt: The application's stored tables as an entity-relationship slice: the application record at the centre; the charm it references west with its origin channel below; the status record east with the endpoint record below it; the configuration keys south. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The application's stored records and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The application references the charm it deploys by UUID; its origin (track/risk/branch, with the base) and the endpoints it inherits from the charm are separate records; the status record and the config keys hang off the application itself.
```

The application's state is spread across a handful of stored tables.
The `application` record carries the name, the life, the charm
reference (the charm's UUID -- the URL is reconstructed from the
charm's own fields), the charm revision the application is pinned to,
and the default-binding space. The `application_status` record is the
application's aggregated status (see
{ref}`Application states <the-application-states>`); the
`application_config` rows are the application's
{ref}`configuration <application-configuration>` keys and values, with
a hash record that tells watchers when the config changed; a
one-boolean `application_setting` record holds the application's trust
flag; the `application_channel` record (with the base from
`application_platform`) pins the {ref}`charm <charm>` origin the
application tracks; the `application_endpoint` records are the
endpoints the charm defines, instantiated for this application.

Not drawn above: the controller-application marker, the Kubernetes
scale record, the expose tables (which grant
{ref}`spaces <space>` and CIDRs to endpoints), and the
remote-offerer pair -- all covered under
{ref}`Types of application <types-of-application>` and
{ref}`Application operations <the-application-operations>`.

(application-endpoint)=
### Application endpoint

In Juju, an application **endpoint** is a struct defined in an
{ref}`application <application>`'s {ref}`charm <charm>`'s
`metadata.yaml` / (since Charmcraft 2.5) `charmcraft.yaml` consisting of
- a name (charm-specific),
- a role (one of `provides`, `requires` = 'can use', or `peers`), and
- an interface

whose purpose is to help define a {ref}`relation <relation>`.

For example, the MySQL application deployed from the `mysql` charm has an endpoint called `mysql` with role `provides` and interface `mysql` and this can be used to form  a {ref}`regular relation <regular-relation>` relation with WordPress.

```{ibnote}
See more: [GitHub | `mysql-operator` > `metadata.yaml`](https://github.com/canonical/mysql-operator/blob/2bd2bcc65590937dab18d1d9b0fe21a445557bb6/metadata.yaml#L35), [Charmhub | `mysql`](https://charmhub.io/mysql/integrations#mysql)
```

All charms have an implicit (not in their `metadata.yaml` / `charmcraft.yaml`) endpoint with name `juju-info`, interface `juju-info`, and role `provides`. This endpoint can be used to form {ref}`subordinate relations <subordinate-relation>` with subordinate charms that have an explicit endpoint with interface `juju-info` and role `requires`. See {ref}`the-implicit-juju-info-relation-endpoint` for details. Examples: [`ntp`](https://charmhub.io/ntp/integrations#juju-info), [`mysql-router`](https://charmhub.io/mysql-router/integrations#juju-info)

In the data model, each endpoint the application uses is an
`application_endpoint` record tying the application to the charm's
endpoint definition; a {ref}`relation <relation>` attaches to it
through the relation-endpoint record, and an extra-binding record can
tie the endpoint to a specific {ref}`space <space>`.

(the-application-states)=
## Application states

An application carries two orthogonal pieces of state: its **life** --
the shared alive / dying / dead cycle every entity has -- and its
**status record**, the application-level summary a charm's leader unit
reports. Neither is transition-validated: the life advances one way
under the removal machinery, and a status write is a membership check
(the value must be known) plus an owner check -- what constrains an
application is who writes, not a transition matrix.

### Life

An application is created alive. The removal machinery marks it dying
(guarded one-way, cascading to its units, relations and machines) when
the application is removed, and declares it dead only once no units
and no relations are left; a scheduled removal job then deletes the
records (see {ref}`Application removal
<the-application-removal>`).

### Status

The application's status is its own record, written only by the
application's **leader unit** (through its agent; the charm hook
command is `status-set --application`, controller-gated to the
leader). When the record is unset, the application's display status is
computed instead by aggregating the units' workload statuses by
severity -- error first, then blocked, maintenance, waiting, active,
terminated, unknown. The value vocabulary is the workload vocabulary
the units share (see {ref}`workload / charm status
<workload--charm-status>`); the who-writes story across all five
status domains is the {ref}`Status domains <status>` view.

(the-application-operations)=
## Application operations

Operations on applications split by concern: deployment creates the
application and its units; configuration, scaling, refresh and
exposure mutate the running application; removal tears it down. All of
them are controller-API operations -- none is leader-gated; the leader
only owns the status write (see
{ref}`Application states <the-application-states>`).

(the-application-deployment)=
### Application deployment

Deploying an application adds its software to a {ref}`model <model>` and arranges for it to run on infrastructure. The intent -- the application name, the {ref}`charm <charm>`, the {ref}`constraints <constraint>` -- goes to the {ref}`controller <controller>` as an RPC call. The controller writes the application and {ref}`unit <unit>` records to the {ref}`database <database>`, then asks the cloud for resources (a virtual machine or a pod). Once the resource is ready the controller starts the {ref}`unit agent <unit-agent>`, which runs the install sequence: `install`, `config-changed`, `start`.

The mechanism and the state it leaves, per cloud type:

:::::{tab-set}

::::{tab-item} Kubernetes

```{ggarch}
:file: ../juju.ggarch
:slides: Data model | Deploy K8s | K8s deployment topology | juju status
:caption: Deploying on Kubernetes: the records, the mechanism that creates them, the topology that results, and the command that verifies it.
:slide-captions: Entity relationship diagram: The seed: the records a deployment consists of -- charm, application, unit, machine/pod, relation, endpoint -- and where the pointers live. | Sequence diagram: The mechanism: the controller writes the application and unit records, schedules the unit pod, and starts the containeragent, which runs the install hooks to unit active. | Topology: The result: the settled topology -- the controller pod and the unit pod, each with their internal agents and containers. | Sequence diagram: The verification: juju status projects exactly those records plus live agent liveness -- what you just deployed is what status reads.
:alt: The data model records (charm, application, unit, machine/pod, relation, endpoint). Then: user invokes juju deploy; controller writes records and schedules the unit pod; containeragent runs the install hooks and reports active. The resulting topology: controller pod and unit pod with their internal agents and containers. Verification: juju status reads those records plus live agent liveness.
```

::::

::::{tab-item} Machines

```{ggarch}
:file: ../juju.ggarch
:slides: Data model | Deploy machine | Machine deployment topology | juju status
:caption: Deploying on a machine cloud: the records, the mechanism that creates them, the topology that results, and the command that verifies it.
:slide-captions: Entity relationship diagram: The seed: the records a deployment consists of -- charm, application, unit, machine/pod, relation, endpoint -- and where the pointers live. | Sequence diagram: The mechanism: the controller writes the application and unit records, asks the cloud to provision a machine, and starts jujud, which runs the install hooks to unit active. | Topology: The result: one jujud per machine -- the controller machine's jujud runs the controller with Dqlite in-process; the unit machine's jujud hosts the unit agent, which runs the charm, which drives the workload directly (no Pebble on machine clouds). | Sequence diagram: The verification: juju status projects exactly those records plus live agent liveness -- what you just deploye…
:alt: The data model records (charm, application, unit, machine/pod, relation, endpoint). Then: user invokes juju deploy; controller writes records and provisions a machine; jujud runs the install hooks and reports active. The resulting topology: controller machine and unit machine, one jujud per machine. Verification: juju status reads those records plus live agent liveness.
```

::::

:::::

```{ibnote}
See more: {ref}`manage-applications`
```

(the-application-configuration)=
### Application configuration

Setting configuration (for example, `juju config mysql tune=fast`)
writes the application's config keys and values -- validated against
the charm's config schema -- and refreshes the config hash, which is
what the application's config watchers fire on. Clearing a key removes
the row; the trust flag lives in its own one-boolean record.

(the-application-scaling)=
### Application scaling (Kubernetes)

On Kubernetes models the application carries a scale record -- the
current scale, the target, and whether scaling is in progress. Setting
the scale (for example, `juju scale-application mysql 3`) rejects
negative values and inconsistent scaling states; the provisioner
reconciles the pod count to the target.

(the-application-refresh)=
### Application refresh

Refreshing (for example, `juju refresh mysql`) swaps the charm the
application references: the controller validates the new charm's
storage and base compatibility, then rewrites the application's charm
reference and re-pins the revision. A base change needs the
force-base flag; a charm that does not match the application's is
rejected.

(the-application-exposure)=
### Application exposure

Exposing an application (for example, `juju expose mysql`) opens its
endpoints to the outside: the expose records grant access per endpoint
either to a {ref}`space <space>` or to a CIDR, an omitted endpoint
meaning all of them. Un-exposing removes the grants.

(the-application-removal)=
### Application removal

Removal (for example, `juju remove-application mysql`) is initiated
through the remove-application operation. Removal follows the same
cooperative pattern as every entity removal: the application and its
units, relations, machines and storage are marked dying in one
cascade, removal jobs are scheduled, and each entity is deleted only
when its own teardown allows it (see {ref}`removing things
<removing-things>`). The `--force` mode skips the dead-state and
resource gates; on Kubernetes the application cannot be removed while
the provisioner still manages its resources.

(the-application-watchers)=
## Application watchers

Nothing about an application is polled by the things that act on it:
they watch it. The application domain's watchable service exposes
these watch surfaces -- what a watcher fires on, not who consumes it
(see {ref}`the unit agent <unit-agent>` and
{ref}`the controller agent <controller-agent>` for the consumers'
side):

- **The application row** and **all applications** -- notifies on
  changes to one application's record and on applications being added
  or removed.
- **An application's units' life** and **one unit's life** -- notifies
  on the life of the application's units and of a single unit.
- **The application's configuration** -- three surfaces: the config
  keys and values, the config hash (the compact change signal), and
  the application's own settings record.
- **The application's scale** -- fires only when the scale value
  changes.
- **The units' addresses and their bindings** -- two surfaces: the
  address changes and the compact address-hash signal.
- **The units added or removed on a machine** and **one unit for the
  legacy uniter** -- the machine-scoped and uniter-scoped unit
  surfaces.
- **The application's charms** -- two surfaces: charm changes for the
  application, and applications whose charm is still unresolved.
- **The application's exposure** -- fires on changes to the expose
  grants.

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

(the-application-rules-and-errors)=
## Application rules and errors

The application domain encodes its rules as a typed error taxonomy;
each error names the rule it enforces. The rules matter to charm
authors deploying and configuring applications and to Juju developers,
who maintain them as the domain's validation law.

The rules an **application name** must satisfy:

- lowercase letters, digits and hyphens; it starts with a letter and
  every hyphen-separated segment contains a letter
  (`my-app-2`, never `-myapp` or `MYAPP`);
- the name is unique per model.

The rules an **application mutation** must satisfy:

- configuration is validated against the charm's config schema
  (`invalid application configuration`);
- a refresh must match the application's charm unless forced, and its
  base must be compatible (`incompatible base`);
- the scale must not go negative and the scaling state must be
  consistent (`scale change invalid`, `scaling state inconsistent`);
- an application can only be declared dead once it has no units and no
  relations; the record cannot be deleted while it is alive
  (see {ref}`Application removal <the-application-removal>`).

The errors that encode them:

- *Existence and life*: `application not found`,
  `application already exists`, `application not alive`,
  `application is dead`, `charm not found`, `charm not resolved`.
- *Names and validation*: `application name not valid`,
  `invalid application configuration`,
  `invalid application constraints`.
- *Composition*: `application has units`, `application has relations`,
  `application has different charm`, `units upgrading`.
- *Scaling*: `scale change invalid`, `scaling state inconsistent`.

(related-entities-application)=
## Related entities

- **Charms** are what an application runs: the application references
  one charm by UUID and tracks its origin; a refresh swaps it
  (see {ref}`charm <charm>`).
- **Units** are the application's running instances -- one or more,
  cascaded with the application on removal (see {ref}`unit <unit>`).
- **Endpoints** are the charm-defined ports the application offers;
  {ref}`relations <relation>` attach to them.
- **Configuration** is the application's key/value payload, set
  against the charm's schema (see
  {ref}`application configuration <application-configuration>`).
- **Constraints** customise the compute the application's units
  request (see {ref}`constraint <constraint>`).
- **Resources** and **actions** are the charm's extra payloads: files
  attached at deploy time and named operations exposed to the user
  (see {ref}`charm resource <charm-resource>`,
  {ref}`action <action>`).
- **Offers** publish the application's endpoints to other models
  (see {ref}`offer <offer>`).
- **Status** owns the application's status record, its writer rule and
  the display aggregation (see
  {ref}`Application states <the-application-states>`).
- **Removal** owns the teardown that takes a dying application to
  dead, units and relations cascading with it (see
  {ref}`Application removal <the-application-removal>`).
- **Spaces** provide the default binding the application's endpoints
  use (see {ref}`space <space>`).
