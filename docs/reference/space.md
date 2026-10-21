---
myst:
  html_meta:
    description: "Juju network space reference: what a space is, the records the model keeps about it, and the calls that list, reload, add, rename, and remove spaces."
---

(space)=
# Space

```{ibnote}
See also: {ref}`Juju | Manage spaces <manage-spaces>`
```

A **network space** is a named group of {ref}`subnets <subnet>` that can communicate with one another, which a {ref}`model <model>` uses to segment network traffic for performance, security, and regulatory compliance. Every model has a space named `alpha` that holds each discovered subnet until a user moves it, and users add, rename, and remove the other spaces. On a MAAS cloud, Juju copies the cloud's own spaces into the model.

(space-database-schema)=
## Space database schema

```{ggarch}
:file: ../juju.ggarch
:view: Space database schema
:caption: The model database keeps one record for each space. A space that comes from the cloud has one provider space record that holds the cloud's ID for it. A subnet belongs to at most one space, and a space has many subnets. The subnet is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Three boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Space is at the bottom left, Provider space is above it (a provider space belongs to one space, and a space has at most one provider space), and Subnet is to the right of Space, joined through a dashed line (a subnet belongs to at most one space, and a space has many subnets), with an outlined arrow mark for a record that another page describes.
```

The space database schema is the set of records that the {ref}`model database <database>` keeps about spaces. The records, with the rules that apply to each, are:

- **Space:** A space's name. The `alpha` space exists from the creation of the model.
  - *Rules:*
    - A name appears once.
- **Provider space:** The ID that the cloud gave a space.
  - *Rules:*
    - A cloud ID appears once.
    - A space has at most one cloud ID.
- **Subnet:** A subnet that the space holds. Another page describes it: see {ref}`subnet <subnet>`.
  - *Rules:*
    - A subnet belongs to at most one space.

(space-calls)=
## Space calls

Space calls are the calls a {ref}`client <client>` can take on spaces through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): list, show, reload, add, rename, move subnets, and remove. Listing subnets and moving them are described on the {ref}`subnet <subnet>` page, and listing availability zones on the {ref}`zone <zone>` page. A model on a Kubernetes cloud allows only listing, showing, and reloading spaces: any other call fails with `facade method "Spaces.<method>" not supported on container models` and the code `not supported`.

The calls that change spaces need a cloud that supports them. The clouds that support spaces are MAAS, EC2, OpenStack, GCE, Azure, LXD (when the LXD server offers the `network` API extension), and manual clouds. OCI and VMware vSphere clouds do not support spaces. On MAAS the cloud owns the spaces: Juju copies them, and refuses to rename or remove them. On every other cloud the operator manages spaces in Juju.

(space-listing)=
### Listing spaces

```{ibnote}
See also: {ref}`Juju | View available spaces <view-available-spaces>`, {ref}`Juju | View details about a space <view-details-about-a-space>`
```

Listing spaces returns the spaces of a model with their subnets, and shows one space with the applications bound to it and a count of its machines.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The list includes `alpha`.
  - Showing spaces takes a list of spaces. Each space in the list gets its own result.
  - A space's applications are those with an endpoint or extra binding in the space. Also included are those whose default space it is and that have an endpoint. The spaces to which an application is exposed are not counted.
  - A space's machine count is the number of distinct network nodes with an address in one of its subnets, so a machine and its containers count by their own nodes.
- *Errors:* The client receives these errors, and only `permission denied` carries a code:
  - **`"<name>" is not a valid tag`** and **`"<name>" is not a valid space tag`**: *Trigger:* The request does not carry a space tag, or the tag's name is neither a UUID version 7 nor lower-case letters and digits in words joined by hyphens. *Remediation:* Use a tag with a name such as `db-space`.
  - **`fetching space "<name>": space not found with <name>: space not found`**: *Trigger:* Showing a space that does not exist. *Remediation:* List the spaces and use an existing name.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.

(space-reloading)=
### Reloading spaces

```{ibnote}
See also: {ref}`Juju | Reload spaces <reload-spaces>`
```

