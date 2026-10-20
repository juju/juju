---
myst:
  html_meta:
    description: "Juju network space reference: what a space, a subnet, and an availability zone are, the records the model keeps about them, and the operations that list, reload, add, rename, move subnets between, and remove spaces."
---

(space)=
# Space

```{ibnote}
See also: {ref}`Juju | Manage spaces <manage-spaces>`, {ref}`Juju | Manage subnets <manage-subnets>`
```

A **network space** is a named group of {ref}`subnets <subnet>` (ranges of IP addresses) that can communicate with one another, which a {ref}`model <model>` uses to segment network traffic for performance, security, and regulatory compliance. Every model has a space named `alpha` that holds each discovered subnet until a user moves it, and users add, rename, and remove the other spaces. On a MAAS cloud, Juju copies the cloud's own spaces into the model.

(space-data-model)=
## Space data model

```{ggarch}
:file: ../juju.ggarch
:view: Space data model
:caption: The model database keeps one record for each space and one for each subnet. A subnet belongs to at most one space (a space has many subnets), and a space that comes from the cloud has one provider space record that holds the cloud's ID for it. A subnet has at most one provider subnet record that holds the cloud's ID for it, at most one provider network (the cloud's network that contains it, such as a VPC), and any number of availability zones, through a zone record for each. A provider network and an availability zone each hold many subnets. All boxes are records.
:alt: Eight boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Subnet is in the middle. Space is to its left, joined through a dashed line because the space is optional (1 to m). Provider subnet is above Subnet (1 to 1), and Provider space is above Space (1 to 1). Subnet's zone is to the right of Subnet (m to 1), and Availability zone is above Subnet's zone (1 to m). Subnet's provider network is below Subnet (1 to 1), and Provider network is to its right (1 to m).
```

The space data model is the set of records that the {ref}`model database <database>` keeps about spaces, subnets, and availability zones. The records, with the rules that apply to each, are:

- **Space:** A space's name. The `alpha` space exists from the creation of the model.
  - *Rules:*
    - A name appears once.
- **Provider space:** The ID that the cloud gave a space.
  - *Rules:*
    - A cloud ID appears once, and a space has at most one.
- (subnet)=
  **Subnet:** A range of IP addresses in CIDR notation, such as `10.0.0.0/24`, with its VLAN tag and its space. Juju learns subnets from the cloud and from the addresses that machines report.
  - *Rules:*
    - A subnet belongs to at most one space.
- **Provider subnet:** The ID that the cloud gave a subnet.
  - *Rules:*
    - A cloud ID appears once, is not empty, and a subnet has at most one.
- **Provider network:** The ID of a cloud network, such as a VPC, that contains subnets.
  - *Rules:*
    - A cloud ID appears once.
- **Subnet's provider network:** The link between a subnet and its provider network.
  - *Rules:*
    - A subnet has at most one.
- (zone)=
  **Availability zone:** A named failure domain of a cloud, such as `us-east-1a`, in which a subnet is available. A user can name zones to say where Juju provisions hardware, so that an outage in one zone does not take everything down: the {ref}`zones <constraint-zones>` constraint takes several, the {ref}`zone <placement-directive-zone>` placement directive takes one, and the directive overrides the constraint.
  - *Rules:*
    - A name appears once.
- **Subnet's zone:** The link that says a subnet is available in a zone.
  - *Rules:*
    - A subnet is linked to a zone once.

A space is a row in the table `space`, and a cloud's ID for it is a row in `provider_space`. A subnet is a row in `subnet`, its cloud ID is a row in `provider_subnet`, its provider network is linked by a row in `provider_network_subnet` to a row in `provider_network`, and its zones are linked by rows in `availability_zone_subnet` to rows in `availability_zone`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`. Applications refer to a space by its UUID and constraints refer to it by name, and those records belong to the {ref}`application <application>` and {ref}`constraint <constraint>` pages.

(space-change-streams)=
## Space change streams

Space change streams are the changes that the workers of a model and of the controller follow, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). No client can watch spaces or subnets, but the firewaller follows the subnets of a model and a controller worker follows spaces and subnets:

