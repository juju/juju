---
myst:
  html_meta:
    description: "Placement directive reference: specify deployment locations using --to flag with machines, zones, subnets, and availability zones. The directive value, where it is stored, and its rules."
---

(placement-directive)=
# Placement directive

In Juju, a **placement directive** is an option based on the `--to` flag that can be passed to certain commands to specify a deploy location, where the commands include {ref}`command-juju-add-machine` ,  {ref}`command-juju-add-unit`,  {ref}`command-juju-bootstrap`,  {ref}`command-juju-deploy`, and the location is  (1) an existing or a new machine or (2) a key-value pair specifying a subnet, system ID, or an availability zone.

Example: `juju add-machine --to 1`, `juju deploy --to zone=us-east-1a`

(the-placement-directive-record)=
## The placement directive record

A placement directive is a **value, not an entity**: it is the
request's answer to "where should this land?", and what persists is
the *resolution* -- a placement record on the
{ref}`machine <machine>` (its scope -- the model, or the specific
machine -- plus the directive it was resolved from), written when the
unit's machine record is created. The directive itself is not stored.

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

(the-placement-directive-in-the-data-model)=
## The placement directive in the data model

The resolved placement is the machine's placement record (scope plus
directive); the un-resolved directive lives only in the request. The
value forms the directive admits -- an existing or new
{ref}`machine designation <machine-designations>`, a subnet, a MAAS
system ID, an availability {ref}`zone <zone>` -- are the list above.

(the-placement-directive-states)=
## Placement directive states

Not applicable -- a directive is request input: it is resolved when
the machine record is written and does not exist as state.

(the-placement-directive-operations)=
## Placement directive operations

The directive is consumed where compute is requested -- deploy,
add-machine, add-unit, bootstrap -- and resolved against the model's
machines (an existing designation) or the provisioner (a new one).

(the-placement-directive-watchers)=
## Placement directive watchers

Not applicable -- nothing watches a directive; the machines it
resolves into have their own.

(the-placement-directive-rules-and-errors)=
## Placement directive rules and errors

```{caution}

When the location is a key-value pair, its availability and meaning may vary from cloud to cloud. For details see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

- a machine designation must satisfy the designation grammar
  (see {ref}`machine rules and errors <machine-rules-and-errors>`);
- the `--to` argument is rejected on Kubernetes models;
- a key-value directive must name a value the cloud can honour
  (`zone=`, `subnet=`, `system-id=` per the list above); a `zone=`
  directive overrides the `zones` {ref}`constraint <constraint>`.

(related-entities-placement-directive)=
## Related entities

- **Machines** are what a directive resolves into, and the placement
  record is theirs (see {ref}`machine <machine>`).
- **Constraints** are the other steering input -- the directive wins
  on overlap (see {ref}`constraint <constraint>`).
- **Zones and subnets** are the key-value forms (see {ref}`zone
  <zone>`, {ref}`subnet <subnet>`).
