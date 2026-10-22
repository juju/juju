---
myst:
  html_meta:
    description: "Juju machine reference: what a machine is, the records the model keeps about it, the changes that agents and workers follow, and the calls that add, provision, list, show, retry, reprovision, and remove machines."
---

(machine)=
# Machine

```{ibnote}
See also: {ref}`Juju | Manage machines <manage-machines>`
```

A **machine** is a {ref}`compute resource <resource-compute>` that a {ref}`model <model>` requests from a machine {ref}`cloud <cloud>` to host {ref}`units <unit>`, usually one, though several units of the same or different {ref}`applications <application>` can share it. A machine is requested explicitly, by adding it, or implicitly, by deploying an application or adding a unit, and whether the cloud provides bare metal, virtual machines, or LXD containers, the result is a machine. A machine has a number as its name, such as `0`, and a LXD container on a machine has the name `0/lxd/5` (container `5` on machine `0`).

(machine-database-schema)=
## Machine database schema

```{ggarch}
:file: ../juju.ggarch
:view: Machine database schema
:caption: The model database keeps one record for each machine. A machine has one status record, at most one cloud instance (with its own status), at most one placement, at most one parent machine, at most one agent presence record, and any number of platform records and block devices. A machine in a container has a parent, which is another machine, so a machine can be the parent of many machines.
:alt: Nine boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Machine is in the middle. Machine status is above it (1 to 1), Platform is above and to the left (m to 1), Block device is above and to the right (m to 1), Parent is to its left and joins Machine twice, once for the container (1 to 1) and once for the host (m to 1), Agent presence is to its right (1 to 1), Placement is below and to the left (1 to 1), and Cloud instance is below it (1 to 1). Instance status is below Cloud instance (1 to 1).
```

The machine database schema is the set of records that the {ref}`model database <database>` keeps about the machines of a model. The records, with the rules that apply to each, are:

- **Machine:** The machine's name, its life (alive, dying, or dead), the hostname that its agent reported, the time its agent started, whether to keep its instance when the machine is removed, and the hash of its agent's password.
  - *Rules:*
    - A machine's name appears once.
    - A machine has a life.
    - The database refuses to move the life of a machine backwards.
- **Machine status:** The status that the machine's agent reports: one of `error`, `started`, `pending`, `stopped`, and `down`.
  - *Rules:*
    - A machine has at most one.
- **Platform:** The machine's base, which is its operating system and channel, and its architecture. A base identifies an operating system image: the name of the operating system, the symbol `@`, and the channel, given as `<track>` or `<track>/<risk>`, such as `ubuntu@22.04` or `ubuntu@22.04/stable`.
- **Cloud instance:** The instance that the cloud started for the machine: its instance ID and display name, its hardware (architecture, memory, cores, root disk, and availability zone), and its life.
  - *Rules:*
    - A machine has at most one.
    - The instance has a life of its own, separate from the machine's, and the database refuses to move it backwards.
    - An instance ID appears once among the machines of the model when it is set.
    - A display name appears once among the machines of the model when it is set.
- **Instance status:** The status of the cloud instance: one of `unknown`, `pending`, `allocating`, `running`, and `provisioning error`.
  - *Rules:*
    - An instance has at most one.
- **Placement:** The directive that placed the machine, such as a provider placement.
  - *Rules:*
    - A machine has at most one.
- **Parent:** The host machine of a container.
  - *Rules:*
    - A machine has at most one parent.
- **Agent presence:** A record that exists while the machine's agent is connected to the API, with the time it was last seen.
  - *Rules:*
    - A machine has at most one.
- **Block device:** A disk that the machine's agent reported, with its name, size, and the names under which the machine links to it.
  - *Rules:*
    - A machine has at most one block device of a given name.
    - A link name appears once on a machine.
- **Other records:** The machine's running agent version and architecture, the flag that says it needs to reboot, the tags that the cloud reported for its instance, its constraints and container type, and the marks that it is provisioned manually and that a reprovision was requested. Its volumes, filesystems, and SSH host keys are also kept per machine, and the {ref}`SSH server <juju-ssh>` page describes the host keys. Each belongs to one machine. A machine has at most one of each except for the tags.

(machine-change-streams)=
## Machine change streams

