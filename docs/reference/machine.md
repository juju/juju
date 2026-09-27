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

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` requested implicitly by a {ref}`deploy <command-juju-deploy>` or {ref}`add-unit <command-juju-add-unit>` placement, or explicitly with {ref}`add-machine <command-juju-add-machine>`, from a machine {ref}`cloud <cloud>`. A LXD container on a cloud instance is also a machine. Everything in this document applies to both.

```{important}

Even though a LXD container is listed in `juju` outputs under 'Machines', and handled via the same CLI commands as a machine, it is named after its host machine; e.g., `0/lxd/5` = LXD container `5` on machine `0`.

```

## Machines in the declaration layer

- **Explicit creation:** `juju add-machine` and `juju remove-machine`, gated on model {ref}`write access <user-access-model-write>`.
- **Implicit creation:** most machines are never declared on their own. A deploy or add-unit placement asks for compute, and the machine record follows.

```{ibnote}
See also: {ref}`tfjuju:manage-machines <tfjuju:manage-machines>`
```

(machine-designations)=
### Machine designations

Many commands take a machine argument. The shapes below combine in comma-separated lists:

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

Designations are for machine clouds only: the `--to` argument is rejected on Kubernetes models.

(machine-lxd-container)=
### LXD container

A **LXD container** is a machine linked to a host machine by a machine-parent record. The host's agent provisions its own containers, not the controller, so adding a container brings the host machine in as a machine of its own:

```text
$ juju add-machine lxd
created container 1/lxd/0

$ juju machines
Machine  State    Address         Inst id        Base          AZ  Message
0        started  10.154.118.110  juju-dadfb7-0  ubuntu@22.04      Running
1        pending                  pending        ubuntu@22.04      Creating container
1/lxd/0  pending                  pending        ubuntu@22.04
```

Most `juju` CLI commands that target machines target containers the same way.

(manual-machine)=
### Manual machine

A **manual machine** is a machine the user provisioned themselves: `juju add-machine ssh:user@host` reaches an existing host over SSH instead of asking the cloud for one. The records keep the manual flag, and the instance status reads `Manually provisioned machine` once the agent reports in. See {ref}`Instance status <instance-status>`.

(controller-machine)=
### Controller machine

The **controller machine** hosts the controller application. It is derived, not stored: a machine is the controller machine when a unit of the controller's application runs on it. It runs the {ref}`controller agent <controller-agent>` and hosts every model worker, the compute provisioner included.

(machine-declaration-rules)=
### Declaration rules and errors

Rules:

- numbers are `0` or positive integers without leading zeros;
- at most one level of container nesting (`0/lxd/4`, never `0/lxd/0/lxd/0`);
- designations are comma-separated lists of the shapes above;
- the `--to` argument is rejected on Kubernetes models.

Errors:

- `machine already exists`;
- `invalid container type`;
- `grandparent machine are not supported currently`;
- `invalid machine constraints`: the payload names a space or a container type that does not exist.

(the-machine-in-the-data-model)=
## Machines in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Machine attributes
:alt: The machine's stored tables as an entity-relationship slice: the machine record at the centre with its name, life, net-node pointer and teardown flags; the parent table naming its child and its host west with the net node below it; the agent status record east; the cloud instance below with its own status under it. Each line is a stored pointer, 1/m at each end; nothing dashed; every drawn pointer is mandatory; the instance id is a nullable field, empty until the cloud reports it.
:caption: Entity relationship diagram: The machine's stored records and the schema associations between them. Each line starts at the fk column that holds the pointer, 1/m at each end; dashed means the row may be absent. The machine shares its net node with the units running on it; the parent record is two pointers into the same table, the single nesting level a container may have; the agent status record hangs off the machine and the instance status record off the instance. Not drawn: the base, the manual satellite row, the placement, constraints, storage attachments, agent version, SSH host keys, and LXD profiles, plus the lookup tables behind the status vocabularies.
```

In the model database, a machine is a native record (`0018-machine.sql`) distributed across a decoupled set of tables:

- **`machine` (dual identity):** an internal `uuid` join handle for relations alongside a model-scoped natural key (`name`) for user designations (`0`, `1/lxd/0`), strictly enforced to reject duplicates (`machine already exists`).
- **`net_node_uuid` (network anchor):** a unique pointer to the network identity whose addresses the machine hangs off. The units running on the machine point at the *same* net node: sharing it is what "runs on" means in the data model.
- **`life_id`, `keep_instance` (lifecycle and teardown):** the shared alive / dying / dead cycle. Removal marks the machine dying, then dead once nothing is left on it, and deletes the records on a schedule. `keep_instance` decides whether the cloud instance is released with the machine. See {ref}`Machine removal <machine-removal>`.
- **`machine_platform`, `machine_manual`, `machine_constraint` (configuration satellites):** three optional per-machine rows: the OS base as `os@channel` plus architecture, the manual flag, and the compute constraints pointing into the {ref}`constraint <constraint>` table. The `machine_manual` row's presence is the flag.
- **`machine_parent` (containers):** two pointers into the same machine table, the child's and its host's, capped at a single nesting level.
- **`machine_cloud_instance` (cloud instance):** one row, born with the machine. `instance_id` stays empty until the cloud creates the instance; the row then stores the hardware: arch, CPU, memory, root disk, `virt_type`, plus the availability zone. The row carries its own life pointer.
- **`machine_status`, `machine_cloud_instance_status` (status projections):** two upsert tables; every write is a membership check followed by an upsert. What constrains a machine is *who* writes which value.
  - `machine_status` is the machine agent's report about the Juju agent on the machine: `pending`, `started`, `stopped` when the agent sees the machine's life is no longer alive, `error` when the teardown request fails. `down` is written by no one; the status reads it when the agent has not been seen recently.
  - `machine_cloud_instance_status` is written controller-side: the provisioning lifecycle `pending`, `allocating`, `running` once the instance ID and addresses are registered, `provisioning error`, and `unknown` as the row's zero value.
  - The vocabularies live in lookup tables: `machine_status_value`, `machine_cloud_instance_status_value`, `life`.

