---
myst:
  html_meta:
    description: "Placement directive reference: specify deployment locations using --to flag with machines, zones, subnets, and availability zones."
---

(placement-directive)=
# Placement directive

In Juju, a **placement directive** is an option based on the `--to` flag that can be passed to certain commands to specify a deploy location, where the commands include {ref}`command-juju-add-machine` ,  {ref}`command-juju-add-unit`,  {ref}`command-juju-bootstrap`,  {ref}`command-juju-deploy`,  {ref}`command-juju-enable-ha`, and the location is  (1) an existing or a new machine or (2) a key-value pair specifying a subnet, system ID, or an availability zone.

Example: `juju add-machine --to 1`, `juju deploy --to zone=us-east-1a`

The rest of this document gives details about the locations.

```{caution}

When the location is a key-value pair, its availability and meaning may vary from cloud to cloud. For details see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

## List of placement directive locations

(placement-directive-machine)=
### `<machine>`

Depending on whether this is an existing machine or a new machine, this will be:

- The existing machine ID.

**Examples:** `1` (existing machine `1`),  `5/lxd/0` (existing container `0` on machine `5`)

- A new machine, specifying a type or relative location.

**Examples:** `lxd` (new container on a new machine), `lxd:5` (new container on machine 5)

```{ibnote}
See more: {ref}`machine-designations`
```

(placement-directive-subnet)=
### `subnet=<subnet>`

Available for Azure and AWS EC2.

(placement-directive-system-id)=
### `system-id=<system ID>`

Available for MAAS.

(placement-directive-zone)=
### `zone=<zone>`

**Purpose:** To specify an availability zone.

```{important}

The `zone` placement directive may be used to override a `zones` {ref}`constraint <constraint>`.

```

**Example:** `zone=us-east-1a`

