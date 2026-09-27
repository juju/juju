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

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` requested implicitly (e.g., through {ref}`command-juju-deploy`, {ref}`command-juju-add-unit`, etc.) or explicitly (e.g., through {ref}`command-juju-add-machine`) from a machine {ref}`cloud <cloud>`. Juju provisions more than bare instances: a LXD container on a regular cloud instance is also, from the point of view of Juju, a 'machine'. Everything in this document applies to both.

```{important}

Even though a LXD container is listed in `juju` outputs under 'Machines', and handled via the same CLI commands as a machine, it is named after its host machine; e.g., `0/lxd/5` = LXD container `5` on machine `0`.

```

## Machines in the declaration layer

The declaration layer handles how clients express compute intent. You add a machine explicitly with `juju add-machine` and remove it with `juju remove-machine` (model {ref}`write access <user-access-model-write>` -- the Machinemanager facade gates both the add and the remove on model write); most machines are never declared on their own: they are requested implicitly, when a deploy or add-unit placement asks for compute. The controller-side counterpart is the same call over the controller API.

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

(machine-lxd-container)=
### LXD container

An **LXD container** is a machine whose record is linked to another
machine's by a machine-parent record. Each machine can have a single
parent and be the parent of many children, and only one level of
nesting is supported -- a container cannot have a container.

The container is provisioned by its host machine's agent, not by the
controller -- which is why adding a container brings its host machine
in as a machine of its own. For example, `juju add-machine lxd` starts
a LXD container on a new machine and adds *both* as 'machines':

```text
$ juju add-machine lxd
created container 1/lxd/0

$ juju machines
Machine  State    Address         Inst id        Base          AZ  Message
0        started  10.154.118.110  juju-dadfb7-0  ubuntu@22.04      Running
1        pending                  pending        ubuntu@22.04      Creating container
1/lxd/0  pending                  pending        ubuntu@22.04
```

In Juju they are both essentially the same -- 'machines': most `juju`
CLI commands that target machines can target system containers in
exactly the same way.

(manual-machine)=
### Manual machine

A **manual machine** is a machine the user provisioned themselves:
`juju add-machine ssh:user@host` reaches an existing host over SSH
instead of asking the cloud for one. The machine's records keep the
manual flag (see the persistence layer below), and its instance status
reads `Manually provisioned machine` once the agent reports in (see
{ref}`Instance status <instance-status>`).

(controller-machine)=
### Controller machine

The **controller machine** is the machine that hosts the controller
application -- Juju derives it, not stores it: a machine is the
controller machine when a unit of the controller's application runs on
it. It is the machine the controller agent runs on, and it hosts every
model worker, the compute provisioner included (see
{ref}`the controller agent <controller-agent>`).

(machine-declaration-rules)=
### Declaration rules and errors

The rules a machine declaration must satisfy (see {ref}`Machine
designations <machine-designations>`):

- numbers are `0` or positive integers without leading zeros;
- at most one level of container nesting (`0/lxd/4`, never
  `0/lxd/0/lxd/0` -- grandparent machines are not supported);
- designations are comma-separated lists of the shapes above;
- the `--to` argument is rejected on Kubernetes models -- containers
  are a machine-cloud concept.

The errors the declaration layer raises when a rule is rejected:

- `machine already exists` (the designation's name is taken);
- `invalid container type`;
- `grandparent machine are not supported currently`;
- `invalid machine constraints` (the constraint payload names a space
  or a container type that does not exist).

(the-machine-in-the-data-model)=
## Machines in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Machine attributes
:alt: The machine's stored tables as an entity-relationship slice: the machine record at the centre with its name, life, net-node pointer and teardown flags; the parent table naming its child and its host west with the net node below it; the agent status record east; the cloud instance below with its own status under it. Each line is a stored pointer; 1/m at each end; nothing dashed -- every drawn pointer is mandatory (the instance id is a nullable field: empty until the cloud reports it).
:caption: Entity relationship diagram: The machine's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The machine shares its net node with the units running on it; the parent record is two pointers into the same table (the single nesting level a container may have); the two status records -- the agent's and the instance's -- hang off the machine and the instance. The base (os@channel + architecture), the manual satellite row, the placement, constraints, storage attachments, agent version, SSH host keys and LXD profiles are per-machine records the slice does not open; the lookup tables behind the status vocabularies are not drawn.
```

In the model database a machine is a **native record** (Juju's own,
not a cloud fact; the DDL: `0018-machine.sql`, its cloud instance in
`0017-machine-cloud-instance.sql`). Rather than managing complex
operational states on a single monolithic table, the model database
distributes the machine's story across a decoupled set of records:

- **The dual identity (`machine`)**: a unique, hidden ID (`uuid`) acts
  as the join handle that the parent record, the instance record and
  the two status records point at; the natural key (`name`) is the
  unique, user-facing designation (`0`, `1/lxd/0`), enforced per model
  -- declare a second machine with the same name and the controller
  rejects it (`machine already exists`).
- **The network anchor (`net_node_uuid`)**: a pointer to the network
  identity whose addresses the machine hangs off -- UNIQUE, one
  machine per network identity. The units running on the machine point
  at the *same* net node: sharing it is what "runs on" means in the
  data model.
- **Lifecycle and teardown (`life_id`, `keep_instance`)**: the shared
  alive / dying / dead cycle. Removal marks the machine dying
  (dropping its child containers with it, unless forced past them) and
  its instance record with it; the machine's own agent asks the
  controller to mark it dead once nothing is left on it; a scheduled
  removal job then deletes the records (see {ref}`Machine removal
  <machine-removal>`). The `keep_instance` flag decides whether the
  cloud instance is released with the machine.
