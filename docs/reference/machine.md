---
myst:
  html_meta:
    description: "Juju machine reference: compute resources on bare metal, VMs, and LXD containers. The machine record, machine kinds, machine states, provisioning, and machine watchers."
---

(machine)=
# Machine
```{audience} user
```

```{ibnote}
See also: {ref}`manage-machines`
```

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` requested implicitly (e.g., through {ref}`command-juju-deploy`, {ref}`command-juju-add-unit`, etc.) or explicitly (e.g., through {ref}`command-juju-add-machine`) from a machine {ref}`cloud <cloud>`. Its neighbours: the {ref}`units <unit>` that run on it, the {ref}`constraints <constraint>` and {ref}`placement directives <placement-directive>` that shape its provisioning, and the {ref}`storage <storage>` attached to it.

```{important}

Juju provisions more than bare instances: a LXD container on a regular cloud instance is also, from the point of view of Juju, a 'machine'. Everything in this document applies to both.

From the point of view of an end user this is true with one small caveat -- even though listed in `juju` outputs under 'Machines', and in general handled via the same CLI commands as a machine, a LXD container provisioned on top of a regular cloud instance will be named after its host machine; e.g., `0/lxd/5` = LXD container `5` on machine `0`.

```

## Machines in the declaration layer

You add a machine explicitly with `juju add-machine` and remove it
with `juju remove-machine` (model {ref}`write access
<user-access-model-write>` -- the Machinemanager facade gates both
the add and the remove on model write); most machines are never
declared on their own: they are requested implicitly, when a deploy
or add-unit placement asks for compute. The controller-side
counterpart is the same call over the controller API.

```{ibnote}
See also: {ref}`tfjuju:manage-machines <tfjuju:manage-machines>`
```

(machine-designations)=
### Machine designations

In Juju, many different commands have a machine argument. The shape of this argument depends on whether the machine is existing vs. new and a regular cloud instance vs. a LXD container on top of a regular cloud instance. The argument can also contain combinations, in comma-separated format. The examples below illustrate all the various cases:

| shape of the machine argument | meaning|
|-|-|
|  | a new machine |
|`0`| machine 0 (an existing machine) |
|`0,4`| machines 0 and 4 (existing machines)|
| `lxd` | a new LXD container or (if specified with `virt-type=virtual-machine`) VM on a new machine |
| `lxd:25`| a new LXD container or (if specified with `virt-type=virtual-machine`) VM on the existing machine 25|
| `0/lxd/4`| the existing LXD container `4` on machine `0`|
|`3,0/lxd/2,lxd:5`| machine 3, existing LXD container 2 on machine 0, and a new LXD container on machine 5|
|`0/lxd/01`| invalid -- numbers are `0` or positive integers without leading zeros|
|`0/lxd/0/lxd/0`| invalid -- only one level of container nesting is supported|

These designations are for machine clouds only: the `--to` argument is rejected on Kubernetes models. The two provisioning paths the designations name are drawn in the execution layer.

(types-of-machine)=
(machine-lxd-container)=
### LXD container

An **LXD container** is a machine whose record is linked to another
machine's by a machine-parent record. Each machine can have a single
parent and be the parent of many children, and only one level of
nesting is supported -- a container cannot have a container.

The container is provisioned by its host machine's agent, not by the
controller -- which is also why adding a container brings its host
machine in as a machine of its own. For example, `juju add-machine
lxd` starts a LXD container on a new machine and adds *both* as
'machines' -- the only difference being that the container 'machine'
is prefixed with the ID of its host machine and the annotation `lxd`:

```text
$ juju add-machine lxd
created container 1/lxd/0

