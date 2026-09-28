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

- **Deployment:** Deploying creates the application: the name, the {ref}`charm <charm>`, the channel and base it tracks, the initial configuration, the {ref}`constraints <constraint>` its units request. Deploying requires {ref}`model write access <user-access-model-write>`.
- **Operations:** Configuring, scaling, refreshing, exposing and removing act on the running application through the same clients; all of them are controller-API operations.

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>`, {ref}`Terraform Provider for Juju | Manage applications <tfjuju:manage-applications>`
```

(the-applications-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - The name is lowercase letters, digits and hyphens, starting with a
    letter, every hyphen-separated segment containing at least one letter
    (`my-app-2`, never `-myapp` or `MYAPP`).
  - The name is unique per model: the creation path checks the model's
    existing applications before it writes.
  - Every creating or mutating operation requires model write access.
- **Errors:**
  - **`application name not valid`:** Triggered when a name fails the
    grammar. Remediation: start with a lowercase letter; use letters,
    digits and hyphens, with at least one letter in every segment.
  - **`application already exists`:** Triggered when creating an
    application whose name the model already uses. Remediation: choose
    another name.

(the-applications-persistence)=
(the-application-in-the-data-model)=
(the-application-states)=
## Applications in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Application attributes
:alt: The application record at the centre with its salient columns; the charm record it references west; the status record east; the config, origin channel and endpoint records below. Each line is a stored pointer; 1/m at each end.
:caption: The application's stored records. An application is one record referencing the charm it deploys, its origin channel, the endpoints it instantiates from the charm, its config keys and its status record; every line is a foreign key in one of those records.
```

In the {ref}`model database <data-model-full-spine>` an application is a **native record**, created
by the deployment machinery, one record per application per model. The
drawn slice is the record set every application carries; the entity's
remaining records are the satellites of its specific roles:

- **Every application is anchored by a single primary entry containing
  its essential identifiers:** an internal system ID the other records
  point at, and the name the client types, unique per model.
- **An application has a life** (alive, dying, dead) and **references
  the {ref}`charm <charm>` it deploys** (stored as a pointer to the
  charm's record, one a refresh rewrites; the URL is reconstructed from
  the charm's own fields).
- **An application has a default {ref}`space <space>`** its endpoints
  bind to.
- **An application has an origin**: The channel it tracks
  (track/risk/branch) and the platform it deployed onto (OS, channel,
  architecture; the origin projections join the charm's reference
  name, source, revision and hash, and the platform, into the origin
  the clients read).
- **An application has the endpoints** of the charm's {ref}`endpoint
  <application-endpoint>` definitions instantiated for it.
- **An application has configs**: The keys the client sets (one
  record per key, its type mirrored from the charm schema), the
  SHA-256 hash the config watchers fire on, and the one-boolean trust
  record.
- **An application has a status record**: The application-level
  summary.

The role satellites the record set rounds out: the controller marker
(the single-record marker for the one controller application), the
Kubernetes scale record (the current scale, the target, the scaling
flag), the expose grants per endpoint (a NULL endpoint is the
wildcard), the compute the units request (joined onto the {ref}`constraint
<constraint>` record), the workload version the application reports,
the application agent's credentials, the Kubernetes service a CAAS
deployment exposes (bound to the net node record), and the
remote-offerer pair: an application record the consuming model
synthesises to stand for an application it cannot see, paired with a
remote-offerer record carrying the far model's identity and the offer
URL; nothing is deployed behind it (see {ref}`offer <offer>`).

The identity pair: the primary key (the internal id) is the join
handle; the natural key the client names is the application's name,
unique
per model. The application record has no type column: an application's
kind is derived from the records around it, and the kinds are not
mutually exclusive; the controller application is also just an
application. Most applications are **regular applications**: a
{ref}`charm <charm>` deployed into the model, with its units and their
machines or pods.

Two projections with writers of their own:

- **life:** The shared alive / dying / dead cycle every entity has. An
  application is created alive; the removal machinery marks it dying
  (guarded one-way, cascading to its units, relations and machines) and
  declares it dead only once no units and no relations are left; a
  scheduled removal job then deletes the records (see {ref}`Application
  removal <the-application-removal>`).
- **The status record:** The application-level summary, written only
  by the application's **leader unit** (through its agent; the charm
  hook command is `status-set --application`, controller-gated to the
  leader). When the record is unset, the application's display status
  is computed by aggregating the units' workload statuses by severity:
  error, then blocked, maintenance, waiting, active, terminated,
  unknown. The value vocabulary is the workload vocabulary the units
  share (see {ref}`workload / charm status <workload--charm-status>`);
  the who-writes story across all five status domains is the
  {ref}`Status domains <status>` view.

Neither projection is transition-validated: a life advance is a one-way
guarded update; a status write is a membership check (the value must be
known) plus an owner check. What constrains an application is who
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

In the data model, each endpoint the application uses is a record
tying the application to the charm's endpoint definition; a
{ref}`relation <relation>` attaches to it through the
relation-endpoint record, and an extra-binding record can tie the
endpoint to a specific {ref}`space <space>`.

(the-applications-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - Every satellite's foreign key points at the application's id; the
    charm's endpoint definition an endpoint instantiates lives on the
    charm side (the endpoint record carries the charm endpoint's id).
  - The charm pointer is mutable: a refresh rewrites it in the same
    transaction that updates and creates the storage directives, merges
    the endpoint bindings, re-filters the config records and bumps the
    charm's modification counter.
  - A config record's type mirrors the charm schema's type for that
    key; the value is nullable (a key set to no value).
  - **Life:** Created alive; marked dying one-way by the removal
    machinery (the cascade covers the units, their relations, machines
    and storage); dead only once no units and no relations reference
    the application; the scheduled removal job then deletes the
    records.
  - **Status:** The leader unit writes it (through its agent;
    `status-set --application` is controller-gated to the leader);
    unset, the display status aggregates the units' workload statuses
    by severity. Neither projection is transition-validated: a life
    advance is a one-way guarded update; a status write is a membership
    plus owner check.
- **Errors:**
  - **`application is not alive`:** Triggered when an operation that
    requires an alive application targets a dying one (adding units,
    model migration). Remediation: none; the application is being
    removed.
  - **`application is dead`:** Triggered when an operation that
    tolerates a dying application targets a dead one (configuration,
    scale and constraint writes). Remediation: none; the records are
    being deleted.
  - **`application has units`:** Triggered when the dead gate finds
    units still referencing the application. Remediation: remove the
    units first, or let the cascade finish.
  - **`application has relations`:** Triggered when the dead gate finds
    relations still referencing the application. Remediation: remove
    the relations first, or let the cascade finish.
  - **`units upgrading`:** Triggered when a model migration's readiness
    check finds units still upgrading their charms. Remediation: wait
    for the upgrades to finish.

(the-applications-execution)=
(the-application-operations)=
## Applications in the execution layer

By the time the operation returns, the application's records exist, and
its units may still be provisioning: the machinery that realizes the
application is not the application's own. It has no machinery of its
own: its units' agents execute it; the model side is controller
bookkeeping. Operations on applications split by concern: deployment
creates the application and its units; configuration, scaling, refresh
and exposure mutate the running application; removal tears it down.
All of them are controller-API operations; none is leader-gated, the
leader only owns the status write (see {ref}`Application states
<the-application-states>`).

(the-application-deployment)=
### Application deployment

Deploying an application adds its software to a {ref}`model <model>`
and arranges for it to run on infrastructure. The intent (the
application name, the {ref}`charm <charm>`, the {ref}`constraints
<constraint>`) goes to the {ref}`controller <controller>` as an RPC
call. The controller writes the application and {ref}`unit <unit>`
records to the {ref}`database <database>`, then asks the cloud for
resources (a virtual machine or a pod). Once the resource is ready the
controller starts the {ref}`unit agent <unit-agent>`, which runs the
install sequence: `install`, `config-changed`, `start`.

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
:slide-captions: Entity relationship diagram: The seed: the records a deployment consists of -- charm, application, unit, machine/pod, relation, endpoint -- and where the pointers live. | Sequence diagram: The mechanism: the controller writes the application and unit records, asks the cloud to provision a machine, and starts jujud, which runs the install hooks to unit active. | Topology: The result: one jujud per machine -- the controller machine's jujud runs the controller with Dqlite in-process; the unit machine's jujud hosts the unit agent, which runs the charm, which drives the workload directly (no Pebble on machine clouds). | Sequence diagram: The verification: juju status projects exactly those records plus live agent liveness -- what you just deployed is what status reads.
:alt: The data model records (charm, application, unit, machine/pod, relation, endpoint). Then: user invokes juju deploy; controller writes records and provisions a machine; jujud runs the install hooks and reports active. The resulting topology: controller machine and unit machine, one jujud per machine. Verification: juju status reads those records plus live agent liveness.
```

::::

:::::

(the-application-configuration)=
### Application configuration

The application's configuration is the charm's config schema
instantiated: the {ref}`charm <charm>` defines the options (key, type,
default, description) in its config schema; the application stores one
record per key a client sets, the value nullable (a key set to no
value).
A read with defaults falls back per key to the charm's default where
no override is set.

Setting configuration validates against the charm's schema: an unknown
key or a value that does not parse to the option's type is invalid; a
secret-typed option must be set to a secret URI; the total size of all
keys and values is capped at 16 MB. The `trust` key never reaches the
config records: it is intercepted into the one-boolean settings record,
where it grants or withholds the units' access to the model's cloud
credentials. Clearing a key removes its record; keys that do not exist
are ignored. Setting configuration by YAML payload is not supported on
this version; synthetic (remote-offerer) applications refuse
configuration operations.

Every write refreshes the config hash, the SHA-256 over the sorted
keys and values plus the trust flag: the hash change is what fires the
units' {ref}`config-changed <hook-config-changed>` hooks. The charm
reads its configuration at runtime through {ref}`config-get
<hook-command-config-get>`; without the all flag, keys set to no value
are hidden from the charm.

(the-application-scaling)=
### Application scaling (Kubernetes)

On Kubernetes models the application carries a scale record: the
current scale, the target, and whether scaling is in progress (the DDL
scopes the record to Kubernetes applications). Setting the scale writes
the record and the provisioner reconciles the pod count to the target;
a negative target and a scale started toward a different target while
one is in flight are refused. The scale watcher fires only when the
scale value changes.

(the-application-refresh)=
### Application refresh

Refreshing swaps the charm the application references: the controller
validates relation compatibility transactionally, updates and creates
the storage directives for the new charm, merges the endpoint bindings,
re-filters the configuration keys to the new charm's schema (keys the
new charm does not define are dropped), rewrites the charm pointer and
bumps the charm-modified counter. A base change needs the force flag.