- **Subnets:** The firewaller follows the subnets of the model. When a subnet changes, including when it moves to another space, the firewaller reloads the spaces and works out again the ingress rules of the units of applications that are exposed to spaces.
- **Spaces and subnets:** On the controller, the `api-address-setter` worker follows the spaces and subnets (see {ref}`controller workers <controller-workers>`).

(space-operations)=
## Space operations

Space operations are the actions a {ref}`client <client>` can take on spaces, subnets, and availability zones through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): list, show, reload, add, rename, move subnets, and remove. A model on a Kubernetes cloud allows only listing and showing spaces and reloading them.

The operations that change spaces need a cloud that supports them. The clouds that support spaces are MAAS, EC2, OpenStack, GCE, Azure, LXD (when the LXD server offers the `network` API extension), and manual clouds. Kubernetes, OCI, and VMware vSphere clouds do not support spaces. On MAAS the cloud owns the spaces: Juju copies them, and refuses to rename, move subnets of, or remove them. On every other cloud the operator manages spaces in Juju.

(space-listing)=
### Listing spaces

```{ibnote}
See also: {ref}`Juju | View available spaces <view-available-spaces>`, {ref}`Juju | View details about a space <view-details-about-a-space>`, {ref}`Juju | List subnets <list-subnets>`
```

Listing spaces returns the spaces of a model with their subnets, shows one space with the applications bound to it and a count of its machines, lists subnets with a filter on space and zone, returns the subnets that match given CIDRs, and returns the availability zones that the cloud reports.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The list includes `alpha`, and its order is not fixed.
  - A space's applications are those with an endpoint or extra binding in the space, and those whose default space it is and that have an endpoint. The spaces to which an application is exposed are not counted.
  - A space's machine count is the number of distinct network nodes with an address in one of its subnets, so a machine and its containers count by their own nodes.
  - A subnet filter by space names the space, and a space that does not exist gives an empty list. A filter by zone hides a subnet only if the subnet has zones and none is the one asked for.
  - A CIDR given to match must be in its canonical form: `10.0.0.5/24` is not, and `10.0.0.0/24` is.
  - The availability zones come live from the cloud, not from the model database, and a cloud that cannot report zones gives an empty list. Kubernetes, Azure, and manual clouds report none.
- *Errors:* The client receives each with no error code unless it is given:
  - **`"<name>" is not a valid tag`** and **`"<name>" is not a valid space tag`**: *Trigger:* A space name that is not a UUID or lower-case words joined by hyphens. *Remediation:* Use a name such as `db-space`.
  - **`fetching space "<name>": space not found with <name>: space not found`**: *Trigger:* Showing a space that does not exist. *Remediation:* List the spaces and use an existing name.
  - **`CIDR "<cidr>" not valid`**: *Trigger:* A CIDR that is not canonical. The client receives it with the code `not valid`. *Remediation:* Give the network address, such as `10.0.0.0/24`.
  - **`getting provider availability zones: <reason>`**: *Trigger:* The cloud refused or failed the request for zones. *Remediation:* Fix the credential or the connection to the cloud.
  - **`permission denied`**: *Trigger:* The caller lacks read access. *Remediation:* Ask a model administrator.

(space-reloading)=
### Reloading spaces

```{ibnote}
See also: {ref}`Juju | Reload spaces <reload-spaces>`
```

Reloading spaces reads the spaces and subnets from the cloud again and stores them in the model. Juju does it by itself when it bootstraps a controller and when it creates a model, and a user does it when the cloud's network has changed.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, and the change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - On a cloud that discovers spaces (only MAAS), Juju adds each space that the cloud reports, with a name made of the cloud's name in lower case, with spaces turned into hyphens, characters other than letters, digits, and hyphens removed, and a number added after a hyphen if the name is taken. Juju removes spaces that have a cloud ID and that the cloud no longer reports, unless something still depends on them, and never removes `alpha`.
  - On every other cloud that supports networking, Juju reads the subnets only, and puts every subnet it has not seen into `alpha`. It skips multicast and link-local addresses.
  - A cloud reports subnets as follows: EC2 reports the subnets of the VPC that the model uses, or of the default VPC. OpenStack reports its Neutron subnets, Azure the subnets of the virtual network, and GCE reports every subnet in every zone. OCI creates networks and subnets if there are none. LXD reports its networks. A Kubernetes cloud reports the two catch-all ranges `0.0.0.0/0` and `::/0` when it cannot find subnets.
  - Reloading never deletes a subnet.
  - A subnet that the model already knows by its cloud ID is not stored again, and a second reload does not apply a change of space or of zones to it.
