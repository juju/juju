---
myst:
  html_meta:
    description: "Placement directive reference: specify deployment locations using --to flag with machines, zones, subnets, and availability zones."
---

(placement-directive)=
# Placement directive

<!--TO DOCS MAINTAINERS:
To retrieve info about the keys, grep the `provider` directory in the code for `placement` (case insensitive); find all the providers that match; and go to each of those providers' `parsePlacement` method and look at the code. For example, here's the ec2 one: https://github.com/juju/juju/blob/137a772ed339b73b856e9adc0a5624976c2890b2/provider/ec2/environ.go#L389 (note the switch statement with two cases, `zone` and `subnet`). Then follow a couple of the functions through to get further details (e.g., about the ec2 subset query).
--->

<!--  See also: {ref}`Binding <binding>`, {ref}`Constraint <constraint>`-->

In Juju, a **placement directive** is an option based on the `--to` flag that can be passed to certain commands to specify a deploy location, where the commands include {ref}`command-juju-add-machine` ,  {ref}`command-juju-add-unit`,  {ref}`command-juju-bootstrap`,  {ref}`command-juju-deploy`, and the location is  (1) an existing or a new machine or (2) a key-value pair specifying a subnet, system ID, or an availability zone.

Example: `juju add-machine --to 1`, `juju deploy --to zone=us-east-1a`

The rest of this document gives details about the locations.

```{caution}

When the location is a key-value pair, its availability and meaning may vary from cloud to cloud. For details see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

## List of placement directive locations

(placement-directive-machine)=
(machine-designations)=
### `<machine>`

A **machine designation** is the argument by which a command names machines: its shape says whether the machine exists and whether it is a host or a container on a host. A comma separates several:

| Shape | Meaning |
|-|-|
| (empty) | A new machine |
| `0` | Machine 0 |
| `0,4` | Machines 0 and 4 |
| `lxd` | A new LXD container, or with `virt-type=virtual-machine` a VM, on a new machine |
| `lxd:25` | A new LXD container, or with `virt-type=virtual-machine` a VM, on machine 25 |
| `0/lxd/4` | LXD container 4 on machine 0 |
| `3,0/lxd/2,lxd:5` | Machine 3, LXD container 2 on machine 0, and a new LXD container on machine 5 |

**Examples:** `juju add-machine lxd:5` (a new LXD container on machine `5`), `juju deploy --to 0/lxd/4` (deployed onto LXD container `4` on machine `0`)

(placement-directive-subnet)=
### `subnet=<subnet>`

Available for Azure and AWS EC2.

(placement-directive-system-id)=
### `system-id=<system ID>`

Available for MAAS.

(placement-directive-zone)=
### `zone=<zone>`

**Purpose:** To specify an availability zone. See {ref}`Requesting zones <zone-requesting>`.

```{important}

The `zone` placement directive may be used to override a `zones` {ref}`constraint <constraint>`.

```

**Example:** `zone=us-east-1a`
