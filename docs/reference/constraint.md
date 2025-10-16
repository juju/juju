(constraint)=
# Constraint

In Juju, a **constraint** is a key-value pair that represents a specification that can be passed to certain `juju` commands /command flags to customise the {ref}`compute resources <resource-compute>` (bare metal machines, virtual machines, system containers, Kubernetes containers) spawned by Juju.

If the resource is a bare metal machine or a virtual machine, a constraint represents a minimum, whereas if the resource is a system container or a Kubernetes container it represents a maximum.

For machine (non-Kubernetes) clouds, constraints can be set directly on individual {ref}`machines <machine>`. However, more commonly they are set at the level of the {ref}`controller <controller>`, {ref}`model <model>`, or {ref}`application <application>`. If you set constraints at multiple levels at once -- that is, with overlap -- the constraint applied at the more specific level takes precedence.

The rest of this document describes all the existing constraints.

```{caution}

Some of these keys -- their availability and their meaning -- vary from one cloud to another. Below this is indicated with a generic note. For specifics see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

<!--
A constraint is a specification that operators indicate to Juju. When adding units, Juju attempts to use the smallest instance type on the cloud that satisfies all of the constraints.

Constraints are not specific to individual machines, but the whole application. Constraints can also be applied during the bootstrap process.
-->


<!--FROM THE ABOUT DOC. TODO: INTEGRATE ABOVE.
<a href="#heading--introduction"><h2 id="heading--introduction">Introduction</h2></a>

A *constraint* is a user-defined hardware specification for a machine that is spawned by Juju. There are  in all ten types of constraints, with the most common ones being `mem`, `cores`, `root-disk`, and `arch`. The definitive constraint resource is found on the https://juju.is/docs/olm/constraints-reference page.

Several noteworthy constraint characteristics:

-   A constraint can be specified whenever a new machine is spawned with the command `bootstrap`, `deploy`, or `add-machine`.
-   Some constraints are only supported by certain clouds.
-   When used with `deploy`, the constraint becomes the application's default constraint.
-   Multiple constraints are logically ANDs (i.e., the machine must satisfy all constraints).
-   When used in conjunction with a placement directive (the `--to` option), the placement directive takes precedence.
-->

(list-of-constraints)=
## List of constraints

<!--Source: https://github.com/juju/juju/blob/develop/core/constraints/constraints.go#L23 -->

(constraint-allocate-public-ip)=
### `allocate-public-ip`

Supplying this constraint will determine whether machines are issued an IP address accessible outside of the cloud's virtual network.  <p> **Valid values:** `true`, `false`. <p> **Note:** Applies to public clouds (GCE, EC2, Azure) and OpenStack. Public cloud instances are assigned a public IP by default.

(constraint-arch)=
### `arch`

The architecture. <br> <br>**Valid values:** `amd64`, `arm64`, `ppc64el`, `s390x`, `riscv64`.

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

Comma-delimited tags assigned to the machine. Tags can be positive, denoting an attribute of the machine, or negative (prefixed with `^`), to denote something that the machine does not have. <p> Example: `tags=virtual,^dualnic` <p> **Note:** Currently only supported by the MAAS provider.

(constraint-virt-type)=
### `virt-type`

Virtualisation type. <p> **Valid values:** `virtual-machine`. When a machine is provisioned with a `lxd` specification, used to override the default type, which is a LXD container.

(constraint-zones)=
### `zones`

A list of availability zones.  Multiple values present a range of zones that a machine must be created within. <p> **Valid values:** Depending on the cloud provider. <p> **Example for the `aws` cloud:** `zones=us-east-1a,us-east-1c` <p> **Note:** A zone can also be used as a {ref}`placement directive <placement-directive>` (`--to zone= <name of zone>`).