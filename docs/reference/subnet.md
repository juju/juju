---
myst:
  html_meta:
    description: "Juju subnet reference: what a subnet is, the records the model keeps about it, the changes that follow it, and the operations that list subnets and move them between spaces."
---

(subnet)=
# Subnet

```{ibnote}
See also: {ref}`Juju | Manage subnets <manage-subnets>`
```

A **subnet** is a range of IP addresses in CIDR notation, such as `10.0.0.0/24`, that belongs to one {ref}`space <space>` of a {ref}`model <model>`. Juju learns subnets from the cloud and from the addresses that machines report, and no operation adds or removes one: a client can only list subnets and move them between spaces.

(subnet-data-model)=
## Subnet data model

```{ggarch}
:file: ../juju.ggarch
:view: Subnet data model
:caption: The model database keeps one record for each subnet. A subnet belongs to at most one space (a space has many subnets), has at most one provider subnet record that holds the cloud's ID for it, and at most one provider network (the cloud's network that contains it, such as a VPC), which holds many subnets. The space is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Five boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Subnet is in the middle. Space is to its left, joined through a dashed line because the space is optional (1 to m), with an outlined arrow mark for a record that another page describes. Provider subnet is above Subnet (1 to 1). Subnet's provider network is below Subnet (1 to 1), and Provider network is to its right (1 to m).
```

The subnet data model is the set of records that the {ref}`model database <database>` keeps about subnets. The records, with the rules that apply to each, are:

- **Subnet:** A subnet's CIDR, its VLAN tag, and its space.
  - *Rules:*
    - A subnet belongs to at most one space.
- **Provider subnet:** The ID that the cloud gave a subnet.
  - *Rules:*
    - A cloud ID appears once and is not empty, and a subnet has at most one.
- **Provider network:** The ID of a cloud network, such as a VPC, that contains subnets.
  - *Rules:*
    - A cloud ID appears once.
- **Subnet's provider network:** The link between a subnet and its provider network.
  - *Rules:*
    - A subnet has at most one.
- **Space:** The record at the far end of a subnet's space. Another page owns it: see {ref}`space <space>`.

A subnet is a row in the table `subnet`, and its cloud ID is a row in `provider_subnet`. Its provider network is linked by a row in `provider_network_subnet` to a row in `provider_network`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`. The {ref}`availability zones <zone>` in which a subnet is available are linked to it by their own records.

(subnet-change-streams)=
## Subnet change streams

Subnet change streams are the changes that the workers of a model and of the controller follow, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). No client can watch subnets, but the firewaller (the worker that opens and closes ports on the cloud) follows the subnets of a model, and when one changes, including when it moves to another space, it works out again the ingress rules of the units of applications that are exposed to spaces. One controller worker, `api-address-setter`, follows spaces and subnets (see {ref}`controller workers <controller-workers>`).

(subnet-operations)=
## Subnet operations

Subnet operations are the actions a {ref}`client <client>` can take on subnets through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): list, find by CIDR, and move to a space. Subnets enter a model when {ref}`spaces are reloaded <space-reloading>` and when machines report their addresses. Machines report an address that no known subnet contains by creating a subnet in `alpha`, but only for a manually provisioned machine or for a single-address range (`/32` or `/128`).

(subnet-listing)=
### Listing subnets

```{ibnote}
See also: {ref}`Juju | List subnets <list-subnets>`
```

Listing subnets returns the subnets of a model, optionally filtered by space and zone, and returns the subnets that match given CIDRs.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - A filter by space names the space, and a space that does not exist gives an empty list.
  - A filter by zone hides a subnet only if the subnet has zones and none is the one asked for, so a subnet with no zone always passes.
  - A CIDR given to match must be in its canonical form: `10.0.0.5/24` is not, and `10.0.0.0/24` is. The answer includes each subnet's UUID, which {ref}`moving subnets <subnet-moving>` needs.
  - The order of the list is not fixed.
- *Errors:*
  - **`"<name>" is not a valid tag`**: *Trigger:* A space filter that is not a UUID or lower-case words joined by hyphens. The client receives it with no error code. *Remediation:* Use a name such as `db-space`.
  - **`CIDR "<cidr>" not valid`**: *Trigger:* A CIDR that is not canonical. The client receives it with the code `not valid`. *Remediation:* Give the network address, such as `10.0.0.0/24`.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.

(subnet-moving)=
### Moving subnets to spaces

```{ibnote}
See also: {ref}`Juju | Move a subnet to another space <move-a-subnet-to-another-space>`
```

Moving subnets to spaces puts the listed subnets into a space, and refuses the move when it would leave a machine without the addresses that its applications or constraints need.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, and the change block must not be on.
- *Rules:*
  - The cloud must support spaces, and the spaces must not come from the cloud (see {ref}`space operations <space-operations>`).
  - A request can carry several moves, each with its own result, and each names at least one subnet by its UUID and a destination space, which can be `alpha`. The destination must exist.
  - A subnet that is already in the destination is skipped without an error.
  - A move is refused unless every machine keeps an address in each space that its applications are bound to and that its own constraints require, and no machine with an address in a moving subnet has a constraint that excludes the destination. The check counts the bindings of the applications with units on the machine and the machine's own constraints, and does not count the constraints of the model or of an application.
  - A request can force the move, which logs the failures of the check and moves the subnets.
- *Errors:*
  - **`spaces not supported`** and **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud does not support spaces, or the spaces come from the cloud. The client receives them with the code `not supported`. *Remediation:* On MAAS, change the space in MAAS and reload.
  - **`no subnets specified`**: *Trigger:* A move that names no subnet. *Remediation:* Name the subnets.
  - **`"<name>" is not a valid subnet tag`** and **`invalid uuid "<id>"`**: *Trigger:* A subnet that is not named by its UUID. *Remediation:* {ref}`List the subnets <subnet-listing>` and use their UUIDs.
  - **`moving subnets to space "<name>": getting space "<name>": space not found with <name>: space not found`**: *Trigger:* The destination does not exist. *Remediation:* Add the space first.
  - **`moving subnets to space "<name>": machine "<id>" is missing addresses in space(s) "<space>"`** and **`moving subnets to space "<name>": machine "<id>" would have <n> addresses in excluded space "<name>" (<addresses>)`**: *Trigger:* The move would leave a machine without the addresses it needs, or give it addresses in a space that a constraint excludes. *Remediation:* Change the constraints or bindings that need the addresses, or force the move.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

## Subnets and model migration

A migration copies the subnets of a model to the target controller with the model. A model that arrives from a 3.x controller gets its spaces without CIDRs, linked by ID, and a subnet for each address whose CIDR is unknown, and a model on a Kubernetes cloud gets only the two catch-all subnets, `0.0.0.0/0` and `::/0`.