$ juju machines
Machine  State    Address         Inst id        Base          AZ  Message
0        started  10.154.118.110  juju-dadfb7-0  ubuntu@22.04      Running
1        pending                  pending        ubuntu@22.04      Creating container
1/lxd/0  pending                  pending        ubuntu@22.04
```

And if you then deploy an application to a LXD container (without specifying any particular container), that will again provision two machines:

```text
$ juju deploy postgresql --to lxd
Located charm "postgresql" in charm-hub, revision 288
Deploying "postgresql" from charm-hub charm "postgresql", revision 288 in channel 14/stable on ubuntu@22.04/stable
ubuntu@charm-dev:~/.local/share/juju$ juju machines
Machine  State    Address         Inst id              Base          AZ  Message
0        started  10.154.118.110  juju-dadfb7-0        ubuntu@22.04      Running
1        started  10.154.118.72   juju-dadfb7-1        ubuntu@22.04      Running
1/lxd/0  pending                  juju-dadfb7-1-lxd-0  ubuntu@22.04      Container started
2        started  10.154.118.209  juju-dadfb7-2        ubuntu@22.04      Running
2/lxd/0  pending                  pending              ubuntu@22.04      acquiring LXD image
```

In Juju, they are both essentially the same -- 'machines'.  For example, most `juju` CLI commands that target machines can actually target system containers in the exact same way.

(manual-machine)=
### Manual machine

A **manual machine** is a machine the user provisioned themselves:
`juju add-machine ssh:user@host` reaches an existing host over SSH
instead of asking the cloud for one. The machine's records keep the
manual flag (see the persistence layer below), and its instance status
reads `Manually provisioned machine` once the agent reports in (see
{ref}`Machine states <machine-states>`).

(controller-machine)=
### Controller machine

The **controller machine** is the machine that hosts the controller
application -- Juju derives it, not stores it: a machine is the
controller machine when a unit of the controller's application runs on
it. It is the machine the controller agent runs on, and it hosts every
model worker, the compute provisioner included (see
{ref}`the controller agent <controller-agent>`).

(the-machines-records)=
(the-machine-record)=
(the-machine-in-the-data-model)=
(machine-states)=
(machine-agent-status)=
(instance-status)=
(machine-base)=
## Machines in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Machine attributes
:alt: The machine's stored tables as an entity-relationship slice: the machine record at the centre with its name, life, net-node pointer and teardown flags; the parent table naming its child and its host west with the net node below it; the agent status record east; the cloud instance below with its own status under it. Each line is a stored pointer; 1/m at each end; nothing dashed -- every drawn pointer is mandatory (the instance id is a nullable field: empty until the cloud reports it).
:caption: Entity relationship diagram: The machine's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The machine shares its net node with the units running on it; the parent record is two pointers into the same table (the single nesting level a container may have); the two status records -- the agent's and the instance's -- hang off the machine and the instance. The base (os@channel + architecture), the manual satellite row, the placement, constraints, storage attachments, agent version, SSH host keys and LXD profiles are per-machine records the slice does not open; the lookup tables behind the status vocabularies are not drawn.
```

