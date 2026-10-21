---
myst:
  html_meta:
    description: "Juju application reference: what an application is, the records the model keeps about it, the changes that agents and workers follow, and the operations that deploy, show, configure, scale, constrain, expose, bind, upgrade, and remove applications and that let units report on them."
---

(application)=
# Application

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>`, {ref}`Terraform Provider for Juju | Manage applications <tfjuju:manage-applications>`
```

An **application** is the deployment of one {ref}`charm <charm>` in a {ref}`model <model>`: the software that the charm defines, run as {ref}`units <unit>`. An application belongs to one model and has a name that no other application of the model has. An application comes into being when a charm is deployed and goes when it is removed.

(application-database-schema)=
## Application database schema

The application database schema is the set of records that the {ref}`model database <database>` keeps about the applications of a model. The records fall into five groups: the application itself, its configuration and constraints, its endpoints and bindings, its exposure, and its scale. The application itself has one figure and these records:

```{ggarch}
:file: ../juju.ggarch
:view: Application database schema
:caption: The model database keeps one record for each application. An application belongs to one charm and to one default space, and a charm and a space each have many applications. An application has at most one status, at most one platform, at most one channel, and at most one agent record. A status takes one of a fixed set of values, and a value has many statuses. The workload version, the controller mark, and the life, operating system, and architecture values are described in the text. The charm and the space are described on their own pages, and the figure shows them only for context.
:alt: Eight boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Application is in the middle. Status is above it (1 to 1), and Status value is above Status (m to 1). Charm is to the left of Application (m to 1), and Space is to its right (m to 1). Charm and Space also have a link mark for records that other pages own. Platform is below Application (1 to 1), Channel is below and to the left (1 to 1), and Agent is below and to the right (1 to 1).
```

The records of the application itself, with the rules that apply to each, are:


- **Application:** The application's name, its life (alive, dying, or dead), the charm it runs, the charm's modified version (a number that grows each time the charm is replaced, and starts at 0), whether its units may upgrade the charm while they are in error, and its default space. The {ref}`charm <charm>` page describes the charm. The {ref}`space <space>` page describes the space.
  - *Rules:*
    - A name appears once in the model.
    - An application has a charm and a default space.
    - The database refuses to move the life of an application backwards.

- **Platform:** The base on which the application runs: an operating system, a channel of that operating system, and an architecture.
  - *Rules:*
    - An application has at most one.
    - The operating system is `ubuntu`.
    - The architecture is one of `amd64`, `arm64`, `ppc64el`, `s390x`, and `riscv64`.

- **Channel:** The {ref}`charm channel <charm-channel>` that the application follows: a risk, with an optional track and branch. A local charm has none.
  - *Rules:*
    - An application has at most one.
    - A channel has a risk.

- **Status:** The status of the application, with a message, data, and the time of the last change. While the value is `unset`, the status that a client sees is derived from the statuses of the units (see {ref}`application status <application-status>`).
  - *Rules:*
    - An application has at most one.
    - The value is one of `unset`, `unknown`, `maintenance`, `waiting`, `blocked`, `active`, `terminated`, and `error`.

- **Workload version:** The version of the workload software, as the units report it.
  - *Rules:*
    - An application has at most one.
    - A version is required.

- **Controller mark:** The record that says an application is the controller application, which is the application that runs the {ref}`controller <controller>` charm in the controller model.
  - *Rules:*
    - The model has at most one.
    - The database refuses to update a controller mark. A mark can be inserted and deleted.

- **Agent:** The hash of the password with which the application's agent logs in. The Kubernetes provisioner sets the password (see {ref}`application change streams <application-change-streams>`). The agent of the application's pods logs in with it as the application.
  - *Rules:*
    - An application has at most one.
    - The hash algorithm is `sha512`.

The configuration and the constraints of an application have their own figure:

```{ggarch}
:file: ../juju.ggarch
:view: Application configuration database schema
:caption: The model database keeps the configuration of an application as one config item for each key, and an application has many config items. A config item has one type, and a type has many config items. An application has at most one config hash, at most one setting record, and at most one application constraint. An application constraint refers to at most one constraint, and a constraint can belong to many application constraints. An application has many device constraints, and a device constraint has many attributes. The constraint is described on its own page, and the figure shows it only for context.
:alt: Nine boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Application is in the middle. Config item is to its left (m to 1), and Config type is to the left of Config item (m to 1). Config hash is above Application (1 to 1), and Setting is below it (1 to 1). Application constraint is to the right of Application (1 to 1), and Constraint is to the right of Application constraint (m to at most 1, with a dashed line). Device constraint is above Application constraint (m to 1), and Device attribute is to the right of Device constraint (m to 1).
```

The records of the configuration and the constraints, with the rules that apply to each, are:


