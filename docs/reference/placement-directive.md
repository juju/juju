---
myst:
  html_meta:
    description: "Placement directive reference: specify deployment locations with machine designations, zones, subnets, and system IDs. The directive value, where it is stored, and its rules."
---

(placement-directive)=
# Placement directive

In Juju, a **placement directive** names the location a compute request targets: an existing machine, a container designation, or a key-value pair naming a subnet, a system ID, or an availability zone.

(the-placement-directive-declaration-rules)=
## The placement directive in the declaration layer

How clients target compute at a location.

- **Where it travels:** The directive is passed wherever compute is requested: deploy, add-unit, add-machine, and bootstrap requests; the deploy, add-unit, and add-machine paths are gated on model {ref}`write access <user-access-model-write>`.
  - *Rule:* Kubernetes models take no placement directives.
  - *Related error:*
    - **`k8s models do not support placement directives`**: *Trigger:* A directive on an add-unit request for a Kubernetes model. *Remediation:* Place on machine clouds only.
- **Machine forms:** A directive naming a machine uses the {ref}`machine designations <machine>`: an existing machine or container, or a new one.
  - *Rule:* A machine designation must satisfy the designation grammar (see {ref}`the machine's declaration rules <machine-declaration-rules>`).
  - *Related errors:*
    - **`invalid --to parameter "<directive>"`**: *Trigger:* The client cannot parse the directive. *Remediation:* Use a valid directive form.
    - **`invalid placement`**: *Trigger:* The controller cannot resolve the directive. *Remediation:* Check the directive and that the named machine exists in the model.
- **Key-value forms:** A directive naming a location is a key-value pair: `subnet=`, `system-id=`, or `zone=`, handed to the provider.
  - *Rule:* Bootstrap accepts only the unscoped key-value forms; the model does not exist yet to place into.
  - *Related errors:*
    - **`unsupported bootstrap placement directive "<directive>"`**: *Trigger:* A scoped directive on the bootstrap path. *Remediation:* Pass only the unscoped key-value forms.
    - **`invalid model id "<id>"`**: *Trigger:* A provider-scoped directive whose value is not the model's UUID. *Remediation:* Use the model-scoped form.
- **Overlap with constraints:** A `zone=` directive overrides the `zones` {ref}`constraint <constraint>`.
- **Overlap with container types:** A machine request names a container type or a placement, not both.
  - *Related error:*
    - **`container type and placement are mutually exclusive`**: *Trigger:* An add-machine request that names both. *Remediation:* Use one or the other.

```{ibnote}
See examples: {ref}`deploy-a-charm` (the deploy-targets examples).
The Terraform provider takes a placement argument on its machine
resource; no Terraform howto covers placement directives.
```

(the-placement-directive-persistence-rules)=
## The placement directive in the persistence layer

A placement directive is a **value, not an entity**: the directive itself is not stored; what persists is its resolution.

- **Placement record:** One per machine, the machine UUID being the natural key. It carries the directive string verbatim and a scope. The schema seeds one scope, `provider`.
  - *Rule:* A machine has exactly one placement record.
- **What is not stored:** Only the key-value forms are recorded. A machine designation resolves into the machine records themselves: an existing machine is reused, a container designation creates the parent and child machine records.

**Writers:** The record is written at machine-creation time, by the machine state's placement resolver, called from the machine service's machine-creation path.

## The placement directive in the execution layer

A placement directive has no machinery of its own: it is request input, resolved when the machine record is written. Whether the cloud can honour a key-value directive is discovered later, at provisioning time, when the machine provisioner reads the stored directive back and asks the cloud for a machine in that location (see {ref}`machine provisioning <the-machines-machinery>`). Nothing watches a directive; the machines it resolves into have their own watchers.

## List of placement directives

```{caution}

When the location is a key-value pair, its availability and meaning may vary from cloud to cloud. For details see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

(placement-directive-machine)=
### `<machine>`

Depending on whether this is an existing machine or a new machine, this will be:

- The existing machine ID.

**Examples:** `1` (existing machine `1`),  `5/lxd/0` (existing container `0` on machine `5`)

- A new machine, specifying a type or relative location.

**Examples:** `lxd` (new container on a new machine), `lxd:5` (new container on machine 5)

See the full grammar: {ref}`machine designations <machine>`.

(placement-directive-subnet)=
### `subnet=<subnet>`

Available for Azure, AWS EC2, and GCE.

(placement-directive-system-id)=
### `system-id=<system ID>`

Available for MAAS.

(placement-directive-zone)=
### `zone=<zone>`

**Purpose:** To specify an availability zone.

```{important}

The `zone` placement directive may be used to override a `zones` {ref}`constraint <constraint>`.

```
