---
myst:
  html_meta:
    description: "Juju network space reference: what a space is, the records the model keeps about it, and the operations that list, reload, add, rename, and remove spaces."
---

(space)=
# Space

```{ibnote}
See also: {ref}`Juju | Manage spaces <manage-spaces>`
```

A **network space** is a named group of {ref}`subnets <subnet>` that can communicate with one another, which a {ref}`model <model>` uses to segment network traffic for performance, security, and regulatory compliance. Every model has a space named `alpha` that holds each discovered subnet until a user moves it, and users add, rename, and remove the other spaces. On a MAAS cloud, Juju copies the cloud's own spaces into the model.

(space-data-model)=
## Space data model

```{ggarch}
:file: ../juju.ggarch
:view: Space data model
:caption: The model database keeps one record for each space. A space that comes from the cloud has one provider space record that holds the cloud's ID for it. A subnet belongs to at most one space, and a space has many subnets. The subnet is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Three boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Space is at the bottom left, Provider space is above it (1 to 1), and Subnet is to the right of Space, joined through a dashed line because the space is optional (1 to m), with an outlined arrow mark for a record that another page describes.
```

The space data model is the set of records that the {ref}`model database <database>` keeps about spaces. The records, with the rules that apply to each, are:

- **Space:** A space's name. The `alpha` space exists from the creation of the model.
  - *Rules:*
    - A name appears once.
- **Provider space:** The ID that the cloud gave a space.
  - *Rules:*
    - A cloud ID appears once, and a space has at most one.
- **Subnet:** The record at the far end of a space's subnets. Another page owns it: see {ref}`subnet <subnet>`.

A space is a row in the table `space`, and a cloud's ID for it is a row in `provider_space`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`. Applications refer to a space by its UUID and constraints refer to it by name, and those records belong to the {ref}`application <application>` and {ref}`constraint <constraint>` pages.

(space-change-streams)=
## Space change streams

Space change streams are the changes that a worker of the controller follows, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). No client can watch spaces, and one controller worker, `api-address-setter`, follows them together with subnets (see {ref}`controller workers <controller-workers>`).

(space-operations)=
## Space operations

Space operations are the actions a {ref}`client <client>` can take on spaces through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): list, show, reload, add, rename, move subnets, and remove. Listing subnets and moving them are described on the {ref}`subnet <subnet>` page, and listing availability zones on the {ref}`zone <zone>` page. A model on a Kubernetes cloud allows only listing and showing spaces and reloading them.

The operations that change spaces need a cloud that supports them. The clouds that support spaces are MAAS, EC2, OpenStack, GCE, Azure, LXD (when the LXD server offers the `network` API extension), and manual clouds. Kubernetes, OCI, and VMware vSphere clouds do not support spaces. On MAAS the cloud owns the spaces: Juju copies them, and refuses to rename or remove them. On every other cloud the operator manages spaces in Juju.

(space-listing)=
### Listing spaces

```{ibnote}
See also: {ref}`Juju | View available spaces <view-available-spaces>`, {ref}`Juju | View details about a space <view-details-about-a-space>`
```

Listing spaces returns the spaces of a model with their subnets, and shows one space with the applications bound to it and a count of its machines.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The list includes `alpha`, and its order is not fixed.
  - A space's applications are those with an endpoint or extra binding in the space, and those whose default space it is and that have an endpoint. The spaces to which an application is exposed are not counted.
  - A space's machine count is the number of distinct network nodes with an address in one of its subnets, so a machine and its containers count by their own nodes.
- *Errors:* The client receives each with no error code:
  - **`"<name>" is not a valid tag`** and **`"<name>" is not a valid space tag`**: *Trigger:* A space name that is not a UUID or lower-case words joined by hyphens. *Remediation:* Use a name such as `db-space`.
  - **`fetching space "<name>": space not found with <name>: space not found`**: *Trigger:* Showing a space that does not exist. *Remediation:* List the spaces and use an existing name.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.

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

A migration copies the spaces of a model to the target controller with the model, except `alpha`, which every model has already. A model that arrives from a 3.x controller gets its spaces without subnets, linked by ID.