In the model database a machine is a **native record** (Juju's own,
not a cloud fact; the DDL: `0018-machine.sql`, its cloud instance in
`0017-machine-cloud-instance.sql`): the machine row carries the
designation the client types (`name`, unique per model --
`machine already exists`), its life, the network identity it shares
with the units running on it (`net_node_uuid`, drawn as the name-only
chip -- the addresses hang off the net node, which the machine and its
units share), the time its agent started, its hostname, and the
teardown flags (`keep_instance` on the row; the manual flag is the
`machine_manual` satellite row's presence). The `machine_platform`
satellite stores the base -- the `os@channel` way to identify the OS
image (`ubuntu@22.04`, `ubuntu@22.04/stable`), Juju 3.1.0's
replacement for the older notion of 'series' -- plus the
architecture. `machine_parent` names a machine's host -- the single
nesting level a container may have, composite in effect: the child row
points up, the host row is the same machine table.
`machine_cloud_instance` tracks the cloud instance once the provider
reports it: the instance ID (empty until the cloud has actually
created the instance -- an honest absence the nullable column states),
its hardware (arch, CPU, memory, root disk, `virt_type`) and its
availability zone. The writes are the machine service's:
`AddMachine` inserts the machine row and its satellites;
`SetMachineCloudInstance` stores the instance once the provider
reports it; the removal service carries the teardown (see
{ref}`Machine removal <machine-removal>`).

The identity pair: the primary key (`machine.uuid`) is the join
handle -- it exists so the parent record, the instance and the two
status records have something to point at (the units running on the
machine point at the net node, not the machine: the shared identity is
what "runs on" means in the data model). The natural key the client
names is `machine.name` (UNIQUE per model): `0`, `1/lxd/0` -- the
designation (see {ref}`Machine designations <machine-designations>`).

Every foreign key is an assertion the record holds: the machine row
holds the net-node pointer (UNIQUE -- one machine per network
identity); the parent record holds two pointers into the same table
(`machine_parent.machine_uuid` the child's, `parent_uuid` the host's
-- drawn west); the instance row holds the machine pointer (its
primary key is the pointer; a pending machine has no instance row yet
-- the row itself may be absent) and, undrawn, its own life pointer
and a nullable zone pointer (a fact about the machine's
{ref}`zone <zone>`, known only once the provider reports it); the two
status records hold the machine pointer and the instance pointer
respectively (their primary keys are the pointers). Not drawn above,
all assertions the schema states: the manual satellite (`machine_manual`
-- the row's presence is the manual flag), the placement
(`machine_placement`), the machine's {ref}`constraints <constraint>`
(`machine_constraint`), its {ref}`storage <storage>` attachments
(`machine_volume`, `machine_filesystem`), the reported agent version,
the SSH host keys, the LXD profiles, and the requires-reboot flag.

The machine table has no type column: a machine's kind is derived,
not stored -- the parent pointer is the container fact, the
`machine_manual` row the manual fact, and the controller machine is
derived in the schema itself (`v_machine_is_controller`: a machine
whose net node runs a unit of the controller's application). A machine
that is none of these is a regular machine: a cloud instance the
controller's compute provisioner started. The kinds are how you add
machines ({ref}`LXD container <machine-lxd-container>`, {ref}`manual
machine <manual-machine>`, {ref}`controller machine
<controller-machine>`), not how they are stored.

Three projections with writers of their own:

- **life** -- the shared alive / dying / dead cycle every entity has,
  flattened onto the machine's and the instance's `life_id` fields. A
  machine is created alive; the removal machinery marks it dying
  (guarded one-way) together with its cloud instance record and --
  unless the removal is forced past them -- its child containers,
  which die with their host; the machine's own agent asks the
  controller to mark it dead once nothing is left on it, and a
  scheduled removal job then deletes the records (see {ref}`Machine
  removal <machine-removal>`).
- **`machine_status`** -- the machine agent's report about the Juju
  agent on the machine: `pending` (created, agent not yet started),
  `started`, `stopped` (the agent noticed the machine's life is no
  longer alive), `error` (with a message -- the teardown request
  failed). `down` is written by no one: it is what the status reads as
  when the agent has not been seen recently -- the presence rule the
  status domain applies on read. The agent's transitions as it shuts
  its machine down:

  ```{ggarch}
  :file: ../juju.ggarch
  :view: Machine agent status
  :no-legend:
  :caption: State machine diagram: The machine agent's status as it shuts its machine down -- the agent reports started at startup; when the life watcher fires (the machine is no longer alive) it reports stopped and asks the controller to have the machine marked dead, waiting until the units and storage assigned to it clear; a failed request parks the status in error.
  :alt: State machine: pending to started on machine agent startup; started to stopped when the life watcher fires; started to error when EnsureDead fails with units or storage still assigned; stopped internally waits for units and storage to clear, then dies.
  ```

- **`machine_cloud_instance_status`** -- the provisioning lifecycle of
  the cloud instance, written controller-side: `pending` (record
  created, instance not yet asked for), `allocating` ('starting')
  while the compute provisioner asks the cloud for the instance,
  `running` once the instance ID and addresses are registered, and
  `provisioning error` when the cloud broker fails. A transient error
  is retried -- the provisioner re-processes the machines marked
  transient and asks again -- back into `allocating`; in steady state
  the instance poller mirrors the provider-reported status (the
  vocabulary also carries `unknown`, the row's zero value). The
  provisioner's transitions:

  ```{ggarch}
  :file: ../juju.ggarch
  :view: Machine provisioning
  :no-legend:
  :caption: State machine diagram: The cloud instance's provisioning status -- the controller's compute provisioner moves a pending machine to allocating ('starting') when it asks the cloud for the instance, to running once the instance and its addresses are registered, and to provisioning error when the broker fails; a transient error is retried back into allocating. In steady state the instance poller mirrors the provider-reported status.
  :alt: State machine: pending to allocating on the compute provisioner starting the instance; allocating to running when instance and addresses are recorded; allocating to provisioning error on broker error; provisioning error back to allocating on a transient retry; running mirrors the provider status.
  ```

None of these is transition-validated: every status write is a
membership check (the value must be known) followed by an upsert --
what constrains a machine is who writes which value, not a transition
matrix. The vocabularies live in lookup tables (`machine_status_value`,
`machine_cloud_instance_status_value`, `life`).

(the-machines-machinery)=
## Machines in the execution layer

By the time the command returns, the machine record exists -- and the
cloud instance may still be allocating: the compute provisioner has
only just asked the cloud for it, and the instance ID and addresses
land as they become known.

A machine has machinery of its own: in the controller, the compute
provisioner and the instance poller drive its cloud instances; on the
machine itself, the machine agent -- a {ref}`Juju agent <agent>` --
hosts the unit agents, provisions the machine's containers, and
shuts the machine down.

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: Topology: The two provisioning paths -- the controller's compute provisioner starts base machines and records the instance ID and addresses as they become known; a host machine's agent provisions its own containers through the LXD broker and watches them via the controller API. What a designation names: machine 0 and its LXD container are rows in the same machine table, the container linked to its host by a machine-parent record. Containers are machines: each runs its own machine agent, which hosts the unit agent.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

(machines-and-units)=
### Machine operations

Operations on machines split by who owns them: the controller starts
and removes base machines; a host machine's agent provisions its own
containers; the user can bring a machine in over SSH.

#### Machine creation

Most machines are created implicitly: deploying an application or
adding a unit on a machine cloud asks Juju for compute, and the
placement -- an existing machine, a new one, or a container on either
(see {ref}`Machine designations <machine-designations>` and
{ref}`placement directive <placement-directive>`) -- is resolved when
the unit's machine record is written. Explicit creation goes through
the add-machine operation (for example, `juju add-machine`): the
controller's machine manager writes the machine record, and the
compute provisioner takes it from there.

(machine-provisioning)=
#### Machine provisioning
```{audience} juju-dev
```

The controller's compute provisioner is a model worker that watches
the model's unprovisioned machines: for each pending machine it asks
the cloud to start an instance (the instance status machine, {ref}`Instance
status <instance-status>`), records the instance ID
and addresses as they become known, and retries transient failures.
Base machines are provisioned this way by the controller; containers
are provisioned by their host machine's agent through the LXD broker,
which learns about its containers from the controller's API (the two
paths are the {ref}`Machine designations <machine-designations>`
view).

(machine-removal)=
#### Machine removal

Removal is initiated through the remove-machine operation (for
example, `juju remove-machine 0`). Removal follows the same
cooperative pattern as every entity removal: the machine (its
containers included, unless the removal is forced past them) is
marked dying, the machine agent notices through its life watcher,
stops, and asks the controller to have the machine marked dead once
no units or storage are assigned to it any more; a scheduled removal
job then deletes the records. The `keep-instance` flag decides
whether the cloud instance is released with the machine.

(manual-provisioning)=
#### Manual provisioning

The add-machine operation over SSH (for example, `juju add-machine
ssh:user@host`) provisions nothing in the cloud: the controller
renders the provisioning script, the user's SSH session runs it on
the target host, and the host's machine agent reports in -- the
machine's record keeps the manual flag (see {ref}`Manual machine
<manual-machine>`).

(machine-watchers)=
### Machine watchers
```{audience} juju-dev
```

Nothing about a machine is polled by the things that act on it: they
watch it. The machine domain's watchable service exposes these watch
surfaces:

- **One machine's life and dependants** -- notifies on changes to the
  machine's life and to the units, containers, and storage entities
  tied to it. This is the machine agent's shutdown watcher: without
  it, the agent would never correctly shut its machine down.
- **A parent machine's containers' life** -- notifies the host
  machine's agent of changes to the life of its containers; this is
  what drives the container provisioner.
- **The model's (non-container) machines** -- notifies the
  controller's compute provisioner (and the instance poller) of the
  machines it must provision.
- **The model machines' life and start times** -- the instance
  poller's surface, for the steady-state status mirroring.
- **The model's machine cloud instances** and **a machine's reboot
  state** -- the instance poller's and the reboot machinery's
  surfaces.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(machine-rules-and-errors)=
## Machine rules and errors

The machine domain encodes its rules as a typed error taxonomy; each
error names the rule it enforces. The rules matter to Juju users
provisioning machines and to Juju developers, who maintain them as
the domain's validation law.

The rules a **machine designation** must satisfy (see
{ref}`Machine designations <machine-designations>`):

- numbers are `0` or positive integers without leading zeros;
- at most one level of container nesting (`0/lxd/4`, never
  `0/lxd/0/lxd/0` -- grandparent machines are not supported);
- designations are comma-separated lists of the shapes above;
- the `--to` argument is rejected on Kubernetes models -- containers
  are a machine-cloud concept.

The rules a **machine mutation** must satisfy:

- a container's host must exist and be the machine named by the
  designation (`lxd:25` places the container on machine 25);
- a machine can only be declared dead once nothing is assigned to it
  -- units, containers, or storage keep it alive (see {ref}`Machine
  removal <machine-removal>`);
- container types must be supported (`invalid container type`);
- machine constraints must be satisfiable (`invalid machine
  constraints`, `machine constraint violation`).

The errors that encode them:

- *Existence and life*: `machine not found`, `machine not alive`,
  `machine is dead`, `machine not provisioned` (the machine has no
  instance yet -- you cannot, say, scp to it).
- *Structure*: `grandparent machine are not supported currently`,
  `machine has no parent`, `machine already exists`,
  `machine cloud instance already exists`, `invalid container type`.
- *Provisioning*: `machine not provisioned`, `invalid machine
  constraints`, `machine constraint violation`.