- *Errors:* The client receives each with the code given:
  - **`spaces discovery in a non-networking environ not supported`**: *Trigger:* The cloud does not offer networking, as on VMware vSphere. The client receives it with the code `not supported`. *Remediation:* None; the cloud offers no spaces.
  - **`subnets not supported`**: *Trigger:* A manual cloud. The client receives it with the code `not supported`. *Remediation:* None; a manual cloud offers no subnet discovery.
  - The cloud's own message: *Trigger:* The cloud refuses or fails the request. *Remediation:* Fix the credential or the connection to the cloud, and reload again.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

(space-adding)=
### Adding spaces

```{ibnote}
See also: {ref}`Juju | Add a space <add-a-space>`
```

Adding spaces creates spaces and puts the subnets with the given CIDRs into them.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, and the change block must not be on.
- *Rules:*
  - The cloud must support spaces. A MAAS cloud is allowed.
  - A request can carry a batch of spaces, and the answer has one result for each. A space that was created stays when a later one fails.
  - The name must be a UUID or lower-case letters and digits in words joined by hyphens, and must not exist yet.
  - Each CIDR must be canonical and must match a subnet that the model knows. Every subnet with that CIDR moves to the new space, even from another space, and nothing checks the machines, constraints, or bindings that depended on its old space.
  - A request can carry the cloud's ID for the space.
- *Errors:*
  - **`spaces not supported`**: *Trigger:* The cloud does not support spaces. The client receives it with the code `not supported`. *Remediation:* None; the cloud offers no spaces.
  - **`"<name>" is not a valid tag`**: *Trigger:* The name is not valid. *Remediation:* Use lower-case words joined by hyphens.
  - **`space name "<name>" not valid`**: *Trigger:* The name fails the check made when the space is stored. The client receives it with the code `not valid`. *Remediation:* Use lower-case words joined by hyphens.
  - **`"<cidr>" is not a valid CIDR`**: *Trigger:* The CIDR is not canonical. *Remediation:* Give the network address, such as `10.0.0.0/24`.
  - **`space "<name>": space already exists`**: *Trigger:* A space has the name. *Remediation:* Choose another name, or rename or remove the existing space.
  - **`subnet "<cidr>": subnet not found`**: *Trigger:* The model knows no subnet with the CIDR. *Remediation:* {ref}`List the subnets <space-listing>`, or {ref}`reload spaces <space-reloading>`.
  - **`operation is blocked`** with the operator's message: *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

(space-renaming)=
### Renaming spaces

```{ibnote}
See also: {ref}`Juju | Rename a space <rename-a-space>`
```

Renaming spaces gives spaces new names.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, and the change block must not be on.
- *Rules:*
  - The cloud must support spaces, and the space must not come from the cloud (on MAAS, rename the space in MAAS and {ref}`reload spaces <space-reloading>`).
  - A space named `alpha` cannot be renamed, and the new name must not be in use, which includes `alpha`.
  - A space that a constraint names cannot be renamed: the database refuses the change. Neither the `default-space` model configuration key nor the `juju-mgmt-space` controller configuration key changes with the name.
- *Errors:*
  - **`spaces not supported`** and **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud does not support spaces, or the space comes from the cloud. The client receives them with the code `not supported`. *Remediation:* On MAAS, change the space in MAAS and reload.
  - **`the "alpha" space cannot be renamed`**: *Trigger:* The request names `alpha`. *Remediation:* None; `alpha` keeps its name.
  - **`space "<name>" already exists`**: *Trigger:* The new name is in use. The client receives it with the code `already exists`. *Remediation:* Choose another name.
  - **`retrieving space "<name>": space not found with <name>: space not found`**: *Trigger:* No space has the name. *Remediation:* List the spaces and use an existing name.
  - **`updating space "<name>": <reason>`** with a foreign key failure: *Trigger:* A constraint names the space. *Remediation:* Remove the constraints that name the space, or change them, and rename again.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

