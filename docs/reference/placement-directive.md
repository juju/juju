---
myst:
  html_meta:
    description: "Placement directive reference: specify deployment locations using --to flag with machines, zones, subnets, and availability zones. The directive value, where it is stored, and its rules."
---

(placement-directive)=
# Placement directive
```{audience} user
```

In Juju, a **placement directive** is an option based on the `--to` flag that can be passed to certain commands to specify a deploy location, where the commands include {ref}`command-juju-add-machine` ,  {ref}`command-juju-add-unit`,  {ref}`command-juju-bootstrap`,  {ref}`command-juju-deploy`, and the location is  (1) an existing or a new machine or (2) a key-value pair specifying a subnet, system ID, or an availability zone.

Example: `juju add-machine --to 1`, `juju deploy --to zone=us-east-1a`

Its neighbours: the {ref}`machine <machine>` it resolves into, the
{ref}`constraint <constraint>` it overrides on overlap (a `zone=`
directive beats the `zones` constraint), and the {ref}`zones <zone>`
and {ref}`subnets <subnet>` that the key-value forms name.

## Placement directives in the declaration layer

You pass a placement directive with the `--to` flag wherever compute
is requested: `juju deploy`, `juju add-unit`, and `juju add-machine`
(model {ref}`write access <user-access-model-write>` -- the
Application and Machinemanager facades gate deploy, add-unit and
add-machine on model write), and `juju bootstrap` on the
controller-creation path (no model access gate of its own -- the
model does not exist yet; bootstrap accepts only the unscoped
key-value forms). The controller-side counterpart is the same call
over the controller API, the directive travelling as part of the
request. The machine-shaped forms are spelled with the
{ref}`machine designations <machine>`.

```{ibnote}
See examples: {ref}`deploy-a-charm` (the deploy-targets examples).
The Terraform provider takes a placement argument on its machine
resource; no Terraform howto covers placement directives.
```

(the-placement-directives-records)=
(the-placement-directive-record)=
(the-placement-directive-in-the-data-model)=
## Placement directives in the persistence layer

A placement directive is a **value, not an entity**: the directive
itself is not stored. What persists is the *resolution*: a placement
record on the {ref}`machine <machine>` -- one row per machine (the
machine UUID is the natural key: a machine has exactly one placement
record), carrying the directive string verbatim and a scope that
marks it as provider-interpreted ('provider' is the only scope the
schema seeds). Only the key-value forms are ever recorded: a machine
designation resolves into the machine records themselves -- an
existing machine is reused, a container designation creates the
parent and child machine records -- and nothing is stored about the
directive. The write is performed by the machine state's
`PlaceMachine` at machine-creation time, called from the machine
service's `AddMachine`.

(the-placement-directive-states)=
### Placement directive states

Not applicable -- a directive is request input: it is resolved when
the machine record is written and does not exist as state.

### Types of placement directive

The parse taxonomy is record-derived: a directive is parsed into one
of four types -- unset, machine, container, or provider -- derived
from its scope at parse time. The record only ever carries the
provider scope (the key-value forms): the machine and container
types resolve into machine records instead.

(the-placement-directives-machinery)=
## Placement directives in the execution layer

A placement directive has no machinery of its own: it is request
input, consumed where compute is requested and resolved against the
model's machines (a machine or container designation) or handed to
the provider (a key-value form). By the time the command returns,
the directive has been parsed and the machine record(s) exist --
whether the cloud can honour a key-value directive is discovered
later, at provisioning time, when the machine provisioner reads the
stored directive back and asks the cloud for a machine in that
location (see {ref}`machine provisioning <the-machines-machinery>`).
There is no directive entity to update later: a placement is settled
when the machine record is written.

(the-placement-directive-operations)=
### Placement directive operations

The client parses the `--to` value first (an unscoped value such as
`zone=us-east-1a` is sent as provider scope for the model); the
controller resolves it when the machine record is created: an
existing machine is validated against the request's constraints and
reused; a container designation acquires or creates the parent
machine and links the child to it; a key-value form creates the
machine and writes the provider placement record on it, deferred
until the instance is started.

(the-placement-directive-watchers)=
### Placement directive watchers

Not applicable -- nothing watches a directive; the machines it
resolves into have their own.

(the-placement-directive-rules-and-errors)=
## Placement directive rules and errors

```{caution}

When the location is a key-value pair, its availability and meaning may vary from cloud to cloud. For details see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

- a machine designation must satisfy the designation grammar
  (see {ref}`the machine's declaration rules
  <machine-declaration-rules>`);
- the `--to` argument is rejected on Kubernetes models
  (`k8s models do not support placement directives`);
- a malformed directive is rejected at the client (`invalid --to
  parameter`); at the controller an invalid or unresolvable
  directive fails with `invalid placement`, a container type passed
  together with a placement is
  `container type and placement are mutually exclusive`, and a
  provider-scoped value that is not the model's UUID fails with
  `invalid model id`;
- `juju bootstrap` accepts only the unscoped key-value forms; a
  scoped directive fails with
  `unsupported bootstrap placement directive`;
- a key-value directive must name a value the cloud can honour
  (`zone=`, `subnet=`, `system-id=` per the list below); a `zone=`
  directive overrides the `zones` {ref}`constraint <constraint>`.

These rules are enforced where the directive is parsed -- at the
client, then at the controller when the machine record is created;
the cloud-variance is never enforced by Juju, it is discovered at
provisioning time.

(list-of-placement-directive-locations)=
## List of placement directives

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

**Example:** `zone=us-east-1a`
