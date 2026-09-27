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

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` from a machine {ref}`cloud <cloud>`: a bare cloud instance, or a LXD container on one, requested implicitly by a deploy or add-unit placement, or explicitly through the controller API.

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

Runtime operations are split between the controller and the individual host machine. The controller orchestrates provisioning via background workers, while the local machine agent, a {ref}`Juju agent <agent>`, manages internal lifecycle and workload placement.

(machines-and-units)=
### Machine operations

- **Controller-side provisioning:** the compute provisioner monitors unprovisioned machines, requests cloud instances, and records the instance ID and addresses as they become known.
- **Agent-side provisioning:** host machine agents provision and manage local LXD containers through the LXD broker, watching them via the controller API.
- **Manual provisioning:** the controller renders a setup script, executed externally over SSH, after which the host agent registers itself.
- **Machine removal:** the controller marks target machines dying; the agent triggers shutdown once assigned units and storage clear; scheduled jobs purge the records; `keep_instance` decides whether the cloud instance is released.
- **Statuses:** the machine agent reports its status (`pending`, `started`, `stopped`, `error`); the controller writes the instance's provisioning status (`allocating`, `running`, `provisioning error`).

(machine-execution-rules)=
### Execution rules and errors

- **Rules:** reported hardware must satisfy the assigned machine constraints (`machine constraint violation`).
- **Errors:** `provisioning error` parks failed cloud allocations; transient broker failures retry back into `allocating`.

(machine-watchers)=
### Machine watchers
```{audience} juju-dev
```

The machine domain's watchable service exposes real-time change streams rather than polling loops:

- **Machine life and dependants:** drives the local shutdown watcher.
- **Container life:** drives host-side container provisioning loops.
- **Model machines:** drives the compute provisioner.
- **Instance states:** drives the instance poller's steady-state mirroring (life, start times, cloud instances) and the reboot machinery.

Every watcher fires an initial baseline snapshot on creation, followed by notifications on qualifying changes. See {ref}`the watcher pattern <watchers>`.
