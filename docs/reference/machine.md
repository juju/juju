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

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` requested implicitly through a deploy or add-unit placement, or explicitly through the controller API, from a machine {ref}`cloud <cloud>`. Juju treats both bare cloud instances and LXD containers on host instances as machines.

```{important}

A LXD container is named after its host machine: `0/lxd/5` is LXD container `5` on machine `0`. Containers appear under 'Machines' in status outputs.

```

(machine-declaration-rules)=
## Machines in the declaration layer

The declaration layer defines how clients express compute intent and submit targets to the controller API.

- **Explicit and implicit creation:** clients can explicitly request machines through the API, gated on model {ref}`write access <user-access-model-write>`. Most machines are created implicitly: a deploy or add-unit placement asks for compute, and the machine record follows.
- **Target designations:** a machine argument names new or existing targets, bare instances or containers, in comma-separated lists:
  - `0` targets an existing machine; `0,4` targets machines 0 and 4.
  - `lxd` requests a new LXD container on a new host; `lxd:25` on the existing machine 25. A VM host is requested by virtual-machine `virt-type`.
  - `0/lxd/4` targets the existing container `4` on machine `0`.
  - *Note:* designations are for machine clouds only; they are rejected on Kubernetes models.
- **Special machine kinds:**
  - **LXD container:** a machine linked to a host machine by a machine-parent record. The host's agent provisions its own containers, not the controller, so a container brings its host in as a machine of its own.
  - **Manual machine:** a machine the user provisioned themselves over SSH instead of asking the cloud for one. The records keep the manual flag, and the instance status reads `Manually provisioned machine` once the agent reports in.
  - **Controller machine:** the machine hosting the controller application, derived, not stored: a machine is the controller machine when a unit of the controller's application runs on it. It runs the {ref}`controller agent <controller-agent>` and hosts every model worker, the compute provisioner included.
- **Declaration rules and errors:**
  - *Rules:* designations use `0` or positive integers without leading zeros; container nesting is limited to a single level, `0/lxd/4`, never `0/lxd/0/lxd/0`.
  - *Errors:* `machine already exists`, `invalid container type`, `grandparent machine are not supported currently`, `invalid machine constraints`.

```{ibnote}
See also: {ref}`tfjuju:manage-machines <tfjuju:manage-machines>`
```

(the-machine-in-the-data-model)=
(machine-persistence-rules)=
## Machines in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Machine attributes
:alt: The machine's stored tables as an entity-relationship slice: the machine record at the centre with its name, life, net-node pointer and teardown flags; the parent table naming its child and its host west with the net node below it; the agent status record east; the cloud instance below with its own status under it. Each line is a stored pointer, 1/m at each end; nothing dashed; every drawn pointer is mandatory; the instance id is a nullable field, empty until the cloud reports it.
:caption: Entity relationship diagram: The machine's stored records and the schema associations between them. Each line starts at the fk column that holds the pointer, 1/m at each end; dashed means the row may be absent. The machine shares its net node with the units running on it; the parent record is two pointers into the same table, the single nesting level a container may have; the agent status record hangs off the machine and the instance status record off the instance. Not drawn: the base, the manual satellite row, the placement, constraints, storage attachments, agent version, SSH host keys, and LXD profiles, plus the lookup tables behind the status vocabularies.
```

In the model database, a machine is a native record (`0018-machine.sql`) distributed across a decoupled set of tables:

