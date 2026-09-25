---
myst:
  html_meta:
    description: "Juju constraints reference: customize compute resources with specifications for CPU, memory, storage, networking, and instance types. The constraint value, where it is stored, and its rules."
---

(constraint)=
# Constraint
```{audience} user
```

In Juju, a **constraint** is a key-value pair that represents a specification that can be passed to certain `juju` commands /command flags to customise the {ref}`compute resources <resource-compute>` (bare metal machines, virtual machines, system containers, Kubernetes containers) spawned by Juju.

If the resource is a bare metal machine or a virtual machine, a constraint represents a minimum, whereas if the resource is a system container or a Kubernetes container it represents a maximum.

(the-constraints-records)=
## The constraint's records

(the-constraint-record)=
### The constraint's identity

A constraint is a **value, not an entity**: it has no life, no status
and no watchers of its own -- it is a stored key/value whose meaning
comes from the entity it constrains. The records: the
{ref}`model <model>`'s constraints (its defaults for everything it
spawns), an {ref}`application's <application>` constraints record
(nullable -- an application may inherit), and a
{ref}`machine's <machine>` constraints record (what that machine was
provisioned with).

(the-constraint-in-the-data-model)=
### The constraint in the data model

The constraint records live where the constrained entity lives: the
model's constraint record in the model database, the application's and
the machines' in the model database beside their owners. If
constraints are set at multiple levels at once -- that is, with
overlap -- the constraint applied at the more specific level takes
precedence (application over model, machine over application).

(the-constraint-states)=
### Constraint states

Not applicable -- a constraint is a stored value: it is written when
set and read at provisioning time; nothing transitions.

(types-of-constraint)=
### Types of constraint

Not applicable -- constraints have no subtypes; each key below is one
independent value.

(the-constraints-machinery)=
## The constraint's machinery

A constraint has no machinery of its own: it is a stored value the
compute provisioner reads at provisioning time -- setting it is a
rewrite of the owner's record.

(the-constraint-operations)=
### Constraint operations

Constraints are set at the level that should own them -- on the model
(`juju set-model-constraints`), the application (`juju
set-constraints`), or as deploy/add-machine flags -- and honoured when
the compute provisioner asks the cloud for resources (see
{ref}`machine provisioning <machine-provisioning>`). There is no
constraint entity to update later: setting is a rewrite of the
owner's constraint record.

(the-constraint-watchers)=
### Constraint watchers

Not applicable -- no watch surface exposes constraint records;
provisioning reads them at the moment it provisions.

(the-constraint-rules-and-errors)=
## Constraint rules and errors
```{audience} charm-dev
```

```{caution}

Some of these keys -- their availability and their meaning -- vary from one cloud to another. Below this is indicated with a generic note. For specifics see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

- a constraint key must be one of the known keys, and its value must
  parse for that key (integers with M/G/T/P suffixes, name lists,
  booleans); an unparseable or unknown constraint is rejected
  (`invalid machine constraints` / `machine constraint violation` at
  provisioning);
- the `spaces` constraint names {ref}`spaces <space>` that must (or,
  with the `^` prefix, must not) reach the machine, and the `zones`
  constraint names {ref}`availability zones <zone>`;

(list-of-constraints)=
## List of constraints

(constraint-allocate-public-ip)=
### `allocate-public-ip`

Supplying this constraint will determine whether machines are issued an IP address accessible outside of the cloud's virtual network.  <p> **Valid values:** `true`, `false`. <p> **Note:** Applies to public clouds (GCE, EC2, Azure) and OpenStack. Public cloud instances are assigned a public IP by default.

(constraint-arch)=
### `arch`

The architecture. <br> <br>**Valid values:** `amd64`, `arm64`, `ppc64el`, `s390x`, `riscv64`.

(constraint-container)=
### `container`

If not nil, indicates that a machine must be the specified container type. <br> <br> **Valid values:** `lxd`.

(constraint-cores)=
### `cores`

Number of effective CPU cores. <br> <br> **Type:** integer. <br> <br> **Alias:** `cpu-cores`.

(constraint-cpu-power)=
### `cpu-power`
Abstract CPU power. <br> <br> **Type:** integer, where 100 units is roughly equivalent to "a single 2007-era Xeon" as reflected by 1 Amazon vCPU. In a Kubernetes context a unit of "milli" is implied. <p> **Note:** Not supported by all providers. Use `cores` for portability.

(constraint-image-id)=
### `image-id`

```{versionadded} 3.2.0
```

The image ID. If not nil, indicates that a machine must use the specified image.

**Note:** Not supported by all providers. Value is provider-specific.  Also, when applied during `juju deploy`, must be used in conjunction with the `--base` flag of the command -- the `image-id` will specify the image to be used for the provisioned machines and the `--base` will specify the operating system  used by the image to be deployed on those machines.

(constraint-instance-role)=
### `instance-role`

Indicates that the specified role/profile for the given cloud should be used.  <p> **Note:** Only valid for clouds which support instance roles. Currently only for AWS with instance profiles and (starting with Juju 3.6) for Azure with managed identities.

(constraint-instance-type)=
### `instance-type`

Cloud-specific instance-type name. Values vary by provider, and individual deployment in some cases. <p> **Note:** When compatibility between clouds is desired, use corresponding values for `cores`, `mem`, and `root-disk` instead.

(constraint-mem)=
### `mem`

Memory (MiB). An optional suffix of M/G/T/P indicates the value is mega-/giga-/tera-/peta- bytes.

(constraint-root-disk)=
### `root-disk`

Disk space on the root drive (MiB). An optional suffix of M/G/T/P is used as per the `mem` constraint. Additional storage that may be attached separately does not count towards this value.

(constraint-root-disk-source)=
### `root-disk-source`

Name of the storage pool or location the root disk is from. <p> **Note:** `root-disk-source` has different behaviour with each provider.

(constraint-spaces)=
### `spaces`

A comma-delimited list of Juju network space names that a unit or machine needs access to. Space names can be positive, listing an attribute of the space, or negative (prefixed with "^"), listing something the space does not have. <p> Example: `spaces=storage,db,^logging,^public` (meaning, select machines connected to the storage and db spaces, but NOT to logging or public spaces). <p> **Note:** EC2 and MAAS are the only providers that currently support the spaces constraint.

(constraint-tags)=
### `tags`

Comma-delimited tags assigned to the machine. Tags can be positive, denoting an attribute of the machine, or negative (prefixed with `^`), to denote something that the machine does not have. <p> Example: `tags=virtual,^dualnic` <p> **Note:** Supported by MAAS and by Kubernetes clouds (where it is used for pod affinity and anti-affinity).

(constraint-virt-type)=
### `virt-type`

Virtualisation type. <p> Only supported by {ref}`LXD <cloud-lxd>` and {ref}`OpenStack <cloud-openstack>`. **Valid values:** Cloud-dependent.

(constraint-zones)=
### `zones`

A list of availability zones.  Multiple values present a range of zones that a machine must be created within. <p> **Valid values:** Depending on the cloud provider. <p> **Example for the `aws` cloud:** `zones=us-east-1a,us-east-1c` <p> **Note:** A zone can also be used as a {ref}`placement directive <placement-directive>` (`--to zone= <name of zone>`).