Machine change streams are the changes that the workers and agents of a model follow, so that they rarely have to poll. A change stream is delivered through a {ref}`watcher <watcher>`, and workers and agents follow the machines of a model, a machine's life and what depends on it, the containers of a host, the reboot flag, and agent start times:

- **Machines of the model:** The compute provisioner follows the machines that are not containers, and starts an instance for each new one. It also follows a request to reprovision a machine.
- **A machine's life and its dependants:** A machine's agent follows its machine's life and the life of its units, its child machines, and its storage, so that it can stop and mark the machine dead when the last of them has gone.
- **Containers of a host:** A machine agent follows the LXD containers on its machine, and starts and stops them.
- **Reboot:** A machine agent follows the reboot flag of its machine and of its parent, and reboots or shuts down as the flag says.
- **Agent start times:** The instance poller follows the life, agent start time, and reprovision state of every machine in the model.

(machine-calls)=
## Machine calls

Machine calls are the calls a {ref}`client <client>` can take on machines through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): add, get the provisioning script, retry provisioning, reprovision, show status, and remove. Connecting to a machine is described on the {ref}`SSH server <juju-ssh>` page. The constraints of a machine are described on the {ref}`constraint <constraint>` page.

(machine-adding)=
### Adding machines

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

Adding machines creates machines or containers in a model. The compute provisioner then starts an instance for each. A request carries, for each machine, an optional base, constraints, a container type, a parent, and a placement.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request names a {ref}`machine designation <machine-designations>`: a new machine, a container on a new or existing machine, or a placement directive. A request that names an existing machine adds no machine and only checks that its architecture meets the constraints.
  - A container type needs no parent.
  - A parent needs a container type.
  - A container type and a placement directive exclude one another. The only container type is `lxd`.
  - The base defaults to the model's preferred base.
  - The operating system of the base must be Ubuntu.
  - A provider placement must name this model: its scope is `model-uuid` or the model's UUID. Its directive is the model's UUID.
  - The constraints of the model and the constraints of the machine are merged. The machine's win.
  - The `container` constraint is dropped.
  - The cloud checks, before anything is stored, that it can start an instance with the merged constraints and the placement.
  - A space in the constraints must exist.
  - The name of a new machine is the next number in the model.
  - The name of a container is `<parent>/lxd/<n>`, with `n` the next number for that parent.
  - A machine is created alive, with the status `pending` and an instance with the status `pending` and no instance ID. A request that carries an instance ID, a nonce, and hardware characteristics, as manual provisioning does, records them with the machine, unless it names a container type or a placement, which discard them. An instance ID that starts with `manual:` marks the machine as manually provisioned.
  - The call takes a list of machines. Each machine in the list gets its own result.
- *Errors:* The client receives `permission denied` (code `unauthorized access`) and `operation is blocked` (code `operation is blocked`) for the whole call. Every other error is in the result for its machine, with no code:
  - **`parent machine specified without container type`**: *Trigger:* A parent without a container type. *Remediation:* Name the container type `lxd`.
  - **`container type and placement are mutually exclusive`**: *Trigger:* Both are in the request. *Remediation:* Give only one.
  - **`invalid container type`**: *Trigger:* A container type other than `lxd`. *Remediation:* Use `lxd`.
  - **`invalid placement: unknown os type "<os>", expected ubuntu`**: *Trigger:* A base whose operating system is not Ubuntu. *Remediation:* Use an Ubuntu base.
  - **`invalid placement: <cause>`**: *Trigger:* The placement directive is empty, not in the form `<parent>/<scope>/<child>`, or names a container type that is not supported. *Remediation:* Correct the directive.
  - **`invalid model id "<id>"`**: *Trigger:* A provider placement whose directive is not this model's UUID. *Remediation:* Use the model's UUID as the directive.
  - **`prechecking instance for create machine: <message>`**: *Trigger:* The cloud cannot start an instance with these constraints and this placement. *Remediation:* Change the constraints or the placement to what the cloud accepts.
  - **`merging machine and model constraints: <reason>`**: *Trigger:* The constraints conflict. *Remediation:* Resolve the conflict.
  - **`machine "<name>" not found`**: *Trigger:* The parent named in the request does not exist. *Remediation:* Name an existing machine.
  - **`getting machine hardware characteristics for "<uuid>": machine not provisioned`**: *Trigger:* A placement on a machine that has no instance yet. *Remediation:* Wait until the machine is provisioned.
  - **`machine "<uuid>" has arch "<a>", but constraints require arch "<b>"`**: *Trigger:* A placement on an existing machine whose architecture differs from the constraint. *Remediation:* Align the `arch` constraint with the machine.
  - **`cannot set constraints, space "<space>" does not exist`**: *Trigger:* A constraint names a missing space. *Remediation:* {ref}`Add the space <add-a-space>` first, or correct the name.
  - **`availability zone not found "<zone>" for machine "<uuid>"`**: *Trigger:* The request's hardware characteristics name an unknown zone. *Remediation:* Use an existing zone.
  - **`permission denied`**: *Trigger:* The caller lacks write access. *Remediation:* Ask a model administrator.
  - **`operation is blocked`**: *Trigger:* The change block is on. *Remediation:* Remove the block.