(space-moving)=
### Moving subnets to spaces

```{ibnote}
See also: {ref}`Juju | Move a subnet to another space <move-a-subnet-to-another-space>`
```

Moving subnets to spaces puts the listed subnets into a space, and refuses the move when it would leave a machine without the addresses that its applications or constraints need.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, and the change block must not be on.
- *Rules:*
  - The cloud must support spaces, and the spaces must not come from the cloud.
  - A request can carry several moves, each with its own result, and each names at least one subnet by its UUID and a destination space, which can be `alpha`. The destination must exist.
  - A subnet that is already in the destination is skipped without an error.
  - A move is refused unless every machine keeps an address in each space that its applications are bound to and that its own constraints require, and no machine with an address in a moving subnet has a constraint that excludes the destination. The check counts the bindings of the applications with units on the machine and the machine's own constraints, and does not count the constraints of the model or of an application.
  - A request can force the move, which logs the failures of the check and moves the subnets.
- *Errors:*
  - **`spaces not supported`** and **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud does not support spaces, or the spaces come from the cloud. The client receives them with the code `not supported`. *Remediation:* On MAAS, change the space in MAAS and reload.
  - **`no subnets specified`**: *Trigger:* A move that names no subnet. *Remediation:* Name the subnets.
  - **`"<name>" is not a valid subnet tag`** and **`invalid uuid "<id>"`**: *Trigger:* A subnet that is not named by its UUID. *Remediation:* {ref}`List the subnets <space-listing>` and use their UUIDs.
  - **`moving subnets to space "<name>": getting space "<name>": space not found with <name>: space not found`**: *Trigger:* The destination does not exist. *Remediation:* Add the space first.
  - **`moving subnets to space "<name>": machine "<id>" is missing addresses in space(s) "<space>"`** and **`moving subnets to space "<name>": machine "<id>" would have <n> addresses in excluded space "<name>" (<addresses>)`**: *Trigger:* The move would leave a machine without the addresses it needs, or give it addresses in a space that a constraint excludes. *Remediation:* Change the constraints or bindings that need the addresses, or force the move.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

(space-removing)=
### Removing spaces

```{ibnote}
See also: {ref}`Juju | Remove a space <remove-a-space>`
```

Removing spaces deletes spaces and returns what depended on them to `alpha`.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, and the change block must not be on.
- *Rules:*
  - The cloud must support spaces, and the spaces must not come from the cloud.
  - The space named `alpha` cannot be removed.
  - A request can ask for a dry run, which reports what depends on the space and removes nothing, and a request that names the space that the controller configuration key `juju-mgmt-space` holds is always a dry run.
  - A space on which something depends is not removed unless the request forces it. The dependants are a model constraint on the space, an application constraint on it, and an application bound to it, through its default space, an endpoint or extra binding, or the spaces to which it is exposed. Dependants are not an error: the answer lists them, as application names, and the model for a model constraint.
  - A removal deletes the constraints on the space, sets the default space of the applications that used it and the spaces they are exposed to to `alpha`, sets the endpoint and extra bindings to the application's default, moves the subnets of the space to `alpha`, and deletes the space.
  - The `default-space` model configuration key and the `juju-mgmt-space` controller configuration key are not changed: a name that no longer exists leaves the default at `alpha`.
- *Errors:*
  - **`spaces not supported`** and **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud does not support spaces, or the spaces come from the cloud. The client receives them with the code `not supported`. *Remediation:* On MAAS, change the space in MAAS and reload.
  - **`the "alpha" space cannot be removed`**: *Trigger:* The request names `alpha`. *Remediation:* None.
  - **`space not found`**: *Trigger:* No space has the name. *Remediation:* List the spaces and use an existing name.
  - **`removing space "<name>": <reason>`**: *Trigger:* The database refused the change. One case is a forced removal of an application that is exposed to both this space and `alpha` at the same endpoint. *Remediation:* Remove the exposure to one of the two, and retry.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

## Spaces and model migration

A migration copies the spaces, subnets, and availability zones of a model to the target controller with the model, except `alpha`, which every model has already. A model that arrives from a 3.x controller gets its spaces without subnets, linked by ID, and a model on a Kubernetes cloud gets only the catch-all subnets.