The machine service writes the records: `AddMachine` inserts the machine row and its satellites, `SetMachineCloudInstance` fills the instance row once the provider reports it, and the removal service carries the teardown.

The machine table has no type column: a machine's kind is derived, not stored. The parent pointer is the container fact; the `machine_manual` row is the manual fact; `v_machine_is_controller` derives the controller machine as a machine whose net node runs a unit of the controller's application. A machine that is none of these is a regular machine: a cloud instance the controller's compute provisioner started.

(machine-persistence-rules)=
### Persistence rules and errors

Rules:

- a machine can only be declared dead once nothing is assigned to it -- units, containers, or storage keep it alive (see {ref}`Machine removal <machine-removal>`).

Errors:

- *existence and life*: `machine not found`, `machine not alive`, `machine is dead`;
- *provisioning*: `machine not provisioned`, meaning the instance record holds no instance ID yet; `machine cloud instance already exists`;
- *structure*: `machine has no parent`.

(the-machines-machinery)=
## Machines in the execution layer

By the time the command returns, the machine record exists; the cloud instance may still be allocating. In the controller, the compute provisioner and the instance poller drive the cloud instances. On the machine, the machine agent -- a {ref}`Juju agent <agent>` -- hosts the unit agents, provisions the containers, and shuts the machine down.

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: Topology: The two provisioning paths -- the controller's compute provisioner starts base machines and records the instance ID and addresses as they become known; a host machine's agent provisions its own containers through the LXD broker and watches them via the controller API. What a designation names: machine 0 and its LXD container are rows in the same machine table, the container linked to its host by a machine-parent record. Containers are machines: each runs its own machine agent, which hosts the unit agent.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

(machines-and-units)=
### Machine operations

#### Machine creation

Most machines are created implicitly: a deploy or add-unit placement resolves when the unit's machine record is written. See {ref}`Machine designations <machine-designations>` and {ref}`placement directive <placement-directive>`. Explicit creation goes through the add-machine operation (`juju add-machine`): the controller's machine manager writes the machine record, and the compute provisioner takes it from there.

(machine-provisioning)=
#### Machine provisioning
```{audience} juju-dev
```

The controller's compute provisioner is a model worker. It watches the model's unprovisioned machines, asks the cloud to start an instance for each pending machine, records the instance ID and addresses as they become known, and retries transient failures. See {ref}`Instance status <instance-status>`. Containers are provisioned by their host machine's agent through the LXD broker, which learns about its containers from the controller's API.

(machine-removal)=
#### Machine removal

Removal goes through the remove-machine operation (`juju remove-machine 0`). The machine, its containers included unless the removal is forced past them, is marked dying. The machine agent notices through its life watcher, stops, and asks the controller to mark the machine dead once no units or storage are assigned to it; a scheduled removal job deletes the records. The `keep-instance` flag decides whether the cloud instance is released with the machine.

(manual-provisioning)=
#### Manual provisioning

The add-machine operation over SSH (`juju add-machine ssh:user@host`) provisions nothing in the cloud. The controller renders the provisioning script; the user's SSH session runs it on the target host; the host's machine agent reports in. The machine's record keeps the manual flag (see {ref}`Manual machine <manual-machine>`).

(machine-agent-status)=
### Machine agent status

The machine agent writes its own status. Its shutdown loop:

```{ggarch}
:file: ../juju.ggarch
:view: Machine agent status
:no-legend:
:caption: State machine diagram: The machine agent's status as it shuts its machine down -- the agent reports started at startup; when the life watcher fires (the machine is no longer alive) it reports stopped and asks the controller to have the machine marked dead, waiting until the units and storage assigned to it clear; a failed request parks the status in error.
:alt: State machine: pending to started on machine agent startup; started to stopped when the life watcher fires; started to error when EnsureDead fails with units or storage still assigned; stopped internally waits for units and storage to clear, then dies.
```

(instance-status)=
### Instance status

The controller writes the instance's provisioning status. The provisioner's transitions:

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

Nothing about a machine is polled by the things that act on it: they watch it. The machine domain's watchable service exposes these surfaces:

- **One machine's life and dependants** -- changes to the machine's life and to the units, containers, and storage tied to it. The machine agent's shutdown watcher.
- **A parent machine's containers' life** -- drives the host machine agent's container provisioner.
- **The model's (non-container) machines** -- the compute provisioner's (and instance poller's) provisioning surface.
- **The model machines' life and start times** -- the instance poller's steady-state mirroring surface.
- **The model's machine cloud instances** and **a machine's reboot state** -- the instance poller's and the reboot machinery's surfaces.

Every watcher fires once immediately when it is created -- the initial query is the baseline snapshot -- and again on each qualifying change (see {ref}`the watcher pattern <watchers>`).

(machine-execution-rules)=
### Execution rules and errors

Rules:

- a machine's reported hardware must satisfy the constraints asked of it: the placement check compares the machine's instance record against the constraints and fails with `machine constraint violation` otherwise.

Errors:

- `provisioning error`: a failing cloud broker parks the instance there; a transient failure is retried back into `allocating` (see {ref}`Instance status <instance-status>`).