- **Config item:** One key and value of the application's {ref}`configuration <application-configuration>`, with the type of the value. The records hold only the keys that someone has set. A key that has no record takes the charm's default.
  - *Rules:*
    - An application has each key once.
    - The type is one of `string`, `int`, `float`, `boolean`, and `secret`.

- **Config hash:** The SHA-256 hash of the application's config items, sorted by key, and of its trust setting. A unit agent compares it with the hash it last saw to learn that the configuration has changed.
  - *Rules:*
    - An application has at most one.
    - A hash is required.

- **Setting:** The application's `trust` setting, which is off by default.
  - *Rules:*
    - An application has at most one.

- **Application constraint:** The {ref}`constraint <constraint>` that the application sets for the machines or pods of its units. The constraint page describes the keys.
  - *Rules:*
    - An application has at most one.

- **Device constraint:** A request for a device that the charm declares: its name, its type, and how many.
  - *Rules:*
    - An application has each device name once.
    - A device constraint has each attribute key once.

(application-endpoint)=
The endpoints of an application and the spaces that they are bound to have their own figure:

```{ggarch}
:file: ../juju.ggarch
:view: Application endpoint database schema
:caption: The model database keeps one endpoint record for each endpoint of the application's charm, and an application has many endpoints. An endpoint refers to one relation that the charm declares, and a charm's relation has many endpoints. An endpoint refers to at most one space, and a space has many endpoints. An application also has many extra bindings. An extra binding refers to one charm extra binding and to at most one space. The application itself refers to one default space. The relation, the charm extra binding, and the space are described on their own pages, and the figure shows them only for context.
:alt: Six boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Application is on the left. Space is to its right (Application to Space m to 1). Endpoint is above Space and joins Application (m to 1) and Space (m to at most 1, with a dashed line). Extra binding is below Space and joins Application (m to 1) and Space (m to at most 1, with a dashed line). Relation is to the right of Endpoint (m to 1), and Charm extra binding is to the right of Extra binding (m to 1). Space, Relation, and Charm extra binding also have a link mark for records that other pages own.
```

The records of the endpoints and bindings, with the rules that apply to each, are:

- **Endpoint:** One end that the application can use in a {ref}`relation <relation>`: the pairing of the application with one endpoint of its charm. A charm endpoint (declared in the charm's metadata) has a name, a role (`provider`, `requirer`, or `peer`), an interface, and a scope. The {ref}`relation <the-implicit-juju-info-relation-endpoint>` page describes the implicit `juju-info` endpoint that every charm has. The endpoint also names the space that it is bound to. A binding to no space means the application's default space.
  - *Rules:*
    - An application has each charm endpoint once.

- **Extra binding:** The binding of one extra binding of the charm (a network that the charm uses outside any relation) to a space. A binding to no space means the application's default space.
  - *Rules:*
    - An application has each charm extra binding once.

The exposure of an application has its own figure:

```{ggarch}
:file: ../juju.ggarch
:view: Application exposure database schema
:caption: The model database keeps exposure as two kinds of record. An application has many exposed spaces and many exposed CIDRs. Each refers to at most one endpoint, and an endpoint has many of each. An exposed space also refers to one space, which the figure leaves out. An exposed CIDR holds a network range as text and refers to nothing else. An exposed record with no endpoint stands for every endpoint of the application.
:alt: Four boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Application is on the left, and Endpoint is to its right (m to 1). Exposed space is above Endpoint, and Exposed CIDR is below it. Each of them joins Application (m to 1) and joins Endpoint (m to at most 1, with a dashed line).
```

The records of the exposure, with the rules that apply to each, are:


- **Exposed space:** A space that an endpoint is exposed to, so that machines in the space can reach the ports that units open.
  - *Rules:*
    - A named endpoint of an application is exposed to a space once.

- **Exposed CIDR:** A network range, in CIDR notation, that an endpoint is exposed to. The range is free-form text with no check at the database.
  - *Rules:*
    - A named endpoint of an application is exposed to a range once.

An application is exposed when it has at least one exposed space or exposed CIDR.

The scale of an application and its Kubernetes provisioning have their own figure:

```{ggarch}
:file: ../juju.ggarch
:view: Application scale database schema
:caption: The model database keeps provisioning records for the applications of a Kubernetes model. An application has at most one scale record, at most one provisioner mark, and at most one operator status. A Kubernetes service record belongs to one application and to one network node. The network node is described on its own page, and the figure shows it only for context.
:alt: Six boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Application is in the middle. Scale is above it (1 to 1), and Provisioner mark is below it (1 to 1). Operator status is to its left (1 to 1). Kubernetes service is to its right (m to 1), and Network node is to the right of Kubernetes service (m to 1). Network node also has a link mark for records that another page owns.
```

The records of the scale and the provisioning, with the rules that apply to each, are:


- **Scale:** The number of units that the application should have (the scale), the target of a scaling that is in progress, whether a scaling is in progress, and the lowest unit number that the application's StatefulSet uses (the start ordinal, which is 0 at first and moves up when a unit with a lower number is removed). An application on a machine cloud has no scale record.
  - *Rules:*
    - An application has at most one.
    - A scaling is not in progress at first.
    - A start ordinal is required.

- **Provisioner mark:** A record that exists while the Kubernetes provisioner manages the Kubernetes resources of the application. Removing the application waits until the mark is gone.
  - *Rules:*
    - An application has at most one.

- **Operator status:** The status that the Kubernetes provisioner reports for the application's pods: a value from the same set as the status record, with a message, data, and the time of the last change.
  - *Rules:*
    - An application has at most one.
    - The value is one of `unset`, `unknown`, `maintenance`, `waiting`, `blocked`, `active`, `terminated`, and `error`.

- **Kubernetes service:** The Kubernetes Service that fronts the application's pods: its provider ID and the network node that holds its addresses.
  - *Rules:*
    - An application has at most one.
    - A provider ID appears once in the model.
    - A network node belongs to at most one Kubernetes service.

(application-change-streams)=
## Application change streams

Application change streams are the notifications that tell the agents and workers of a model that something about an application has changed, so that they rarely have to poll. A change stream is delivered through a watcher (an object that tells its holder when something it follows has changed). A {ref}`worker <agent-workers>` is a long-running component inside an agent that does one job. Units, a machine firewall, and the Kubernetes workers follow these changes in an application:

- **The application record:** A {ref}`unit agent <unit-agent>` follows the record of its own application, and learns of a change of the charm, the life, or the default space.
- **Configuration:** A unit agent follows the config hash of its application, and runs the `config-changed` {ref}`hook <hook-config-changed>` when the hash differs from the one it last saw. The trust setting is part of the same hash.
- **Exposure:** On a machine model the `firewaller` worker follows the exposed spaces and ranges of each application. It opens or closes the firewall for the ports that units opened when the exposure changes.
- **Applications, scale, trust, and unit life:** On a Kubernetes model, `caas-application-provisioner` follows the set of applications that the model holds. It starts one application worker for each application. That worker follows the application's scale, its settings (trust), and the life of its units. `caas-firewaller` follows the set of applications to run the firewalls of the model.
- **Pending charms:** A worker follows the applications whose charm is still being downloaded, and downloads the charm (see {ref}`charm <charm>`).

The application worker of a Kubernetes model carries out these changes in the Kubernetes cluster. When it starts for an alive application, it sets a new password for the application's agent and stores its hash in the agent record. It marks the application with a provisioner mark before it creates Kubernetes resources. It clears the mark when a dead application's resources are gone. It writes the scale record as it scales. It creates or removes pods to match the scale. When a pod's unit is removed, it removes the unit's record. It writes the operator status: `waiting` with the message `waiting for units to settle down` while units are being added, `active` after that, and `error` with the message of the failure when it cannot create the resources. It also copies the status that the Kubernetes Service reports into the operator status. It records the addresses of the Service.

(application-operations)=
## Application operations

Application operations are the actions that {ref}`clients <client>` and unit agents can take on applications through the controller API (the set of calls that clients and agents make to the controller). A **{ref}`unit agent <unit-agent>`** is the process that runs the charm of a unit. It logs in as its unit and acts only for that unit. No model access level or block applies to a unit agent. Clients deploy, show, configure, scale, constrain, expose, bind, upgrade, and remove applications. Unit agents read the application's configuration and status, and report its status and workload version. Adding and removing relations is described on the {ref}`relation <relation>` page, consuming an offer on the {ref}`offer <offer>` page, removing a single unit and resolving the errors of units on the {ref}`unit <unit>` page, and changing the storage directives of an application on the {ref}`storage <storage-directive>` page.

(application-deploying)=
### Deploying applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>`
```

Deploying applications creates an application from a charm, with its units. A client names a charm and the controller finds its revision, or the client gives a charm that it has already added to the model, such as a local charm or a charm of a bundle. The request can give the application's name, the number of units, configuration, trust, constraints, placement directives, storage, devices, endpoint bindings, resources, and a force option.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request that names a charm has the channel `stable` and one unit unless it says otherwise. The controller takes the architecture from the constraints, else from the model, and takes the base from the request, else from the model's `default-base` setting, else from the machines of the placement directives. It resolves the charm for that platform. A base that the charm does not support is refused unless the request is forced.
  - A request that gives a charm needs the charm's URL to include its revision, and needs a pending resource for each resource of the charm.
  - A request with no application name uses the name of the charm.
  - An application name starts with a lowercase letter and holds lowercase letters, digits, and hyphens. A hyphen is followed by at least one letter somewhere before the next hyphen, so `a-1` is not valid and `a-b1` is.
  - The name of an application appears once in the model.
  - The controller charm cannot be deployed by hand.
  - A request that names a charm refuses a Kubernetes charm (a charm that declares containers) on a machine model. It accepts a charm that declares no container on a Kubernetes model and warns that the charm has no workload to run there. A subordinate charm gets no warning.
  - A request creates the number of units that it asks for, each with its own machine on a machine model. A subordinate charm is deployed with no units and no constraints. Its units are created later, when a relation joins the subordinate application to a principal application (see {ref}`subordinate relations <subordinate-relation>`).
  - A charm whose `assumes` expressions the model does not meet is refused unless the request is forced.
  - The request can name existing storage to attach to the application's first unit. It then must ask for exactly one unit.
  - On a Kubernetes model a charm in the first metadata format, a charm that needs block storage, more than one placement directive, and attaching existing storage are all refused.
  - A new application has the life alive, the status `unset`, the platform and channel that the controller resolved, and a default space that is the model's `default-space` setting or else the alpha space. A binding with an empty endpoint name changes the default space.
  - An application gets one endpoint record for each endpoint of its charm, one extra binding for each extra binding, and one peer relation for each peer endpoint (see {ref}`peer relation <peer-relation>`).
  - An application gets one storage directive for each storage of its charm. The count is the charm's minimum, or zero when the minimum is zero. The size is the charm's minimum, or 1024 MiB. The pool is the one that the request names, or else the model's default for the storage kind.
  - A Kubernetes application also gets a scale record with the number of units that the request asked for.
  - When the request fails, the pending resources that it named are deleted.
- *Errors:* The client receives each in the result for the application. A failure while creating the application has the prefix `cannot deploy "<application>": `. A name that is not valid, or an application that exists, is reported with further prefixes from the steps that failed, and the message ends with the text shown:
  - **`application name is not valid`** (code `not valid`): *Trigger:* The name does not match the pattern. *Remediation:* Use lowercase letters, digits, and hyphens, and start with a letter.
  - **`application already exists`**: *Trigger:* The model has an application with the name. *Remediation:* Choose another name.
  - **`charm "<name>" not found`** (code `not found`): *Trigger:* The charm was not added to the model. *Remediation:* Add the charm first.
  - **`charm url must include revision`** and **`not all pending resources for charm provided`**: *Trigger:* A charm that the client gives has no revision, or a resource of the charm has no pending upload. *Remediation:* Give the revision. Upload each resource first.
  - **`<charm> is a Kubernetes charm (it declares containers) but <model> is a machine model; its workload will not run`** (code `not supported`): *Trigger:* The request names a Kubernetes charm for a machine model. *Remediation:* Deploy the charm to a Kubernetes model, or choose a machine charm.
  - **`not supported <reason>`** (code `not supported`): *Trigger:* The model does not meet the `assumes` expressions of the charm. *Remediation:* Meet the features, or force the request.
  - **`manual deploy of the controller charm not supported`** (code `not supported`): *Trigger:* The charm is the controller charm. *Remediation:* Deploy another charm.
  - **`subordinate application must be deployed without units`** and **`subordinate application must be deployed without constraints, not "<constraints>"`**: *Trigger:* The request gives units or constraints for a subordinate charm. *Remediation:* Leave them out.
  - **`base "<base>" not supported by charm, the charm supported bases are: <bases>. Use --force to deploy the charm anyway.`**: *Trigger:* A request that names a charm resolves to a base that is not one of the charm's bases. *Remediation:* Request a base that the charm supports. The message ends with ` Used the default-base.` when the base came from the model.
  - **`attaching existing storage can only be done when the number of units is 1`** (code `not found`) and **`storage instance "<id>" does not exist`** (code `not found`): *Trigger:* The request attaches storage to more than one unit, or names storage that does not exist. *Remediation:* Ask for one unit, and name existing storage.
  - **`attaching storage to CAAS units is not supported`** (code `not supported`), **`deploying format v1 charm "<name>" not supported`** (code `not supported`), **`only 1 placement directive is supported for container models, got <n>`**, and **`block storage "<name>" is not supported for container charms`**: *Trigger:* The request does something that a Kubernetes model refuses. *Remediation:* Remove the option, or choose another charm.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. These fail the whole call, with no prefix. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-showing)=
### Showing applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>`
```

Showing applications returns what a client needs to describe applications. A request names applications by tag. The answer has one result for each. The read calls answer four questions: what the applications are (`ApplicationsInfo`, `UnitsInfo`), how one application is configured (`Get`, `CharmConfig`, `GetConfig`), which charm one application runs (`GetCharmURLOrigin`), and which unit leads one application (`Leader`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model.
- *Rules:*
  - The details of an application are its tag, the name of its charm, its base, its channel (only for a Charmhub charm that has one), its constraints, whether it is a principal application, whether it is exposed, its life, its endpoint bindings (endpoint name to space name, with the empty name for the default space), and its exposed endpoints (endpoint name to spaces and ranges, with the empty name for all endpoints).
  - A subordinate application has no constraints in the answer.
  - The answer of the configuration call (`Get`) has the merged charm configuration (each option with its type, description, default, and current value), the application's `trust` setting, its constraints, its base, and its channel. The calls `CharmConfig` and `GetConfig` return the merged configuration alone.
  - The charm of an application is returned as its URL and its origin: source, ID, hash, revision, architecture, base, and channel.
  - The leader is the name of the unit that holds the leadership of the application.
  - The endpoint names of an application are the endpoints of its charm.
- *Errors:* The client receives each in the result for the application, except the last:
  - **`application "<name>" not found`** or **`application <name> not found`** (code `not found`): *Trigger:* The model has no application with the name. *Remediation:* Use the name of an existing application.
  - **`leader for application-<name> not found`** (code `not found`): *Trigger:* No unit holds the leadership. *Remediation:* Retry when a unit is running.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller lacks read access. This fails the whole call. *Remediation:* Ask a model admin.

(application-configuring)=
### Configuring applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Configure an application <configure-an-application>`
```

Configuring applications sets or unsets the {ref}`configuration <application-configuration>` keys of applications, including the `trust` setting. A request lists applications. Each entry has the application name and either the keys with their values or the keys to unset.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on. Reading the configuration needs {ref}`read access <user-access-model-read>`.
- *Rules:*
  - A value is given as a string. The controller converts it to the type of the charm option.
  - The key `trust` is a setting of the application, not an option of the charm. It must be a boolean.
  - A key that the charm does not declare is refused.
  - The value of a `secret` option must be a secret URI.
  - Setting configuration recomputes the config hash. The change reaches the units through the hash (see {ref}`application change streams <application-change-streams>`).
  - The configuration of an application, its keys and values, holds at most 16 MiB.
  - Unsetting removes the keys from the config items, so that the charm default applies again. A key that is not set is ignored. Unsetting `trust` sets it to off.
  - Setting configuration for an application that is dead is refused. A remote application of a cross-model relation is not found.
  - A configuration in YAML form is not accepted for an application that already exists.
- *Errors:* The client receives each in the result for the entry, except the last:
  - **`invalid application config: unknown option "<key>"`** (code `not valid`): *Trigger:* The charm has no option with the key. *Remediation:* Use a key that the charm declares.
  - **`parsing trust setting: <reason>`**: *Trigger:* The value of `trust` is not a boolean. *Remediation:* Give `true` or `false`.
  - **`invalid secret URI for option "<key>": <reason>`** and **`invalid secret config`**: *Trigger:* The value of a `secret` option is not a string, or is not a secret URI. *Remediation:* Give the URI of a secret.
  - **`updating application config: refreshing config hash: checking application config size: max allowed total size (16777216): quota limit exceeded`** (code `quota limit exceeded`): *Trigger:* The configuration is larger than the limit. *Remediation:* Unset keys.
  - **`application is dead`**: *Trigger:* The application is dead. *Remediation:* Deploy it again.
  - **`application "<name>" not found`** or **`application <name> not found`** (code `not found`): *Trigger:* The model has no such application. *Remediation:* Use the name of an existing application.
  - **`config yaml not supported not implemented`** (code `not implemented`): *Trigger:* The entry has YAML configuration. *Remediation:* Give the keys and values.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. These fail the whole call. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-scaling)=
### Scaling applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Scale an application <scale-an-application>`
```

Scaling applications changes the number of units of an application. On a Kubernetes model a client sets the scale of the application. The provisioner then adds or removes pods. On a machine model a client adds units. Each unit gets a machine. Removing a single unit is described on the {ref}`unit <unit>` page.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request on a Kubernetes model gives either a new scale or a change of scale for each application, never both.
  - A change of scale is relative to the stored scale and cannot take it below zero. The request fails when the scale changed between its read and its write.
  - The controller application cannot be scaled to zero.
  - The answer gives the new scale. The provisioner then moves the pods to the stored scale.
  - A request on a Kubernetes model cannot attach existing storage.
  - A request on a machine model gives the number of units to add and can give a placement directive and existing storage for each unit. The number is at least one.
  - Existing storage can be attached only when one unit is added.
  - A machine model refuses to set the scale. A Kubernetes model refuses to add units.
- *Errors:* The client receives each in the result for the application, except the first two and the last:
  - **`scaling applications on a non-container model not supported`** (code `not supported`): *Trigger:* The model runs on a machine cloud. *Remediation:* Add units.
  - **`adding units to a container-based model is not supported`** (code `not supported`): *Trigger:* The model runs on Kubernetes. *Remediation:* Set the scale.
  - **`scale < 0 not valid`** and **`requesting both scale and scale-change not valid`** (code `not valid`): *Trigger:* The scale is negative, or the request has both a scale and a change. *Remediation:* Give a scale of zero or more, or one of the two.
  - **`attaching storage when scaling a k8s application not implemented`** (code `not implemented`): *Trigger:* The request attaches storage. *Remediation:* Leave the storage out.
  - **`scale change invalid: cannot remove more units than currently exist`** and **`cannot scale controller application to 0 units`**: *Trigger:* The change goes below zero, or the application is the controller application. *Remediation:* Give a smaller change. Keep at least one unit.
  - **`scaling state is inconsistent`**: *Trigger:* The scale changed during the request. *Remediation:* Repeat the request.
  - **`must add at least one unit`** and **`AttachStorage is non-empty, but NumUnits is <n>`** (code `not valid`): *Trigger:* A request to add units asks for none, or attaches storage to several units. *Remediation:* Ask for one or more units, and one unit when attaching storage.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. These fail the whole call. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-constraining)=
### Setting application constraints

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Manage constraints for an application <manage-constraints-for-an-application>`
```

Setting application constraints stores the {ref}`constraints <constraint>` that Juju uses when it asks for the machines or pods of the application's units. A client also reads them.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on. Reading the constraints needs {ref}`read access <user-access-model-read>`.
- *Rules:*
  - Each call replaces all the constraints of the application.
  - A constraint that the cloud does not support is accepted and logged. Juju ignores it when it provisions.
  - A dying application accepts constraints.
  - Reading the constraints needs the tag of an application.
- *Errors:* The client receives each without a prefix:
  - **`application <name> not found`** (code `not found`): *Trigger:* The model has no such application. *Remediation:* Use the name of an existing application.
  - **`application is dead`**: *Trigger:* The application is dead. *Remediation:* Deploy it again.
  - **`invalid application constraints`**: *Trigger:* The constraints name a container type or a space that does not exist. *Remediation:* Name a valid container type and an existing space.
  - **`unexpected tag type, expected application, got <kind>`**: *Trigger:* A read names something other than an application. *Remediation:* Name an application.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks the access, or the change block is on. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-exposing)=
### Exposing and unexposing applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Manage an application's public availability over the network <manage-an-applications-public-availability-over-the-network>`
```

Exposing applications lets machines outside the model reach the ports that the units of an application opened. The request names the endpoints to expose, and for each endpoint the spaces and the CIDR ranges that may reach it. Unexposing removes the exposure of the named endpoints.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request that names no endpoint exposes every endpoint to `0.0.0.0/0` and `::/0`.
  - An endpoint that names no space and no range is exposed to `0.0.0.0/0` and `::/0`.
  - Exposing replaces the settings of each endpoint that the request names. It leaves the other endpoints as they are.
  - An endpoint name is an endpoint of the application's charm. The empty name stands for all endpoints.
  - Each space is named, and must exist.
  - Unexposing a list of endpoints removes their exposed spaces and ranges. An empty list removes all of them.
  - The controller application cannot be unexposed.
  - The `firewaller` worker (see {ref}`application change streams <application-change-streams>`) opens the ports only for the sources that the exposure names.
- *Errors:* The client receives each without a prefix:
  - **`one or more of the provided endpoints [<names>] do not exist`**: *Trigger:* A name is not an endpoint of the charm. *Remediation:* Use the endpoints that {ref}`Showing applications <application-showing>` lists.
  - **`space "<name>" not found`** (code `not found`): *Trigger:* A named space does not exist. *Remediation:* Use an existing {ref}`space <space>`.
  - **`unexposing the controller application not supported`** (code `not supported`): *Trigger:* The application is the controller application. *Remediation:* Leave it exposed.
  - **`application <name> not found`** (code `not found`): *Trigger:* The model has no such application. *Remediation:* Use the name of an existing application.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-binding)=
### Changing endpoint bindings

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Change space bindings for an application <change-space-bindings-for-an-application>`
```

Changing endpoint bindings binds endpoints of applications to {ref}`spaces <space>`. A request lists applications. Each entry has the bindings to merge (endpoint name to space name, with the empty name for the default space) and a force option.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request changes only the endpoints that it names. The other bindings stay as they are.
  - An endpoint is an endpoint or an extra binding of the application's charm. The empty name is the default space of the application.
  - A request that changes no binding is accepted at once.
  - Every unit of the application must have an address in each space that the request names. A request with the force option is accepted without that check.
  - Each entry has its own result.
- *Errors:* The client receives each in the result for the entry:
  - **`one or more of the provided endpoints "<names>" do not exist`**: *Trigger:* A name is not an endpoint of the charm. *Remediation:* Use the endpoints that {ref}`Showing applications <application-showing>` lists.
  - **`space(s) "<names>" not found`**: *Trigger:* A named space does not exist. *Remediation:* Use an existing {ref}`space <space>`.
  - **`unit "<unit>" not in space "<space>"`**: *Trigger:* A unit has no address in a named space. The result lists one message for each unit. *Remediation:* Move the unit into the space, or force the request.
  - **`application <name> not found`** (code `not found`): *Trigger:* The model has no such application. *Remediation:* Use the name of an existing application.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. These fail the whole call. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-upgrading)=
### Upgrading applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Upgrade an application <upgrade-an-application>`
```

Upgrading applications replaces the charm of an application with another revision, or with another charm. The request names the application, the charm's URL and origin, and can give storage directives, endpoint bindings, resources, and the options to force the base and to proceed with units that are in error.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on, unless the request forces the units.
- *Rules:*
  - The application and the new charm must exist. The application must not be dead.
  - A relation of the application must have a counterpart in the new charm with the same role, interface, and scope, and a limit that the existing relations meet. A peer relation of the old charm needs a peer endpoint of the same name in the new charm.
  - The new base must be compatible with the application's base, unless the request forces the base.
  - The new charm may add storage. It must keep every storage of the old charm. It may not raise a minimum size or minimum count, lower a maximum count, change a storage between single and multiple when it has a location, change a shared or read-only flag, change the location, or change the type. On a Kubernetes model the storage must be the same.
  - A storage directive override that the request gives names a pool that exists and a count that fits.
  - The charm of the application becomes the new charm. Its modified version takes the next number.
  - The configuration keeps the keys that the new charm still declares. The other keys are dropped.
  - The endpoints follow the new charm. An endpoint that the new charm adds is bound to the default space, unless the request binds it.
  - The channel and the platform change when the request gives them.
  - A storage directive is added for each storage that the new charm adds.
- *Errors:* The client receives each without a prefix:
  - **`application "<name>" not found`** and **`charm "<url>" not found`** (code `not found`): *Trigger:* The application or the charm does not exist. *Remediation:* Use an existing application, and add the charm first.
  - **`application is dead`**: *Trigger:* The application is dead. *Remediation:* Deploy it again.
  - **`cannot set charm "<url>": refreshing application from base "<current>" to "<requested>": incompatible base for charm`** (code `incompatible base`): *Trigger:* The new base is not compatible with the current one. *Remediation:* Choose a charm for the current base, or force the base.
  - **`cannot set charm "<url>" because <reason>`** (code `not supported`): *Trigger:* The new charm's storage breaks a rule above. The reason is one of `storage definition "<name>" removed`, `storage definition "<name>" new minimum size <n> MiB exceeds existing minimum size <m> MiB`, `storage definition "<name>" new minimum count <n> exceeds existing minimum count <m>`, `storage definition "<name>" new maximum count <n> is less than existing maximum count <m>`, and similar texts for the other changes. *Remediation:* Choose a charm with compatible storage.
  - **`charm has no corresponding relation "<name>"`**, **`charm has no corresponding peer relation "<name>". Please scale down to 0 units to refresh`**, **`cannot change role of relation "<name>" from <old> to <new>`**, **`cannot change interface of relation "<name>" from <old> to <new>`**, and **`cannot change scope of relation "<name>" from <old> to <new>`**: *Trigger:* The new charm changes a relation that the application uses. *Remediation:* Remove the relation first, or choose a charm that keeps it.
  - **`storage directive "<name>" references an invalid pool`** (code `not valid`), **`storage directive "<name>" references unknown storage pool "<pool>"`** (code `not found`), and **`storage directive "<name>" override count <n> exceeds maximum <max>`** (code `not valid`): *Trigger:* A storage directive override of the request is wrong. *Remediation:* Name an existing pool and a smaller count.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-removing)=
### Removing applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>` > {ref}`Remove an application <remove-an-application>`
```

Removing applications removes an application with its units and with the records and cloud resources that depend on them. The request names applications by tag. It can ask to destroy the storage of the units, to force the removal, to wait before the force applies, and to do a dry run. Removing a remote application that a model consumes is a separate call with the same access and blocks.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. Neither the remove block nor the change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) may be on.
- *Rules:*
  - The controller application cannot be removed.
  - A dry run changes nothing. It returns the units that the removal would destroy, and on a machine model the storage that it would destroy and the storage that it would detach.
  - The application, its relations, and its units become dying at once. Storage becomes dying too when the request destroys storage. A machine becomes dying when it held only the units that are being removed. The removal of each of them is scheduled.
  - A removal that is not forced makes the application dead when it has no unit and no relation left, and deletes it on a later pass. On a Kubernetes model it also waits until the provisioner has deleted the Kubernetes resources and cleared its mark.
  - A forced removal does not wait for those conditions. The time to wait applies only to a forced removal. With a wait, Juju first tries the removal that is not forced, and forces it when the time has passed.
  - Deleting the application deletes its config items, config hash, setting, constraint, device constraints, endpoints, extra bindings, exposure, platform, channel, status, operator status, agent, workload version, scale, provisioner mark, and Kubernetes service with its network node, the secrets that the application owns, and the offers of the application.
  - The charm is deleted afterward when no other application or unit uses it.
- *Errors:* The client receives each in the result for the application, except the last:
  - **`application "<name>" not found`** (code `not found`): *Trigger:* The model has no such application. *Remediation:* Use the name of an existing application.
  - **`removing the controller application not supported`** (code `not supported`): *Trigger:* The charm of the application is the controller charm. *Remediation:* Destroy the controller instead.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the remove block or the change block is on. These fail the whole call. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(application-reading-configuration)=
### Reading application configuration

The unit agent reads the configuration of its application, through the `config-get` hook tool (see {ref}`config-get <hook-command-config-get>`). The agent asks for the settings of its unit, and gets the merged configuration of the application's charm. It runs the `config-changed` hook when the config hash of its application differs from the one it last saw.

The operation's required access and rules are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The answer has every option of the charm. An option that has no config item has the charm's default.

(application-reading-status)=
### Reading application status

The unit agent of the leader unit reads the status of its application through the `status-get` hook tool, with the option `--application` (see {ref}`status-get <hook-command-status-get>`). The answer has the status of the application and the statuses of its units.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit. The unit must be the leader of its application.
- *Rules:*
  - The application status is the stored status, or the status derived from the unit statuses while the stored value is `unset` (see {ref}`application status <application-status>`).
- *Errors:* The unit agent receives each in the result for the unit:
  - **`application "<name>" not found`** (code `not found`): *Trigger:* The unit's application does not exist. *Remediation:* Use a unit of an existing application.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The unit is not the leader, or is not the unit that the agent runs. *Remediation:* Use the unit that the agent runs, and read the status from the leader.

(application-setting-status)=
### Setting application status

The unit agent of the leader unit sets the status of its application through the `status-set` hook tool, with the option `--application` (see {ref}`status-set <hook-command-status-set>`). The request gives the unit, the status, a message, and data. The controller records the time of the change and adds it to the status history.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit. The unit must be the leader of its application.
- *Rules:*
  - The hook tool accepts `maintenance`, `blocked`, `waiting`, and `active`.
  - The status replaces the earlier status of the application, with its message and data.
  - Setting a status ends the derivation from the unit statuses (see {ref}`application status <application-status>`).
- *Errors:* The unit agent receives each in the result for the unit:
  - **`invalid status "<status>", expected one of [maintenance blocked waiting active]`**: *Trigger:* The hook tool got another status. This comes from the hook tool. *Remediation:* Use one of the four.
  - **`this unit is not the leader`**: *Trigger:* A unit that is not the leader used the option `--application`. This comes from the hook tool. *Remediation:* Run the tool on the leader unit.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The unit is not the leader, or is not the unit that the agent runs. *Remediation:* Use the unit that the agent runs, and set the status from the leader.
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use an existing unit.

(application-setting-workload-version)=
### Setting workload versions

The unit agent reports the version of the software that its unit runs, through the `application-version-set` hook tool (see {ref}`application-version-set <hook-command-application-version-set>`). The request gives the unit and the version.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The version is stored for the unit and for its application in the same transaction. The last version that a unit reported is the application's version.
  - The hook tool needs a version.
- *Errors:* The unit agent receives each in the result for the unit:
  - **`no version specified`**: *Trigger:* The hook tool got no version. This comes from the hook tool. *Remediation:* Pass the version.
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use an existing unit.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(application-and-model-migration)=
## Applications and model migration

A {ref}`migration <model-migrating>` moves the applications of a model with their units, configuration, constraints, endpoint bindings, exposure, storage directives, and scale. It starts only when every application and every unit is alive. No unit may be upgrading. A precheck that fails reports `application(s) "<names>" are not alive`, `unit(s) "<names>" are not alive`, or `unit(s) "<names>" are upgrading`. A remote application of a cross-model relation keeps the UUID that both models agreed on for it. The relation then still refers to it after the move. Principal applications are imported before subordinate ones.
