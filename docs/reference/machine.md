---
myst:
  html_meta:
    description: "Juju machine reference: compute resources on bare metal, VMs, and LXD containers. The machine record, machine kinds, machine states, provisioning, and machine watchers."
---

(machine)=
# Machine

```{ibnote}
See also: {ref}`manage-machines`
```

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` requested implicitly (e.g., through {ref}`command-juju-deploy`, {ref}`command-juju-add-unit`, etc.) or explicitly (e.g., through {ref}`command-juju-add-machine`) from a machine {ref}`cloud <cloud>`.

```{important}

Juju provisions more than bare instances: a LXD container on a regular cloud instance is also, from the point of view of Juju, a 'machine'. Everything in this document applies to both.

From the point of view of an end user this is true with one small caveat -- even though listed in `juju` outputs under 'Machines', and in general handled via the same CLI commands as a machine, a LXD container provisioned on top of a regular cloud instance will be named after its host machine; e.g., `0/lxd/5` = LXD container `5` on machine `0`.

```

(the-machine-record)=
## The machine record

A machine is a record in the model database. It is identified by its
**machine ID** -- a unique name such as `0` or `1/lxd/0` (see
{ref}`Machine designations <machine-designations>`) -- and it carries
the machine's life, its own network identity (a net node, shared with
the {ref}`units <unit>` that run on the machine), the time its agent
started, its hostname, and the flags that steer its teardown (whether
the cloud instance should be kept when the machine is removed, and
whether it was provisioned manually).

(machine-base)=
### Machine base

```{versionadded} 3.1.0
```

In Juju, a **base** is  a way to identify a particular operating system (OS) image for a Juju {ref}`machine <machine>`.

This can be done via the name of the OS followed by the `@` symbol and the channel of the OS that you want to target, specified in terms of `<track>` or, optionally, `<track>/<risk>`. For example, `ubuntu@22.04` or `ubuntu@22.04/stable`.

A 'base' replaces the older notion of 'series'.

### Machine designations

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: Topology: What a machine designation names: machine 0 and its LXD container are rows in the same machine table, the container linked to its host by a machine-parent record -- the designation is that containment path. Two provisioning paths: the controller (its compute provisioner) starts base machines; the host machine's agent provisions its own containers through the LXD broker and watches them via the API. Containers are machines: each runs its own machine agent, which hosts the unit agent.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

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

These designations are for machine clouds only: the `--to` argument is rejected on Kubernetes models.

(types-of-machine)=
## Types of machine

A machine's kind is not stored: the machine table has no type column.
The kind is derived from three facts in the data model -- whether the
machine has a parent ({ref}`LXD container <machine-lxd-container>`),
whether it was provisioned by the user rather than the cloud
({ref}`manual machine <manual-machine>`), and whether it hosts the
controller ({ref}`controller machine <controller-machine>`). A machine
that is none of these is a regular machine: a cloud instance the
controller's compute provisioner started.

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
instead of asking the cloud for one. The machine's record carries the
manual flag, and its instance status reads `Manually provisioned
machine` once the agent reports in (see {ref}`Machine states
<machine-states>`).

(controller-machine)=
### Controller machine

The **controller machine** is the machine that hosts the controller
application -- Juju derives it, not stores it: a machine is the
controller machine when a unit of the controller's application runs on
it. It is the machine the controller agent runs on, and it hosts every
model worker, the compute provisioner included (see
{ref}`the controller agent <controller-agent>`).

(the-machine-in-the-data-model)=
## The machine in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Machine attributes
:alt: The machine's stored tables as an entity-relationship slice: the machine record at the centre; the parent table naming its child and its host; the status record and the net node beside it; the cloud instance below with its own status under it. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The machine's stored records and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The machine's container type, manual flag, life, base (os@channel + architecture) and the instance's availability zone are stored as fields on their records; the lookup tables behind them are not drawn.
```

The machine's state is spread across a handful of stored tables. The
`machine` record carries the machine ID, its life, its net node, the
agent start time, the hostname, and the teardown flags (keep instance,
manual). The `machine_parent` record names a machine's host -- this is
the single nesting level a container may have -- and
`machine_cloud_instance` tracks the cloud instance: the instance ID
(empty until the cloud provider has actually created the instance),
its hardware (arch, CPU, memory, root disk, `virt_type`), and its
availability zone.