- **`machine`:** stores the identity pair. A hidden `uuid` acts as the internal join handle; a model-scoped natural key (`name`) provides the user-facing designation (`0`, `1/lxd/0`), strictly enforced to reject duplicates (`machine already exists`).
- **`net_node_uuid`:** a unique pointer to the network identity whose addresses the machine hangs off. The units running on the machine point at the same net node: sharing it is what "runs on" means in the data model.
- **`life_id`, `keep_instance`:** the shared alive / dying / dead cycle, and whether the cloud instance is released with the machine at teardown.
- **Configuration satellites:** three optional per-machine rows: the OS base as `os@channel` plus architecture (`machine_platform`), the manual flag (`machine_manual`, the row's presence is the flag), and the compute constraints (`machine_constraint`, a pointer into the {ref}`constraint <constraint>` table).
- **`machine_parent`:** two pointers into the same machine table, the child's and its host's, establishing the parent-child relationship for containers. Container nesting is capped at a single level.
- **`machine_cloud_instance`:** tracks cloud provisioning state, holding the instance identifiers, the hardware specs, and the availability zone once reported by the provider. The row is born with the machine, carries its own life pointer, and its `instance_id` stays empty until the cloud creates the instance.
- **Status projections:** two non-validated upsert tables tracking agent health (`machine_status`) and cloud provisioning lifecycles (`machine_cloud_instance_status`); every write is a membership check followed by an upsert, and what constrains a machine is who writes which value.
  - `machine_status` is the machine agent's report about the Juju agent on the machine: `pending`, `started`, `stopped` when the agent sees the machine's life is no longer alive, `error` when the teardown request fails. `down` is written by no one; the status reads it when the agent has not been seen recently.
  - `machine_cloud_instance_status` is written controller-side: the provisioning lifecycle `pending`, `allocating`, `running` once the instance ID and addresses are registered, `provisioning error`, and `unknown` as the row's zero value.
  - The vocabularies live in lookup tables: `machine_status_value`, `machine_cloud_instance_status_value`, `life`.
- **Persistence rules and errors:**
  - *Rules:* a machine can only be declared dead once nothing is assigned to it: units, containers, or storage keep it alive; a machine's kind is derived, not stored, and the schema view `v_machine_is_controller` derives the controller machine as a machine whose net node runs a unit of the controller's application.
  - *Errors:* `machine not found`, `machine not alive`, `machine is dead`, `machine not provisioned`, `machine cloud instance already exists`, `machine has no parent`.

The machine service writes these records: `AddMachine` inserts the machine row and its satellites, `SetMachineCloudInstance` fills the instance row once the provider reports it, and the removal service carries the teardown.

(the-machines-machinery)=
## Machines in the execution layer

By the time the command returns, the machine record exists; the cloud instance may still be allocating. In the controller, the compute provisioner and the instance poller drive the cloud instances. On the machine, the machine agent, a {ref}`Juju agent <agent>`, hosts the unit agents, provisions the containers, and shuts the machine down.

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: Topology: The two provisioning paths. The controller's compute provisioner starts base machines and records the instance ID and addresses as they become known; a host machine's agent provisions its own containers through the LXD broker and watches them via the controller API. What a designation names: machine 0 and its LXD container are rows in the same machine table, the container linked to its host by a machine-parent record. Containers are machines: each runs its own machine agent, which hosts the unit agent.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

(machines-and-units)=
### Machine operations

- **Provisioning paths:**
  - *Controller-side:* the compute provisioner watches the model's unprovisioned machines, asks the cloud to start an instance for each, records the instance ID and addresses as they become known, and retries transient failures.
  - *Agent-side:* the host machine's agent provisions its own containers through the LXD broker, learning about them from the controller's API.
- **Manual provisioning:** the controller renders the provisioning script, the user runs it on the target host over SSH, and the host's machine agent reports in to register itself. See {ref}`Instance status <instance-status>`.
- **Machine removal:** removal marks the machine, its containers included unless forced past them, dying; the machine agent notices through its life watcher and asks the controller to mark the machine dead once no units or storage are assigned to it; a scheduled removal job deletes the records. The `keep_instance` flag decides whether the cloud instance is released with the machine.
- **Execution rules and errors:**
  - *Rules:* a machine's reported hardware must satisfy the constraints asked of it; the placement check fails with `machine constraint violation` otherwise.
  - *Errors:* `provisioning error`, where a failing cloud broker parks the instance; a transient failure is retried back into `allocating`.

(machine-agent-status)=
### Machine agent status

The machine agent writes its own status. Its shutdown loop:

```{ggarch}
:file: ../juju.ggarch
:view: Machine agent status
:no-legend:
:caption: State machine diagram: The machine agent's status as it shuts its machine down. The agent reports started at startup; when the life watcher fires, the machine is no longer alive, it reports stopped and asks the controller to have the machine marked dead, waiting until the units and storage assigned to it clear; a failed request parks the status in error.
:alt: State machine: pending to started on machine agent startup; started to stopped when the life watcher fires; started to error when EnsureDead fails with units or storage still assigned; stopped internally waits for units and storage to clear, then dies.
```

(instance-status)=
### Instance status

The controller writes the instance's provisioning status. The provisioner's transitions:

```{ggarch}
:file: ../juju.ggarch
:view: Machine provisioning
:no-legend:
:caption: State machine diagram: The cloud instance's provisioning status. The controller's compute provisioner moves a pending machine to allocating, meaning starting, when it asks the cloud for the instance, to running once the instance and its addresses are registered, and to provisioning error when the broker fails; a transient error is retried back into allocating. In steady state the instance poller mirrors the provider-reported status.
:alt: State machine: pending to allocating on the compute provisioner starting the instance; allocating to running when instance and addresses are recorded; allocating to provisioning error on broker error; provisioning error back to allocating on a transient retry; running mirrors the provider status.
```

(machine-watchers)=
### Machine watchers
```{audience} juju-dev
```

The things that act on a machine watch it rather than poll it. The machine domain's watchable service exposes:

- one machine's life and dependants: the machine agent's shutdown watcher;
- a parent machine's containers' life: the host agent's container provisioner;
- the model's non-container machines: the compute provisioner's provisioning surface;
- the model machines' life and start times: the instance poller's steady-state mirroring surface;
- the model's machine cloud instances, and a machine's reboot state: the instance poller's and the reboot machinery's surfaces.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
