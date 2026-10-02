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

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` from a machine {ref}`cloud <cloud>`: a bare cloud instance, or a LXD container on one, requested implicitly by a deploy or add-unit placement, or explicitly through the controller API.

```{important}

A LXD container is named after its host machine: `0/lxd/5` is LXD container `5` on machine `0`. Containers appear under 'Machines' in status outputs.

```

(machine-declaration-rules)=
## Machines in the declaration layer

How clients express compute intent and submit targets to the controller API.

- **Explicit creation:** API requests for machines, gated on model {ref}`write access <user-access-model-write>`.
- **Implicit creation:** Most machines follow from deploy or add-unit placements.
- **Target designations:** Comma-separated lists naming new or existing targets.
  - `0`: an existing machine; `0,4`: machines 0 and 4.
  - `lxd`: a new container on a new host; `lxd:25`: on machine 25.
  - `0/lxd/4`: the existing container `4` on machine `0`.
  - Machine clouds only; rejected on Kubernetes models.
  - *Rule:* Machine numbers are `0` or positive integers without leading zeros.
  - *Rule:* Containers nest one level deep (`0/lxd/4`, never `0/lxd/0/lxd/0`).
  - *Related errors:*
    - **`machine name "<name>" has too many containers`**: *Trigger:* A machine name with more than one container level. *Remediation:* Name the container directly under its host machine.
    - **`invalid container type "<type>"`**: *Trigger:* A designation or a container constraint names a container type other than `lxd` (the only one 4.1 supports). *Remediation:* Use `lxd`.
- **Machine constraints:** Constraints can be set directly on a machine, as well as on the model or an application.
  - *Related errors:*
    - **`invalid machine constraints`**: *Trigger:* The constraints name a container type or a space that does not exist. *Remediation:* Correct the container type or the space.
    - **`machine constraint violation`**: *Trigger:* The constraints are set on a machine whose recorded cloud instance has a different architecture from the one required; architecture is the only characteristic compared. *Remediation:* Require the architecture the machine has, or request a new machine.
- **Special machine kinds:**
  - **LXD container:** A machine linked to its host by a machine-parent record; provisioned by the host's agent.
  - **Manual machine:** Provisioned by the user over SSH; the records keep the manual flag.
  - **Controller machine:** Derived, not stored; hosts the {ref}`controller agent <controller-agent>` and every model worker.

```{ibnote}
See also: {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(the-machine-in-the-data-model)=
(machine-persistence-rules)=
## Machines in the persistence layer

In the {ref}`model database <database>`, a machine is a native record, distributed across a decoupled set of records.

- **Primary entry:** The single record that anchors a machine: an internal system ID the other records point at, and the name the client types (the designation `0`, `1/lxd/0`), unique per model.
  - *Related errors:*
    - **`machine not found`**: *Trigger:* A lookup by machine UUID or name that matches no machine. *Remediation:* Check the model's machines.
    - **`machine already exists`**: *Trigger:* A model migration imports a machine whose name the target model already holds. *Remediation:* Migrate into a model that has no machine of that name.
- **Network identity:** A pointer naming the shared net node; a machine and all units running on it anchor to the same node.
- **Life:** `alive`, `dying` or `dead`, plus a flag saying whether the cloud instance survives teardown.
  - *Related error:*
    - **`machine is dead`**: *Trigger:* An operation on a machine whose life is `dead`. *Remediation:* None; the machine's lifecycle has ended.
- **Base:** The OS channel plus architecture. A satellite record flags manually provisioned machines; containers carry their container type.
- **Constraints:** A pointer into the {ref}`constraint <constraint>` record.
- **Parent record (containers only):** Holds two pointers, the child and its host.
  - *Related error:*
    - **`machine has no parent`**: *Trigger:* Asking for the parent of a machine that is not a container. *Remediation:* Check that the machine is a container (`<host>/lxd/<n>`).
- **Cloud instance record:** Tracks the creation state, the hardware (arch, CPU, memory, root disk, virtualisation type) and the availability zone. Born with the machine; the instance ID stays empty until the cloud reports it.
  - *Related errors:*
    - **`machine not provisioned`**: *Trigger:* An action that needs the instance runs before the cloud has reported an instance ID. *Remediation:* Wait until the compute provisioner has recorded the instance.
    - **`machine cloud instance already exists`**: *Trigger:* Recording an instance for a machine that already has one. *Remediation:* Check for a second provisioner acting on the same machine.
- **Status records:** Two records.
  - The agent's health report: `pending`, `started`, `stopped`, `error`. `down` is never written, only read when the agent has not been seen recently.
  - The provisioning lifecycle, written by the controller and by the provider or container broker: `pending`, `allocating`, `running`, `provisioning error`, `unknown`. `provisioning error` is a status value: cloud and container providers record it when allocation fails.
- **Derived attributes:** Machine kinds are derived rather than stored; the schema's own views classify a machine (controller, container, manually provisioned) from the records around it.

**Writers:** The machine service inserts the records (`AddMachine`, `SetMachineCloudInstance`); the removal service carries out the teardown.

(the-machines-machinery)=
(machine-execution-rules)=
## Machines in the execution layer

Runtime operations are split between the controller and the individual host machine. The controller orchestrates provisioning through background workers, while the local machine agent, a {ref}`Juju agent <agent>`, manages internal lifecycle and workload placement.

(machines-and-units)=
### Machine operations and status

- **Controller-side provisioning:** The compute provisioner monitors unprovisioned machines, requests cloud instances, and records the instance ID and addresses as they become known.
- **Agent-side provisioning:** Host machine agents provision and manage local LXD containers through the LXD broker, watching them via the controller API.
- **Manual provisioning:** The controller renders a setup script, executed externally over SSH, after which the host agent registers itself.
- **Machine removal:** The controller marks the machine `dying`; the agent triggers shutdown once assigned units and storage clear; scheduled jobs purge the records; the machine's keep-instance flag decides whether the cloud instance is released.
  - *Rule:* A machine becomes `dead` only when it is `dying` and hosts no containers, units or storage.
  - *Related errors:*
    - **`entity still alive`**: *Trigger:* Marking a machine `dead` while its life is still `alive`. *Remediation:* Start the removal first, so the machine becomes `dying`.
    - **`machine has containers`**, **`machine has units`**, **`machine has storage`**: *Trigger:* Marking a machine `dead` while it still hosts containers, units or storage. *Remediation:* Remove or move them first.
- **Reboot:** The reboot machinery decides whether a machine shuts down, reboots or does nothing: a machine whose parent needs a reboot shuts down, a machine that needs a reboot itself reboots, and otherwise nothing happens.
  - *Related error:*
    - **`grandparent machine are not supported currently`**: *Trigger:* The lookup finds a machine whose parent itself has a parent. *Remediation:* None; name validation refuses containers nested deeper than one level, so the check guards a layout that Juju does not support.
- **Statuses:** The machine agent reports its status (`pending`, `started`, `stopped`, `error`); the controller and the providers write the instance's provisioning status (`allocating`, `running`, `provisioning error`).

(machine-watchers)=
### Machine watchers

The machine domain's watchable service exposes real-time change streams rather than polling loops:

- **Machine life and dependants:** Drives the local shutdown watcher.
- **Container life:** Drives host-side container provisioning loops.
- **Model machines:** Drives the compute provisioner.
- **Instance states:** Drives the instance poller's steady-state mirroring (life, start times, cloud instances) and the reboot machinery.

Every watcher fires an initial baseline snapshot on creation, followed by notifications on qualifying changes. See {ref}`the watcher pattern <watchers>`.