Reloading spaces reads the spaces and subnets from the cloud again and stores them in the model. Juju does it by itself when it bootstraps a controller and when it creates a model. A user does it when the cloud's network has changed.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`admin access <user-access-model-admin>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - On a cloud that discovers spaces (only MAAS), Juju adds each space that the cloud reports. It makes the name from the cloud's name: lower case, spaces turned into hyphens, and characters other than letters, digits, and hyphens removed. It adds a hyphen and a number if the name is taken.
  - On a cloud that discovers spaces, Juju removes a space that has a cloud ID and that the cloud no longer reports, unless something still depends on it. Juju never removes `alpha`.
  - On every other cloud that supports networking, Juju reads the subnets only, and puts every subnet it has not seen into `alpha`. It skips multicast and link-local addresses.
  - A cloud reports subnets as follows: EC2 reports the subnets of the virtual private cloud (VPC) that the model uses, or of the default VPC. OpenStack reports its Neutron subnets, Azure the subnets of the virtual network, and GCE reports the subnets of the model's VPC, or of every network when the model has none, and treats each as available in every zone. OCI creates networks and subnets if there are none. LXD reports its networks. A Kubernetes cloud reports the two catch-all ranges `0.0.0.0/0` and `::/0` when it cannot find subnets.
  - Reloading never deletes a subnet.
  - A reload that meets a subnet the model already knows by its cloud ID stores none of the subnets of that reload, because the subnets are stored in one transaction that the duplicate cloud ID aborts. A later reload does not apply a change of space or of zones.
- *Errors:* The client receives these errors, each with the code given:
  - **`spaces discovery in a non-networking environ not supported`**: *Trigger:* The cloud does not offer networking, as on VMware vSphere. The client receives it with the code `not supported`. *Remediation:* Use a cloud that offers networking.
  - **`subnets not supported`**: *Trigger:* A manual cloud. The client receives it with the code `not supported`. *Remediation:* Use a cloud that supports subnet discovery.
  - The cloud's own message: *Trigger:* The cloud refuses or fails the request. *Remediation:* Fix the credential or the connection to the cloud, and reload again.
  - **`operation is blocked`** with the operator's message: *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

(space-adding)=
### Adding spaces

```{ibnote}
See also: {ref}`Juju | Add a space <add-a-space>`
```

Adding spaces creates spaces and puts the subnets with the given CIDRs into them.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`admin access <user-access-model-admin>` to the model. The change block must not be on.
- *Rules:*
  - The cloud must support spaces. Unlike renaming and removing, adding is allowed on MAAS.
  - The call takes a list of spaces. Each space in the list gets its own result. A space that was created stays when a later one fails.
  - The name must be a UUID version 7 or lower-case letters and digits in words joined by hyphens, and must not exist yet.
  - Each CIDR (a network range such as `10.0.0.0/24`) must be canonical and must match a subnet that the model knows. Every subnet with that CIDR moves to the new space, including subnets that belong to another space.
  - A request can include the cloud's ID for the space.
- *Errors:*
  - **`spaces not supported`**: *Trigger:* The cloud does not support spaces. The client receives it with the code `not supported`. *Remediation:* Use a cloud that supports spaces.
  - **`"<name>" is not a valid tag`** and **`"<name>" is not a valid space tag`**: *Trigger:* The request does not carry a space tag, or the tag's name is neither a UUID version 7 nor lower-case letters and digits in words joined by hyphens. *Remediation:* Use a tag with a name such as `db-space`.
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

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`admin access <user-access-model-admin>` to the model. The change block must not be on.
- *Rules:*
  - The call takes a list of spaces. Each space in the list gets its own result.
  - The cloud must support spaces. The cloud must not discover its spaces (on MAAS, rename the space in MAAS and {ref}`reload spaces <space-reloading>`).
  - A space named `alpha` cannot be renamed.
  - The new name must not be in use, which includes `alpha`.
  - A space that a constraint names cannot be renamed: the database refuses the change. A rename does not change the `default-space` model configuration key or the `juju-mgmt-space` controller configuration key.