Each of the two carries its own status record: `machine_status` (the
machine agent's status: pending, started, stopped, down, error) and
`machine_cloud_instance_status` (the instance's provisioning status:
unknown, pending, allocating, running, provisioning error). Both keep
a message, the update time, and the status vocabulary as a lookup
table.

The rest of the machine's attributes live in per-machine satellite
records: the base (`machine_platform`: os, channel, architecture), the
placement (`machine_placement`: scope + directive), the machine's
{ref}`constraints <constraint>` (`machine_constraint`), its
{ref}`storage <storage>` attachments (`machine_volume`,
`machine_filesystem`), its reported agent version, its SSH host keys,
its LXD profiles, and its addresses -- which hang off the machine's
net node, the same network identity the units running on the machine
share.

(machine-states)=
## Machine states

A machine carries orthogonal state machines: its **life** -- the
shared alive / dying / dead cycle every entity has, kept for both the
machine and its cloud instance -- and two status vocabularies, the
machine agent's own status and the cloud instance's provisioning
status. Unlike the relation's status, none of these is transition-
validated: every status write is a membership check (the value must
be known) followed by an upsert -- what constrains a machine is who
writes which value, not a transition matrix.

### Life

A machine is created alive. The removal machinery marks it dying
(guarded one-way) when the machine is removed, together with the
machine's cloud instance record and -- unless the removal is forced
past them -- the machine's child containers, which die with their
host. The machine is declared dead only once nothing is left on it:
the machine's own agent calls the controller to have it marked dead
when it notices its life is no longer alive and no units or storage
are assigned to it any more; a scheduled removal job then deletes the
records.

(machine-agent-status)=
### Machine status

```{ggarch}
:file: ../juju.ggarch
:view: Machine agent status
:no-legend:
:caption: State machine diagram: The machine agent's status as it shuts its machine down -- the agent reports started at startup; when the life watcher fires (the machine is no longer alive) it reports stopped and asks the controller to have the machine marked dead, waiting until the units and storage assigned to it clear; a failed request parks the status in error.
:alt: State machine: pending to started on machine agent startup; started to stopped when the life watcher fires; started to error when EnsureDead fails with units or storage still assigned; stopped internally waits for units and storage to clear, then dies.
```

The machine's own status is the machine agent's report about the
Juju agent running on the machine: `pending` (created, agent not yet
started), `started`, `stopped` (the agent noticed the machine's life
is no longer alive), and `error` (with a message -- the teardown
request failed). `down` is not written by anyone: it is what a
machine's status reads as when its agent has not been seen recently
-- the presence rule the status domain applies on read.

(instance-status)=
### Instance status

```{ggarch}
:file: ../juju.ggarch
:view: Machine provisioning
:no-legend:
:caption: State machine diagram: The cloud instance's provisioning status -- the controller's compute provisioner moves a pending machine to allocating ('starting') when it asks the cloud for the instance, to running once the instance and its addresses are registered, and to provisioning error when the broker fails; a transient error is retried back into allocating. In steady state the instance poller mirrors the provider-reported status.
:alt: State machine: pending to allocating on the compute provisioner starting the instance; allocating to running when instance and addresses are recorded; allocating to provisioning error on broker error; provisioning error back to allocating on a transient retry; running mirrors the provider status.
```

The instance status is the provisioning lifecycle of the machine's
cloud instance, written by the controller side: `pending` while the
machine record waits to be provisioned, `allocating` ('starting')
while the compute provisioner asks the cloud to start the instance,
`running` once the instance ID and addresses are registered, and
`provisioning error` when the cloud broker fails. A transient
provisioning error is retried -- the provisioner resets the machine
to pending and tries again -- and in steady state the instance poller
keeps the status in step with what the cloud provider reports.

(machines-and-units)=
## Machine operations

Operations on machines split by who owns them: the controller starts
and removes base machines; a host machine's agent provisions its own
containers; the user can bring a machine in over SSH.

### Machine creation

Most machines are created implicitly: deploying an application or
adding a unit on a machine cloud asks Juju for compute, and the
placement -- an existing machine, a new one, or a container on either
(see {ref}`Machine designations <machine-designations>` and
{ref}`placement directive <placement-directive>`) -- is resolved when
the unit's machine record is written. Explicit creation goes through
the add-machine operation (for example, `juju add-machine`): the
controller's machine manager writes the machine record, and the
compute provisioner takes it from there.

### Machine provisioning

The controller's compute provisioner is a model worker that watches
the model's unprovisioned machines: for each pending machine it asks
the cloud to start an instance (the instance status machine in
{ref}`Instance status <instance-status>`), records the instance ID
and addresses as they become known, and retries transient failures.
Base machines are provisioned this way by the controller; containers
are provisioned by their host machine's agent through the LXD broker,
which learns about its containers from the controller's API (the two
paths are the {ref}`Machine designations <machine-designations>`
view).

(machine-removal)=
### Machine removal

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
### Manual provisioning

The add-machine operation over SSH (for example, `juju add-machine
ssh:user@host`) provisions nothing in the cloud: the controller
renders the provisioning script, the user's SSH session runs it on
the target host, and the host's machine agent reports in -- the
machine's record keeps the manual flag (see {ref}`Manual machine
<manual-machine>`).

## Machine watchers

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

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

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

(machines-and-units)=
(related-entities-machine)=
## Related entities

- **Units** run on machines: there is usually one unit per machine,
  but several units of the same or of different applications can share
  one (see {ref}`unit <unit>`). The unit and the machine share the
  machine's net node -- that shared network identity is how Juju
  knows a unit "runs on" its machine.
- **The base** identifies the OS image a machine runs (os@channel +
  architecture, stored with the machine; see {ref}`Machine base
  <machine-base>`).
- **Constraints** customise a machine's hardware (see
  {ref}`constraint <constraint>`); they are stored with the machine
  and honoured at provisioning time.
- **Storage** attaches to machines through their volumes and
  filesystems (see {ref}`storage <storage>`).
- **Status** owns the machine's two status vocabularies and the
  presence rule that reads a silent agent as `down` (see
  {ref}`Machine states <machine-states>`).
- **Removal** owns the teardown that takes a dying machine to dead,
  its containers dying with their host (see {ref}`Machine removal
  <machine-removal>`).
- **The placement directive** is how a request names the machine a
  unit should land on (see {ref}`placement directive
  <placement-directive>`).
- **Availability zones** are a fact about the machine's cloud
  instance, recorded on the instance record (see {ref}`zone
  <zone>`).