(machine-provisioning)=
### Provisioning machines

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Provisioning a machine
:caption: Provisioning a machine runs in this order: the compute provisioner sees a new machine in the model database, reads the machine's provisioning information, marks the instance status `allocating`, and asks the cloud to start an instance, trying each availability zone and making up to ten attempts, ten seconds apart. When the cloud returns an instance, the provisioner records its ID, hardware, and nonce in the model database. The instance downloads and starts the machine agent, which logs in, which records the agent's presence, and reports the status `started`. A start that fails on its last attempt sets the instance status `provisioning error` and the machine status `error`.
:alt: A sequence diagram with four participants: the compute provisioner, the model database, the cloud, and the machine agent. The compute provisioner reads a new machine from the model database, marks the instance status allocating, and asks the cloud to start an instance. The cloud returns the instance, and the provisioner records the instance in the model database. The instance downloads and starts the machine agent, which logs in and reports the status started to the model database.
```

Provisioning machines turns a machine record into a running cloud instance with a machine agent. The compute provisioner, the process on the controller that starts instances, does it for every machine that is not a container, and each machine's agent does it for the containers of that machine.

The caller, the rules, and the errors are:

- *Caller:* No client makes this call. The provisioner runs it for every new machine, which a user creates by adding a machine, deploying an application, or adding a unit.
- *Rules:*
  - The provisioner starts an instance only for machines that have no instance ID and whose status is `pending` or whose instance status is `allocating`.
  - It spreads the machines of an application across availability zones where the cloud supports it.
  - It tries each availability zone before it uses up an attempt.
  - It makes ten attempts, ten seconds apart.
  - It gives the instance a random password for the agent, a nonce, the API addresses, the CA certificate, the constraints, the tags, and the agent binaries.
  - It records the instance ID once. A machine whose instance is recorded cannot be given another instance without {ref}`reprovisioning <machine-reprovisioning>`.
  - If it cannot record the instance, it stops the instance.
  - The number of provisioning workers is the `num-provision-workers` model configuration key.
- *Errors:* A start that fails on its last attempt shows as the instance status `provisioning error`, with the cloud's message, and the machine status `error`. *Remediation:* Fix the cause the message names, and {ref}`retry provisioning <machine-retrying>`.

(machine-script)=
### Getting provisioning scripts

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`
```

Getting provisioning scripts returns a shell script that installs and starts the machine agent on a machine that Juju did not start, for manual provisioning. A client first adds the machine with an instance ID that starts with `manual:`, its hardware, and a nonce, and then runs the script on the host.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model.
- *Rules:*
  - The machine must exist with a recorded architecture.
  - The model must have an agent version with binaries for the machine's operating system and architecture.
  - Every call sets a new random password for the machine's agent, so a script that was fetched before is no longer valid.
  - A request can leave out the package update and upgrade commands. Otherwise the model's `enable-os-refresh-update` and `enable-os-upgrade` keys decide.
- *Errors:* The client receives each with no code unless one is given:
  - **`getting instance config: machine "<name>" not found`**: *Trigger:* The machine does not exist. The client receives it with the code `not found`. *Remediation:* {ref}`Add the machine <machine-adding>` first.
  - **`getting instance config: arch is not set for "<name>"`**: *Trigger:* The machine has no recorded hardware. *Remediation:* Add the machine with its hardware characteristics.
  - **`getting instance config: no agent version set in model configuration`**: *Trigger:* The model has no agent version. *Remediation:* Set an agent version on the model.
  - **`getting instance config: finding agent binaries: <reason>`**: *Trigger:* The controller has no binaries for the model's agent version and the machine's operating system and architecture. *Remediation:* Make binaries available, or {ref}`upgrade the model <model-upgrading>`.
  - **`permission denied`**: *Trigger:* The caller lacks write access. *Remediation:* Ask a model administrator.

(machine-retrying)=
### Retrying provisioning of machines

```{ibnote}
See also: {ref}`Juju | Retry provisioning for a failed machine <retry-provisioning-for-a-failed-machine>`
```

Retrying provisioning of machines marks the provisioning error of a machine as transient, so that the provisioner starts an instance for it again within a minute. A request names machines, or all machines.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. The change block must not be on.
- *Rules:*
  - A machine whose instance status is not `provisioning error` is skipped when the request asks for all machines, and is an error when the request names it.
  - A machine ID that does not exist in the model is ignored without an error.
  - The status and its message stay as they were. The retry only sets a `transient` mark in the status data. The provisioner then starts a new instance within a minute.
  - Once a minute the provisioner looks for machines whose provisioning failed with an error marked as transient, and starts them again.
- *Errors:*
  - **`machine <name> is not in an error state (<status>)`**: *Trigger:* A named machine is not in a provisioning error. *Remediation:* Retry only machines whose status shows a provisioning error.
  - **`"<tag>" is not a valid machine tag`**: *Trigger:* The request names something other than a machine. *Remediation:* Name machines.
  - **`permission denied`**: *Trigger:* The caller lacks write access. *Remediation:* Ask a model administrator.
  - **`operation is blocked`**: *Trigger:* The change block is on. *Remediation:* Remove the block.

(machine-reprovisioning)=
### Reprovisioning machines

Reprovisioning machines replaces the lost cloud instance of a machine with a new one. The machine keeps its name. It detaches the instance that is gone and puts the machine back to `pending`, so that the compute provisioner starts a new instance. The call is available on controllers that serve version 12 of the machine manager facade.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. The change block must not be on.
- *Rules:*
  - A machine can be reprovisioned only if it is alive, is not a container, a controller, or a manually provisioned machine, hosts no containers, has no model-scoped storage attached, and has no reprovision request pending.
  - The machine's agent must be gone.
  - The cloud must report that the instance is gone or not running: a running instance refuses the request.
  - No model migration and no controller upgrade can be in progress.
  - The reprovision clears the machine's addresses and network records, the block devices that no volume uses, its machine-scoped storage, and the instance's tags. Its units leave their relation scopes and are replaced with new units. The reprovision also clears the machine's nonce, hostname, and agent start time and empties its instance record.
  - It sets both the machine status and the instance status to `pending`, with the message `reprovisioning requested` and the old instance ID in the status data. The provisioner removes the marker when the new instance is recorded.
- *Errors:* The client receives each with no error code:
  - **`model migration is in progress; cannot reprovision machine "<name>"`** and **`controller upgrade is in progress; cannot reprovision machine "<name>"`**: *Trigger:* A migration or upgrade is running. *Remediation:* Wait for it to finish.
  - **`reprovisioning machine "<name>": <reason>`**: *Trigger:* The reason is one of `reprovision request already exists`, `machine is not alive`, `machine is a container`, `machine is a controller machine`, `machine is manually provisioned`, `machine hosts child containers`, `model-scoped storage attached to machine`, `storage scope is ambiguous`, or `machine storage is not alive`. *Remediation:* Remove the child containers or the model-scoped storage, or wait for the pending request. A container, controller, or manual machine cannot be reprovisioned.
  - **`machine "<name>": machine not provisioned`**: *Trigger:* The machine has no instance. *Remediation:* There is nothing to replace: retry provisioning instead.
  - **`machine "<name>": machine agent is still present`**: *Trigger:* The machine's agent is connected. *Remediation:* Stop the agent or the instance, and retry.
  - **`machine "<name>" instance "<id>": machine provider instance is running`**: *Trigger:* The cloud reports the instance running. *Remediation:* Stop or delete the instance in the cloud, and retry.
  - **`reprovisioning machine "<name>": machine cloud instance changed`**: *Trigger:* The instance was replaced while the request ran. *Remediation:* Retry.
  - **`cannot create replacement for reprovisioned unit "<old>" as "<new>": replacement unit name already exists; the application unit sequence is behind existing unit names`**: *Trigger:* A replacement unit's name is taken. *Remediation:* None known: report it.
  - **`operation is blocked`**: *Trigger:* The change block is on. *Remediation:* Remove the block.
  - **`reprovisioning machines is not supported by this controller`**: *Trigger:* The controller does not serve version 12. The client receives it with the code `not supported`. *Remediation:* Upgrade the controller.

(machine-listing)=
### Showing the status of machines

```{ibnote}
See also: {ref}`Juju | List all machines <list-all-machines>`, {ref}`Juju | View details about a machine <view-details-about-a-machine>`, {ref}`Juju | View the status of a machine <view-the-status-of-a-machine>`
```

Showing the status of machines returns the machines of a model, with their containers nested under them, as one part of the model's full status. Each machine has its ID, hostname, DNS name, addresses, base, hardware, constraints, agent status, and instance status, and the controller machines have their jobs and whether they hold a vote in the database cluster.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model, or with {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A machine's instance ID is `pending` until the cloud has returned an instance.
  - The agent status is `down`, with the message `agent is not communicating with the server`, whenever neither the machine's agent nor the agent of one of its containers is connected, unless the stored status is `pending` or `stopped`.
  - A machine hosts units, so its jobs include `host-units`, and a machine of the controller model that is a controller also has `manage-model`.
  - The network interfaces appear only after the instance ID is set.
- *Errors:*
  - **`permission denied`**: *Trigger:* The caller has neither read access to the model nor superuser access to the controller. *Remediation:* Ask a model administrator.

(machine-removing)=
### Removing machines

```{ibnote}
See also: {ref}`Juju | Remove a machine <remove-a-machine>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

Removing machines marks machines dying together with their containers and units, and schedules the removal of each. A machine is deleted when its instance has been stopped and everything on it is gone.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. Neither the remove block nor the change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) may be on.
- *Rules:*
  - Without the force option, a machine that hosts containers or units is refused.
  - A request can ask for a dry run, which reports the units, storage, and containers that the removal affects and changes nothing, and can ask to keep the instance, so that the cloud keeps the instance running.
  - With the force option, the machine, its containers, their units, and the storage the machine owns go to dying together, and the removal does not wait for them. The maximum wait defaults to one minute: Juju first tries a clean removal, and forces the removal after the wait if the clean one has not finished. A wait of zero forces the removal at once. Without the force option the wait is ignored.
  - The machine's agent sees the machine dying, sets its status `stopped`, and marks the machine dead when it hosts no containers, units, or storage. A machine that has no instance is marked dead by the provisioner.
  - The provisioner then stops the instance in the cloud, unless the request asked to keep it, and marks the instance dead.
  - The removal deletes the machine's records, in the end the machine itself, when the machine and the instance are dead and nothing depends on them. It waits for the containers of a host to be gone first.
  - Removing the last unit of a machine, and destroying a model, remove machines in the same way.
- *Errors:*
  - **`removing machine: machine "<uuid>": cannot set machine "<uuid>" to dying without force: machine has containers`** and **`...: machine has units`**: *Trigger:* The machine hosts containers or units and the request has no force option. *Remediation:* Remove the containers and units first, or use the force option.
  - **`getting machine UUID: getting UUID for machine "<name>": machine "<name>": machine not found`**: *Trigger:* The machine does not exist. *Remediation:* Check the name.
  - **`classifying storage for machine "<name>": <reason>`**: *Trigger:* The controller cannot tell which storage the removal would affect. *Remediation:* Check the storage, then retry.
  - **`operation is blocked`** with the operator's message: *Trigger:* The remove or change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.
  - **`permission denied`**: *Trigger:* The caller lacks write access. *Remediation:* Ask a model administrator.

## Machines and model migration

A reprovision is refused while a migration is in progress.
