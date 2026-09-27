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

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` from a machine {ref}`cloud <cloud>`: a bare cloud instance, or a LXD container on one. Requested implicitly by a deploy or add-unit placement, or explicitly through the controller API.

```{important}

A LXD container is named after its host machine: `0/lxd/5` is LXD container `5` on machine `0`. Containers appear under 'Machines' in status outputs.

```

## Machines in the declaration layer

How clients express compute intent and submit targets to the controller API.

- **Explicit creation:** API requests for machines, gated on model {ref}`write access <user-access-model-write>`.
- **Implicit creation:** most machines follow from deploy or add-unit placements.
- **Target designations:** comma-separated lists naming new or existing targets:
  - `0`: an existing machine; `0,4`: machines 0 and 4.
  - `lxd`: a new container on a new host; `lxd:25`: on machine 25.
  - `0/lxd/4`: the existing container `4` on machine `0`.
  - Machine clouds only; rejected on Kubernetes models.
- **Special machine kinds:**
  - **LXD container:** a machine linked to its host by a machine-parent record; provisioned by the host's agent.
  - **Manual machine:** provisioned by the user over SSH; the records keep the manual flag.
  - **Controller machine:** derived, not stored; hosts the {ref}`controller agent <controller-agent>` and every model worker.

```{ibnote}
See also: {ref}`tfjuju:manage-machines <tfjuju:manage-machines>`
```

(machine-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - Designations use `0` or positive integers without leading zeros.
  - Container nesting is capped at a single level (`0/lxd/4`, never `0/lxd/0/lxd/0`).
- **Errors:**
  - `machine already exists`
  - `invalid container type`
  - `grandparent machine are not supported currently`
  - `invalid machine constraints`

(the-machine-in-the-data-model)=
## Machines in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Machine attributes
:alt: The machine's stored tables as an entity-relationship slice: the machine record at the centre with its name, life, net-node pointer and teardown flags; the parent table naming its child and its host west with the net node below it; the agent status record east; the cloud instance below with its own status under it. Each line is a stored pointer, 1/m at each end; nothing dashed; every drawn pointer is mandatory; the instance id is a nullable field, empty until the cloud reports it.
:caption: Entity relationship diagram: The machine entity schema and relational associations. Notation: each line starts at the foreign key holding the pointer; 1/m cardinality at each end; dashed would mark a row that may be absent (none here). Core tables: `machine` anchors the entity, linked to `machine_parent` for container hierarchies, `net node` for the shared network identity, `machine_cloud_instance` for provider state, and the status satellites (`machine_status`, `machine_cloud_instance_status`). Omitted for clarity: the satellite configuration rows (the manual flag, the OS base, constraints), storage attachments, and the status lookup tables.
```

In the model database, a machine is a native record (`0018-machine.sql`) distributed across a decoupled table schema:

- **`machine`**: stores the core identity pair. An internal `uuid` serves as the relation join handle; a model-scoped natural key (`name`) provides the user designation (`0`, `1/lxd/0`), enforced to reject duplicates (`machine already exists`).
- **`net_node_uuid`**: unique pointer to the shared network identity; machines and all units running on them anchor to the same net node.
- **`life_id`**: tracks the shared entity lifecycle state (alive, dying, dead).
- **`keep_instance`**: boolean flag determining whether the cloud instance persists after machine teardown.
- **`machine_platform`**: optional satellite storing the OS base (`os@channel` plus architecture).
- **`machine_manual`**: optional satellite whose mere row presence flags the machine as manually provisioned.
- **`machine_constraint`**: optional pointer into the {ref}`constraint <constraint>` table.
- **`machine_parent`**: two pointers into the machine table establishing container host-child relationships, strictly capped at a single nesting level.
- **`machine_cloud_instance`**: single provider record tracking creation state, hardware specs (arch, CPU, memory, root disk, `virt_type`), and the availability zone. Born with the machine; `instance_id` empty until the cloud reports it; carries its own life pointer.
- **`machine_status`**: upsert table tracking the machine agent's health reports on the Juju agent (`pending`, `started`, `stopped`, `error`); `down` is never written, only read when the agent has not been seen recently.
- **`machine_cloud_instance_status`**: controller-written upsert table tracking the provisioning lifecycle (`pending`, `allocating`, `running`, `provisioning error`, `unknown`).

Writers: the machine service inserts the records (`AddMachine`, `SetMachineCloudInstance`); the removal service carries the teardown.

(machine-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - Machines transition to dead only when all assigned units, containers, and storage clear.
  - Machine kinds are derived via schema views rather than stored type columns (`v_machine_is_controller`).
- **Errors:**
  - `machine not found`
  - `machine not alive`
  - `machine is dead`
  - `machine not provisioned`
  - `machine cloud instance already exists`
  - `machine has no parent`

(the-machines-machinery)=
## Machines in the execution layer

In the controller, the compute provisioner and the instance poller; on the machine, the machine agent, a {ref}`Juju agent <agent>`.

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: Topology: The two provisioning paths. The controller's compute provisioner starts base machines and records the instance ID and addresses as they become known; a host machine's agent provisions its own containers through the LXD broker and watches them via the controller API. What a designation names: machine 0 and its LXD container are rows in the same machine table, the container linked to its host by a machine-parent record. Containers are machines: each runs its own machine agent, which hosts the unit agent.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

(machines-and-units)=
### Machine operations

- **Controller-side provisioning:** the compute provisioner starts a cloud instance for each unprovisioned machine, then records the instance ID and addresses.
- **Agent-side provisioning:** the host machine's agent provisions its containers through the LXD broker.
- **Manual provisioning:** the controller renders the script; the user runs it on the host over SSH; the host's machine agent reports in.
- **Removal:** the machine is declared dead once nothing is assigned; a scheduled job deletes the records; `keep_instance` releases the cloud instance.

(machine-execution-rules)=
### Execution rules and errors

- **Rules:**
  - Reported hardware must satisfy the assigned constraints (`machine constraint violation`).
- **Errors:**
  - `provisioning error`: a failing cloud broker; transient failures retry back into `allocating`.

(machine-agent-status)=
### Machine agent status

The machine agent's report; its shutdown loop:

```{ggarch}
:file: ../juju.ggarch
:view: Machine agent status
:no-legend:
:caption: State machine diagram: The machine agent's status as it shuts its machine down. The agent reports started at startup; when the life watcher fires, the machine is no longer alive, it reports stopped and asks the controller to have the machine marked dead, waiting until the units and storage assigned to it clear; a failed request parks the status in error.
:alt: State machine: pending to started on machine agent startup; started to stopped when the life watcher fires; started to error when EnsureDead fails with units or storage still assigned; stopped internally waits for units and storage to clear, then dies.
```

(instance-status)=
### Instance status

The controller-written provisioning status; the provisioner's transitions:

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

The machine domain's watchable service exposes:

- one machine's life and dependants: the machine agent's shutdown watcher;
- a parent machine's containers' life: the host agent's container provisioner;
- the model's non-container machines: the compute provisioner's provisioning surface;
- the model machines' life and start times: the instance poller's mirroring surface;
- the model's machine cloud instances, and a machine's reboot state: the instance poller's and the reboot machinery's surfaces.

Every watcher fires once immediately on creation, then on each qualifying change. See {ref}`the watcher pattern <watchers>`.