- *Errors:*
  - **`spaces not supported`**: *Trigger:* The cloud does not support spaces. The client receives it with the code `not supported`. *Remediation:* Use a cloud that supports spaces.
  - **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud discovers its spaces. The client receives it with the code `not supported`. *Remediation:* Change the space in MAAS, and {ref}`reload spaces <space-reloading>`.
  - **`"<name>" is not a valid tag`** and **`"<name>" is not a valid space tag`**: *Trigger:* The request does not carry a space tag, or the tag's name is neither a UUID version 7 nor lower-case letters and digits in words joined by hyphens. *Remediation:* Use a tag with a name such as `db-space`.
  - **`the "alpha" space cannot be renamed`**: *Trigger:* The request names `alpha`. *Remediation:* Leave `alpha` as it is.
  - **`space "<name>" already exists`**: *Trigger:* The new name is in use. The client receives it with the code `already exists`. *Remediation:* Choose another name.
  - **`retrieving space "<name>": space not found with <name>: space not found`**: *Trigger:* No space has the name. *Remediation:* List the spaces and use an existing name.
  - **`updating space "<name>": <reason>`** with a foreign key failure: *Trigger:* A constraint names the space. *Remediation:* Remove the constraints that name the space, or change them, and rename again.
  - **`operation is blocked`** with the operator's message: *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

(space-removing)=
### Removing spaces

```{ibnote}
See also: {ref}`Juju | Remove a space <remove-a-space>`
```

Removing spaces deletes spaces and returns what depended on them to `alpha`.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`admin access <user-access-model-admin>` to the model. The change block must not be on.
- *Rules:*
  - The call takes a list of spaces. Each space in the list gets its own result. A database failure ends the whole request. The spaces after it are not processed.
  - The cloud must support spaces. The cloud must not discover its spaces.
  - The space named `alpha` cannot be removed.
  - A request can ask for a dry run, which reports what depends on the space and removes nothing. A request that names the space held by the controller configuration key `juju-mgmt-space` is always a dry run. The answer names that key.
  - A space on which something depends is not removed unless the request forces it. The dependants are a model constraint on the space, an application constraint on it, and an application bound to it, through its default space, an endpoint or extra binding, or the spaces to which it is exposed. Dependants are not an error: unless the request forces the removal, the answer lists them, as application names, and the model for a model constraint. A forced removal lists nothing.
  - A removal deletes the constraints on the space, sets the default space of the applications that used it and the spaces they are exposed to to `alpha`, sets the endpoint and extra bindings to the application's default, moves the subnets of the space to `alpha`, and deletes the space.
  - A removal does not change the `default-space` model configuration key or the `juju-mgmt-space` controller configuration key.
- *Errors:*
  - **`spaces not supported`**: *Trigger:* The cloud does not support spaces. The client receives it with the code `not supported`. *Remediation:* Use a cloud that supports spaces.
  - **`modifying provider-sourced spaces not supported`**: *Trigger:* The cloud discovers its spaces. The client receives it with the code `not supported`. *Remediation:* Change the space in MAAS, and {ref}`reload spaces <space-reloading>`.
  - **`"<name>" is not a valid tag`** and **`"<name>" is not a valid space tag`**: *Trigger:* The request does not carry a space tag, or the tag's name is neither a UUID version 7 nor lower-case letters and digits in words joined by hyphens. *Remediation:* Use a tag with a name such as `db-space`.
  - **`the "alpha" space cannot be removed`**: *Trigger:* The request names `alpha`. *Remediation:* Leave `alpha` in place.
  - **`space not found`**: *Trigger:* No space has the name. *Remediation:* List the spaces and use an existing name.
  - **`removing space "<name>": <reason>`**: *Trigger:* The database refused the change. One case is a forced removal of an application that is exposed to both this space and `alpha` at the same endpoint. *Remediation:* Remove the exposure to one of the two, and retry.
  - **`operation is blocked`** with the operator's message: *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.
  - **`permission denied`**: *Trigger:* The caller is not a model administrator. *Remediation:* Ask a model administrator.

## Spaces and model migration

A {ref}`migration <model-migrating>` copies the spaces and subnets of a model to the target controller, except `alpha`, which every model has already. A model that arrives from a 3.x controller keeps its subnets, and Juju links each one to its space through the space's former ID. A model on a Kubernetes cloud arrives with only the two catch-all subnets `0.0.0.0/0` and `::/0`.