(the-application-exposure)=
### Application exposure

Exposing opens the application's endpoints to the outside: the expose
records grant access per endpoint, either to a {ref}`space <space>` or
to a CIDR, an omitted endpoint meaning all of them. Grants naming
neither a space nor a CIDR fall back to the all-networks CIDRs, IPv4
and IPv6. Un-exposing removes the grants; the exposure watcher fires on
grant changes.

(the-application-removal)=
### Application removal

Removal is initiated through the remove-application operation and
follows the same cooperative pattern as every entity removal: the
application and its units, relations, machines and storage are marked
dying in one cascade, removal jobs are scheduled, and each entity is
deleted only when its own teardown allows it (see {ref}`removing things
<removing-things>`). The `--force` mode skips the dead-state and
resource gates; on Kubernetes the application cannot be removed while
the provisioner still manages its resources.

(the-application-watchers)=
### Application watchers

Nothing about an application is polled by the things that act on it:
they watch it. The application domain's watchable service exposes these
watch surfaces (what a watcher fires on, not who consumes it; see
{ref}`the unit agent <unit-agent>` and {ref}`the controller agent
<controller-agent>` for the consumers' side):

- **The application record** and **all applications:** Changes to one
  application's record, and applications being added or removed.
- **An application's units' life** and **one unit's life:** The life
  of the application's units and of a single unit.
- **The application's configuration:** Three surfaces: the config keys
  and values, the config hash (the compact change signal), and the
  application's own settings record.
- **The application's scale:** Fires only when the scale value
  changes.
- **The units' addresses and their bindings:** Two surfaces: the
  address changes and the compact address-hash signal.
- **The units added or removed on a machine** and **one unit for the
  legacy uniter:** The machine-scoped and uniter-scoped unit surfaces.
- **The application's charms:** Two surfaces: charm changes for the
  application, and applications whose charm is still unresolved.
- **The application's exposure:** Changes to the expose grants.

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-applications-execution-rules)=
### Execution rules and errors

