---
myst:
  html_meta:
    description: "Juju application reference: application declaration, persistence (the record and its satellites, states, types), execution (deployment, configuration, scaling, refresh, exposure, removal, watchers), and rules."
---

(application)=
# Application

In Juju, an **application** is a running abstraction of a {ref}`charm <charm>` in the Juju {ref}`model <model>`: the software the charm defines, deployed and managed as one record. This could correspond to a traditional software package but it could also be less or more.

An application consists of one or more {ref}`units <unit>`, and it can have {ref}`resources <charm-resource>`, {ref}`configuration <application-configuration>`, {ref}`relations <relation>` through its {ref}`endpoints <application-endpoint>`, and {ref}`actions <action>`. A {ref}`constraint <constraint>` customises the compute its units request; an {ref}`offer <offer>` publishes its endpoints to {ref}`other models <cross-model-relation>`.

(the-applications-declaration)=
## Applications in the declaration layer

You add an application to a model by deploying it, and you configure,
scale, refresh, expose, or remove it through the same clients;
deploying requires {ref}`model write access <user-access-model-write>`.

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>`, {ref}`Terraform Provider for Juju | Manage applications <tfjuju:manage-applications>`
```

(the-applications-persistence)=
(the-application-record)=
(the-application-in-the-data-model)=
(the-application-states)=
(types-of-application)=
## Applications in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Application attributes
:alt: The application's stored tables as an entity-relationship slice: the application record at the centre with its uuid, name, life and charm pointer; the charm record it references west; the origin channel below the charm; the status record east with the endpoint record below it; the configuration keys south. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory.
:caption: Entity relationship diagram: The application's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The application references the charm it deploys by UUID; its origin (track/risk/branch) and the endpoints it instantiates from the charm are separate records; the status record and the config keys hang off the application itself.
```

In the model database an application is a **native record** -- Juju's
own, created by the deployment machinery, one row per application per
model (the DDL: `0019-application.sql`, with the endpoint row in
`0024-relation.sql`). The row carries the name the client chose, its
life, the {ref}`charm <charm>` it was deployed from -- stored as the
charm's UUID, a mutable pointer a refresh rewrites (the URL is
reconstructed from the charm's own fields) -- the counter that bumps
each time that pointer is rewritten (`charm_modified_version`), and
the {ref}`space <space>` its endpoints bind to by default. The
satellites in the picture are the application's own: the origin it
tracks (`application_channel`: track/risk/branch, with the base from
`application_platform`), the endpoints the charm defines,
instantiated for this application (`application_endpoint`), the
aggregated status (`application_status`), and the configuration keys
(`application_config`, one row per key).

The identity pair: the primary key (`application.uuid`) is the join
handle -- it exists so the charm pointer, the origin, the endpoints,
the status and the config keys have something to point at. The natural
key the client names is `application.name` (UNIQUE per model):
lowercase letters, digits and hyphens, starting with a letter, every
hyphen-separated segment containing a letter (`my-app-2`, never
`-myapp` or `MYAPP`); the creation service checks the name against the
model's existing applications and rejects a duplicate with
`application already exists`.

Every foreign key is an assertion the record holds: the application
row holds the charm pointer (`application.charm_uuid` -- a refresh
rewrites it); the application_status, application_channel and
application_endpoint rows hold the application pointer each; the
application_config rows hold one pointer per key. The charm's endpoint
definition an endpoint instantiates lives on the charm side
(`application_endpoint.charm_relation_uuid` -- {ref}`charm <charm>`'s
story). The application row also holds its default-binding space
pointer (`application.space_uuid`) -- in the picture as a field, not
an edge: the {ref}`space <space>` record's story is its own page's.
Not drawn above, all assertions the schema states: the controller
marker (`application_controller` -- a dedicated singleton, one row, in
one application, enforced by the schema), the Kubernetes scale record,
the expose tables (which grant {ref}`spaces <space>` and CIDRs to
endpoints), the config-hash record that tells watchers when the config
changed, the one-boolean trust record (`application_setting`), and the
remote-offerer pair -- an application record the consuming model
synthesises to stand for an application it cannot see, paired with a
remote-offerer record carrying the far model's identity and the offer
URL; nothing is deployed behind it (see {ref}`offer <offer>`).

The application table has no type column: an application's kind is
derived from the records around it, and the kinds are not mutually
exclusive -- the controller application is also just an application.
Most applications are **regular applications**: a {ref}`charm <charm>`
deployed into the model, with its units and their machines or pods.

Two projections with writers of their own:

- **life** -- the shared alive / dying / dead cycle every entity has.
  An application is created alive; the removal machinery marks it
  dying (guarded one-way, cascading to its units, relations and
  machines) and declares it dead only once no units and no relations
  are left -- the removal machinery in the execution layer drives it,
  the record here stores it; a scheduled removal job then deletes the
  records (see {ref}`Application removal <the-application-removal>`).
- **`application_status`** -- the application-level summary, written
  only by the application's **leader unit** (through its agent; the
  charm hook command is `status-set --application`,
  controller-gated to the leader). When the record is unset, the
  application's display status is computed instead by aggregating the
  units' workload statuses by severity -- error first, then blocked,
  maintenance, waiting, active, terminated, unknown. The value
  vocabulary is the workload vocabulary the units share (see
  {ref}`workload / charm status <workload--charm-status>`); the
  who-writes story across all five status domains is the
  {ref}`Status domains <status>` view.

Neither is transition-validated: a life advance is a one-way guarded
update and a status write is a membership check (the value must be
known) plus an owner check -- what constrains an application is who
writes, not a transition matrix.

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

(the-applications-execution)=
(the-application-operations)=
## Applications in the execution layer

By the time the command returns, the application's records exist --
and its units may still be provisioning: the machinery that realizes
the application is not the application's own. It has no machinery of
its own -- its units' agents execute it; the model side is controller
bookkeeping. Operations on applications split by concern: deployment
creates the application and its units; configuration, scaling,
refresh and exposure mutate the running application; removal tears it
down. All of them are controller-API operations -- none is
leader-gated; the leader only owns the status write (see
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
### Application watchers

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

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-application-rules-and-errors)=
## Application rules and errors

The application domain encodes its rules as a typed error taxonomy;
each error names the rule it enforces. The rules themselves are stated
where they belong: the name grammar in the persistence layer, the
mutation gates in the execution layer's operations and states. The
errors matter to charm authors deploying and configuring applications
and to Juju developers, who maintain them as the domain's validation
law.

The errors that encode them:

- *Existence and life*: `application not found`,
  `application already exists`, `application not alive`,
  `application is dead`, `charm not found`, `charm not resolved`.
- *Names and validation*: `application name not valid`,
  `invalid application configuration`,
  `invalid application constraints`, `incompatible base`.
- *Composition*: `application has units`, `application has relations`,
  `application has different charm`, `units upgrading`.
- *Scaling*: `scale change invalid`, `scaling state inconsistent`.