- **The configuration satellites (`machine_platform`,
  `machine_manual`, `machine_constraint`)**: three optional per-machine
  rows -- the OS base (`os@channel` + architecture, e.g.
  `ubuntu@22.04`), the
  manual flag (the `machine_manual` row's mere presence is the flag),
  and the compute constraints (a pointer into the {ref}`constraint
  <constraint>` table).
- **Containers (`machine_parent`)**: two pointers into the same
  machine table -- the child's and its host's -- establishing the
  parent-child relation, capped at a single nesting level.
- **The cloud instance (`machine_cloud_instance`)**: one row, born
  with the machine -- the record exists to track the instance-creation
  process. Its `instance_id` is a nullable field, empty until the
  cloud has actually created the instance (read as *not provisioned*
  until then); once the provider reports, the row also stores the
  hardware (arch, CPU, memory, root disk, `virt_type`) and the
  availability zone (a fact about the machine's {ref}`zone <zone>`,
  known only once reported). The row carries its own life pointer.
- **The status projections (`machine_status`,
  `machine_cloud_instance_status`)**: two upsert tables -- every write
  is a membership check (the value must be known) followed by an
  upsert; nothing is transition-validated, what constrains a machine
  is *who* writes which value. `machine_status` is the machine agent's
  report about the Juju agent on the machine: `pending` (created,
  agent not yet started), `started`, `stopped` (the agent noticed the
  machine's life is no longer alive), `error` (with a message -- the
  teardown request failed). `down` is written by no one: it is what
  the status reads as when the agent has not been seen recently -- the
  presence rule the status domain applies on read.
  `machine_cloud_instance_status` is written controller-side, the
  provisioning lifecycle of the cloud instance: `pending` (instance
  not yet asked for), `allocating` ('starting') while the compute
  provisioner asks the cloud, `running` once the instance ID and
  addresses are registered, `provisioning error` when the cloud broker
  fails, and `unknown` (the row's zero value). The vocabularies live
  in lookup tables (`machine_status_value`,
  `machine_cloud_instance_status_value`, `life`).

The writes are the machine service's: `AddMachine` inserts the machine
row and its satellites; `SetMachineCloudInstance` fills the instance
row once the provider reports it; the removal service carries the
teardown (see {ref}`Machine removal <machine-removal>`).

The machine table has no type column: a machine's kind is derived, not
stored -- the parent pointer is the container fact, the
`machine_manual` row the manual fact, and the controller machine is
derived in the schema itself (`v_machine_is_controller`: a machine
whose net node runs a unit of the controller's application). A machine
that is none of these is a regular machine: a cloud instance the
controller's compute provisioner started. The kinds are how you add
machines ({ref}`LXD container <machine-lxd-container>`, {ref}`manual
machine <manual-machine>`, {ref}`controller machine
<controller-machine>`), not how they are stored.

(machine-persistence-rules)=
### Persistence rules and errors

The rules the stored records enforce:

- a machine can only be declared dead once nothing is assigned to it
  -- units, containers, or storage keep it alive (see {ref}`Machine
  removal <machine-removal>`).

The errors the stored state raises:

- *existence and life*: `machine not found`, `machine not alive`,
  `machine is dead`;
- *provisioning*: `machine not provisioned` (the machine's instance
  record holds no instance ID yet -- you cannot, say, scp to it),
  `machine cloud instance already exists` (the record already holds an
  instance);
- *structure*: `machine has no parent` (the operation needed the
  parent record; there is none).

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

(machine-agent-status)=
### Machine agent status

The machine agent writes its own status; its loop as it shuts its
machine down:

```{ggarch}
:file: ../juju.ggarch
:view: Machine agent status
:no-legend:
:caption: State machine diagram: The machine agent's status as it shuts its machine down -- the agent reports started at startup; when the life watcher fires (the machine is no longer alive) it reports stopped and asks the controller to have the machine marked dead, waiting until the units and storage assigned to it clear; a failed request parks the status in error.
:alt: State machine: pending to started on machine agent startup; started to stopped when the life watcher fires; started to error when EnsureDead fails with units or storage still assigned; stopped internally waits for units and storage to clear, then dies.
```

(instance-status)=
### Instance status

The controller writes the instance's provisioning status; the
provisioner's transitions:

```{ggarch}
:file: ../juju.ggarch
:view: Machine provisioning
:no-legend:
:caption: State machine diagram: The cloud instance's provisioning status -- the controller's compute provisioner moves a pending machine to allocating ('starting') when it asks the cloud for the instance, to running once the instance and its addresses are registered, and to provisioning error when the broker fails; a transient error is retried back into allocating. In steady state the instance poller mirrors the provider-reported status.
:alt: State machine: pending to allocating on the compute provisioner starting the instance; allocating to running when instance and addresses are recorded; allocating to provisioning error on broker error; provisioning error back to allocating on a transient retry; running mirrors the provider status.
```

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

(machine-execution-rules)=
### Execution rules and errors

The rules the running system enforces:

- a machine's reported hardware must satisfy the constraints asked of
  it -- when a placement is resolved, the machine's instance record is
  compared against the constraints and the placement fails with
  `machine constraint violation` otherwise;
- a failing cloud broker parks the instance in `provisioning error`;
  a transient failure is retried back into `allocating` (see
  {ref}`Instance status <instance-status>`).