- **Rules:**
  - Every operation requires model write access; the leader owns only
    the status write.
  - A configuration write validates against the charm's schema, mirrors
    the option's type into the config record, intercepts `trust` into
    the settings record, and refreshes the config hash.
  - The total size of the configuration keys and values is capped at
    16 MB.
  - A constraints write overwrites the full constraint set; an invalid
    container type or an unknown space is refused.
  - A refresh validates relation compatibility transactionally and
    re-filters the configuration keys to the new charm's schema.
  - A scale target is non-negative; a scale started toward a different
    target while one is in flight is refused.
- **Errors:**
  - **`application not found`:** Triggered when an operation names an
    application the model does not have. Remediation: check the name
    and the model.
  - **`invalid application config`:** Triggered when a configuration
    write names a key the charm does not define or a value that does
    not parse to the option's type. Remediation: set keys the charm's
    schema defines, with parseable values.
  - **`invalid secret config`:** Triggered when a secret-typed option
    is set to a string that is not a secret URI. Remediation: set a
    secret URI, or clear the option.
  - **`quota limit exceeded`:** Triggered when a write pushes the total
    size of the configuration keys and values over the 16 MB cap
    (16,777,216 bytes). Remediation: clear keys or shorten values.
  - **`invalid application constraints`:** Triggered when a constraints
    write names an invalid container type or a space the model does not
    have. Remediation: use a valid container type and an existing
    space.
  - **`incompatible base for charm`:** Triggered when a refresh changes
    the application's base incompatibly without the force flag.
    Remediation: refresh within the base, or force the base change.
  - **`charm not found`:** Triggered when the charm the operation needs
    is not in the model's charm records. Remediation: check the charm
    reference.
  - **`charm not resolved`:** Triggered when the charm is known but its
    archive is not yet available. Remediation: wait for the charm
    download to finish; check the charm's source.
  - **`scale change invalid`:** Triggered when a scale operation sets a
    negative scale or removes more units than exist. Remediation: scale
    within the current count.
  - **`scaling state is inconsistent`:** Triggered when a scale starts
    toward a different target while a scale is already in flight.
    Remediation: let the running scale finish, then scale again.

