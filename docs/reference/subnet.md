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

A **subnet** is a range of IP addresses in CIDR notation, such as `10.0.0.0/24`, that can belong to one {ref}`space <space>` of a {ref}`model <model>`. Juju learns subnets from the cloud and from the addresses that machines report. A client can list subnets and move them between spaces.

(subnet-data-model)=
## Subnet data model

```{ggarch}
:file: ../juju.ggarch
:view: Subnet data model
:caption: The model database keeps one record for each subnet. A subnet belongs to at most one space (a space has many subnets), has at most one provider subnet record that holds the cloud's ID for it, and at most one provider network (the cloud's network that contains it, such as a VPC), which holds many subnets. A box with a second, outlined arrow mark in the corner is a record that another page describes.
:alt: Five boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Subnet is in the middle. Space is to its left, joined by a dashed line (optional, one space to many subnets), with an outlined arrow mark for a record that another page describes. Provider subnet is above Subnet (1 to 1). Subnet's provider network is below Subnet (1 to 1), and Provider network is to its right (1 to m).
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
- **Space:** A space that holds subnets. Another page owns it: see {ref}`space <space>`.

A subnet is a row in the table `subnet`, and its cloud ID is a row in `provider_subnet`. Its provider network is linked by a row in `provider_network_subnet` to a row in `provider_network`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`. The {ref}`availability zones <zone>` in which a subnet is available are linked to it by their own records.

(subnet-change-streams)=
## Subnet change streams

Subnet change streams are notifications that tell a worker of the controller that a subnet changed. A change to a subnet, including a move to another space, changes the addresses that the ingress rules of {ref}`units <unit>` of applications exposed to that space allow, and the firewaller (the worker that opens and closes ports on the cloud) applies the change. The {ref}`controller workers <controller-workers>` section lists the other workers that follow subnets.

(subnet-operations)=
## Subnet operations

Subnet operations are the actions a {ref}`client <client>` can take on subnets through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): list, find by CIDR, and move to a space. Subnets enter a model when {ref}`spaces are reloaded <space-reloading>` and when machines report their addresses.

(subnet-listing)=
### Listing subnets

```{ibnote}
See also: {ref}`Juju | List subnets <list-subnets>`
```

Listing subnets returns the subnets of a model, optionally filtered by space and zone.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model.
- *Rules:*
  - A filter by space keeps the subnets in that space. A space that does not exist gives an empty list.
  - A filter by zone hides a subnet only if the subnet has zones and none is the one asked for, so a subnet with no zone always passes.
- *Errors:*
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid space tag`**: *Trigger:* A space filter that is not the tag `space-` followed by a space name. The client receives them with no error code. *Remediation:* Use a tag such as `space-db-space`.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator for read access.

(subnet-finding-by-cidr)=
### Finding subnets by CIDR

Finding subnets by CIDR returns, for each CIDR given, the subnets that it matches.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model.
- *Rules:*
  - A CIDR must be in its canonical form: `10.0.0.5/24` is not, and `10.0.0.0/24` is.
  - The answer includes each subnet's UUID, which {ref}`moving subnets <subnet-moving>` needs.
  - The call takes a list of CIDRs. Each CIDR in the list gets its own result.
- *Errors:*
  - **`CIDR "<cidr>" not valid`**: *Trigger:* A CIDR that is not canonical. The client receives it in the result for that CIDR, with the code `not valid`. *Remediation:* Give the network address, such as `10.0.0.0/24`.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator for read access.

(subnet-moving)=
### Moving subnets to spaces

```{ibnote}
See also: {ref}`Juju | Move a subnet to another space <move-a-subnet-to-another-space>`
```

Moving subnets to spaces puts the listed subnets into a space, and refuses the move when it would leave a machine without the addresses that its applications or constraints need.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model. The change block must be off.
- *Rules:*
  - The cloud must support spaces, and Juju must define them: the cloud must not discover them (see {ref}`space operations <space-operations>`).
  - The call takes a list of moves. Each move in the list gets its own result. Each move names at least one subnet by its tag, which is `subnet-` followed by its UUID. Each move also names a destination space by its tag, such as `space-alpha`. The destination must exist.
  - A subnet that is already in the destination is skipped without an error. A UUID that matches no subnet is ignored. The result lists only the subnets that moved.
  - A move is refused unless both of these hold:
    - Every machine keeps an address in each space that the {ref}`bindings <application>` of its applications and its own constraints require.
    - No machine with an address in a moving subnet has a constraint that excludes the destination.
  - The check counts the bindings of the applications with units on the machine and the machine's own constraints. The constraints of the model and of an application are not part of it.
  - A move with the force option set skips the check, logs each failure as a warning, and moves the subnets.
- *Errors:*
  - **`spaces not supported`**: *Trigger:* The cloud does not support spaces. The client receives it with the code `not supported`. *Remediation:* Use a cloud that supports spaces.
  - **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud discovers its spaces. The client receives it with the code `not supported`. *Remediation:* On MAAS, change the space in MAAS and {ref}`reload spaces <space-reloading>`.
  - **`the operation has been blocked`** (or the message set with the block): *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.
  - **`no subnets specified`**: *Trigger:* A move that names no subnet. *Remediation:* Name the subnets.
  - **`"<tag>" is not a valid space tag`** and **`"<tag>" is not a valid tag`**: *Trigger:* A destination that is not the tag `space-` followed by a space name. *Remediation:* Use a tag such as `space-db-space`.
  - **`"<tag>" is not a valid subnet tag`**, **`"<tag>" is not a valid tag`**, and **`invalid uuid "<id>"`**: *Trigger:* A subnet that is not given as the tag `subnet-` followed by its UUID. *Remediation:* {ref}`Find the subnets by CIDR <subnet-finding-by-cidr>` and use their UUIDs.
  - **`moving subnets to space "<name>": getting space "<name>": space not found with <name>: space not found`**: *Trigger:* The destination does not exist. *Remediation:* Add the space first.
  - **`moving subnets to space "<name>": machine "<id>" is missing addresses in space(s) "<space>"`** and **`moving subnets to space "<name>": machine "<id>" would have <n> addresses in excluded space "<name>" (<addresses>)`**: *Trigger:* The move would leave a machine without the addresses it needs, or give it addresses in a space that a constraint excludes. *Remediation:* Change the constraints or bindings that need the addresses, or force the move.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator for admin access.

(subnet-migration)=
## Subnets and model migration

A {ref}`migration <model-migrating>` copies the spaces and subnets of a model to the target controller, except `alpha`, which every model already has. Juju links each subnet to its space through the space's former ID. An address whose subnet is unknown gets a new subnet in the `alpha` space and a warning in the log, so reassign it afterward. A model on a Kubernetes cloud arrives with only the two catch-all subnets, `0.0.0.0/0` and `::/0`.
