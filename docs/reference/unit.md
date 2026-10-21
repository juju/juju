---
myst:
  html_meta:
    description: "Juju unit reference: what a unit is, the records the model keeps about it, the leader, the changes that unit agents follow, and the operations that add, show, resolve, and remove units and that unit agents use to report and to claim leadership."
---

(unit)=
# Unit

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>`
```

A **unit** is one running instance of an {ref}`application <application>`: the process that runs the application's {ref}`charm <charm>` on one {ref}`machine <machine>` or in one Kubernetes pod, together with the records that the {ref}`model <model>` keeps about it. A unit belongs to one application and is named `<application>/<number>`, such as `mysql/0`. A unit comes into being when units are added to an application, when a principal unit's relation creates a subordinate, or when an application is deployed, and goes when it is removed.

(unit-database-schema)=
## Unit database schema

```{ggarch}
:file: ../juju.ggarch
:view: Unit database schema
:caption: The model database keeps one record for each unit. A unit belongs to one application and runs one charm. An application and a charm each have many units. A unit has at most one workload status, one agent status, one workload version, one agent version, one agent presence, one resolved marker, one unit state, one principal (the unit it accompanies, which is another unit, so a unit can be the principal of many units), and, on a Kubernetes model, one pod and one pod status. A unit has many charm state items and many relation state items. A pod has many ports. The status values, the resolve modes, the life values, the network node, and the password hash algorithm are described in the text and are not boxes in the figure, because they are lookups or belong to other pages. The leader is a lease in the controller database and is not in the figure.
:alt: Sixteen boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Unit is in the middle. Application is to its left (m to 1) and Charm is to its right (m to 1). Application and Charm each have an arrow mark because other pages describe them. Above Unit is a row of five boxes, from left to right Agent version, Agent status, Workload status (directly above Unit), Workload version, and Agent presence, each joined to Unit (1 to 1). Below Unit is a row with Resolved marker, Unit state (directly below Unit), Principal, and Pod, each joined to Unit (1 to 1), and Principal joins Unit twice, once for the subordinate (1 to 1) and once for the principal unit (m to 1). Pod status is above Pod and is joined to Unit (1 to 1). Charm state and Relation state are in a second row below Unit and are joined to Unit (m to 1). Pod port is below Pod (m to 1).
```

The unit database schema is the set of records that the {ref}`model database <database>` keeps about the units of a model. The records, with the rules that apply to each, are:

- **Unit:** The unit's name, its life (`alive`, `dying`, or `dead`), the application it belongs to, the charm it runs, its network node (the record that its addresses hang from), and an optional hash of the password that its agent logs in with. A unit's name is its application's name, a slash, and a number. Juju numbers the units of an application from 0, in increasing order, from one sequence for each application.
  - *Rules:*
    - A unit name appears once in the model.
    - A password hash appears once among the units. A unit with no password has no hash.
    - A unit's life never moves backward. A `dying` unit never becomes `alive`. A `dead` unit never becomes `dying`.
    - A unit has an application, a charm, and a network node.
(leader-unit)=
- **Leader:** The unit that holds the lease of its application, which makes it the **leader unit** (the leader) of the application. The lease is a record in the {ref}`controller database <database>`, named by the application. The leader is the one unit that may change what is shared by all units of the application: the application's status, its secrets, and its settings in a relation. An application has at most one leader. A unit agent claims leadership (see {ref}`requesting unit leadership <unit-claiming-leadership>`).
  - *Rules:*
    - A model has one lease of the type `application-leadership` for each application name.
- **Principal:** The link from a subordinate unit to the principal unit that it accompanies. A principal unit has no such record. The {ref}`relation <relation>` page describes how a subordinate unit comes into being.
  - *Rules:*
    - A unit has at most one principal.
    - A principal is a unit of the model.
- **Workload status:** The status of the charm's workload, with a message, optional data, and the time of the last change. The status is one of `unset`, `unknown`, `maintenance`, `waiting`, `blocked`, `active`, `terminated`, or `error`. A charm sets `maintenance`, `blocked`, `waiting`, and `active`. Juju sets `error`, `terminated`, `unknown`, and `unset` (see {ref}`status <unit-status>`).
  - *Rules:*
    - A unit has at most one workload status.
    - The status is one of the eight values.
- **Agent status:** The status of the unit agent, with a message, optional data, and the time of the last change. The status is one of `allocating`, `executing`, `idle`, `error`, `failed`, `lost`, or `rebooting`. The changes of status are described in {ref}`setting unit statuses <unit-setting-status>`.
  - *Rules:*
    - A unit has at most one agent status.
    - The status is one of the seven values.
- **Agent presence:** The time at which the unit's agent was last seen logged in to the controller. The record exists while the agent is logged in.
  - *Rules:*
    - A unit has at most one agent presence record.
- **Workload version:** The version of the software that the unit runs, as the charm reports it (see {ref}`setting workload versions <application-setting-workload-version>`).
  - *Rules:*
    - A unit has at most one workload version.
- **Agent version:** The version and the architecture of the agent binary that the unit's agent reports that it runs.
  - *Rules:*
    - A unit has at most one agent version.
- **Resolved marker:** The request to move a unit out of its error status, with a resolve mode: `retry-hooks` runs the hook that failed again, and `no-hooks` skips it. The marker exists until the unit agent clears it.
  - *Rules:*
    - A unit has at most one resolved marker.
    - The resolve mode is `retry-hooks` or `no-hooks`.
- **Unit state:** The state that the unit agent keeps in the controller so that it can resume after a restart: the state of the agent's operations, of its storage, and of its secrets, each a piece of text that the agent writes.
  - *Rules:*
    - A unit has at most one unit state record.
- **Charm state:** One key and value that the charm stored for its unit (see the `state-set` {ref}`hook tool <hook-command-state-set>`).
  - *Rules:*
    - A unit has each key once.
- **Relation state:** One key and value that the agent stores for each relation of its unit.
  - *Rules:*
    - A unit has each key once.
- **Pod:** On a Kubernetes model, the name that the cloud gives to the pod of the unit, which is the provider ID.
  - *Rules:*
    - A unit has at most one pod.
- **Pod port:** One port that the pod of a unit exposes.
  - *Rules:*
    - A pod lists a port once.
- **Pod status:** On a Kubernetes model, the status that Kubernetes reports for the unit's pod, with a message, optional data, and the time of the last change. The status is one of `unset`, `waiting`, `blocked`, `running`, or `error`.
  - *Rules:*
    - A unit has at most one pod status.
    - The status is one of the five values.

The records that other pages own refer to a unit: its {ref}`application <application>`, {ref}`charm <charm>`, {ref}`relations <relation>` and settings, {ref}`secrets <secret>`, {ref}`storage <storage>` attachments, resources, and the tasks of an {ref}`action <action>`.

(unit-change-streams)=
## Unit change streams

Unit change streams are the notifications that the {ref}`agent <agent>` of a unit follows, so that it can run the {ref}`hooks <hook>` of its charm at the right time. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). The streams that belong to the unit itself are:

- **Unit record:** `WatchUnit` reports a change of the unit's own record, of its principal, and of its resolved marker. The unit agent then reads the life of the unit again.
- **Resolved marker:** `WatchUnitResolveMode` reports that a client set or cleared the resolved marker of the unit. The unit agent then reads the marker.
- **Addresses:** `WatchUnitAddressesHash` reports a change of the hash of the unit's addresses and of the endpoint bindings of the spaces that the addresses belong to. The unit agent runs the `config-changed` {ref}`hook <hook-config-changed>` when the hash differs from the one it last saw.
- **Agent version:** `WatchAPIVersion` of the `Upgrader` facade reports that the model's target agent version changed. It is served on a machine model.
- **Leadership:** The unit agent's leadership tracker renews the lease of its unit, or learns through `BlockUntilLeadershipReleased` that the lease of another unit has expired. The agent runs the `leader-elected` hook when its unit becomes the leader. It runs the `leader-deposed` hook when its unit stops being the leader.

A unit agent follows other changes too: the configuration of its application (see {ref}`application change streams <application-change-streams>`), its relations (see {ref}`relation change streams <relation-change-streams>`), the tasks queued for it (see {ref}`action change streams <action-change-streams>`), and its secrets (see {ref}`secret change streams <secret-change-streams>`).

(unit-operations)=
## Unit operations

Unit operations are the actions that {ref}`clients <client>` and unit agents can take on units through the controller API (the set of calls that clients and agents make to the controller). A **{ref}`unit agent <unit-agent>`** is the process that runs the charm of a unit. It logs in as its unit and acts only for that unit. An agent that logs in as an application acts for any unit of that application. No model access level and no block applies to a unit agent. Clients add, show, find the leader of, resolve, and remove units. Unit agents log in, set statuses, report versions and passwords, claim leadership, keep their state, read their details, and end their units. Adding relations is described on the {ref}`relation <relation>` page, and running actions on the {ref}`action <action>` page. The hooks that Juju runs in response are described on the {ref}`hook <hook>` page.

(unit-adding)=
### Adding units

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>` > {ref}`Add a unit <add-a-unit>`
```

Adding units creates one or more units of an application on a machine model. The request names the application and the number of units, and can give a placement directive and existing storage for each unit. The answer lists the names of the new units. Each new unit gets a machine. The {ref}`compute provisioner <machine-provisioning>` then starts an instance for it. A Kubernetes model sets the scale of the application instead (see {ref}`scaling applications <application-scaling>`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - The number of units is at least one.
  - The application must exist and be alive.
  - A new unit is named with the next number in the application's sequence, is `alive`, has the agent status `allocating`, and has the workload status `waiting` with the message `waiting for machine`.
  - Placement directives apply in order. Directive number *n*, counting from 0, applies to new unit number *n*, counting from 0. A unit with no directive gets a new machine. A directive that names a machine (see {ref}`machine designations <machine-designations>`) puts the unit on that machine. The unit then shares the machine's network node.
  - The cloud provider checks each unit's base, placement, and constraints before the units are created.
  - Existing storage can be attached only when one unit is added. A storage instance that does not exist cannot be attached.
  - A model that runs on Kubernetes refuses the request.
- *Errors:* The client receives each as the error of the call:
  - **`adding units to a container-based model is not supported`** (code `not supported`): *Trigger:* The model runs on Kubernetes. *Remediation:* Set the scale of the application.
  - **`must add at least one unit`** and **`AttachStorage is non-empty, but NumUnits is <n>`** (code `not valid`): *Trigger:* The request asks for no units, or attaches storage to several units. *Remediation:* Ask for one or more units, and for one unit when attaching storage.
  - **`storage instance "<id>" does not exist`** and **`application "<name>" not found`** (code `not found`): *Trigger:* The storage or the application does not exist. *Remediation:* Use the ID of existing storage and the name of an existing application.
  - **`adding <n> units to application "<name>": adding units to application "<name>": application is not alive`**: *Trigger:* The application is dying. *Remediation:* Wait until the application is gone, then deploy it again.
  - **`adding <n> units to application "<name>": adding units to application "<name>": application is dead`**: *Trigger:* The application is dead. *Remediation:* Deploy the application again.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(unit-showing)=
### Showing units

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>` > {ref}`View details about a unit <view-details-about-a-unit>`
```

Showing units returns the details of units. The request names units, or applications whose units are all returned. The answer holds, for each unit, its tag, life, workload version, charm URL, whether it is the leader, and the relation data of its application. On a machine model it holds the unit's machine, public address, and opened ports. On a Kubernetes model it holds the provider ID of the pod, its address, and its ports. A unit's statuses are in the status of the model (see {ref}`showing unit statuses <unit-showing-status>`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. A caller with read access to the model can always call it.
- *Rules:*
  - A tag names a unit or an application. A tag of another kind gets an error in its place.
  - A unit is the leader when its name is the holder of its application's lease.
  - A failure for one unit does not stop the others.
- *Errors:* The client receives each in the result for the tag:
  - **`application <name> not found`** and **`unit <name> not found`** (code `not found`): *Trigger:* The application or the unit does not exist. *Remediation:* Use the name of an existing application or unit.
  - **`tag "<tag>" not valid`** (code `not valid`): *Trigger:* The tag names neither a unit nor an application. *Remediation:* Name units as `unit-<application>-<number>` and applications as `application-<name>`.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller lacks read access. This fails the whole call. *Remediation:* Ask a model admin.

(unit-finding-leader)=
### Finding leaders

Finding leaders returns the name of the {ref}`leader unit <leader-unit>` of an application.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. A caller with read access to the model can always call it.
- *Rules:*
  - The request names one application by tag.
  - The answer is the name of the unit that holds the application's lease. An application with no leader has none to return.
- *Errors:* The client receives each in the result:
  - **`leader for application-<name> not found`** (code `not found`): *Trigger:* The application has no leader, or does not exist. *Remediation:* Wait for a unit to claim leadership, then ask again.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller lacks read access. *Remediation:* Ask a model admin.

(unit-resolving)=
### Setting resolved markers

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>` > {ref}`Mark unit errors as resolved <mark-unit-errors-as-resolved>`
```

Setting resolved markers marks the units that are in error as resolved, so that their agents move on. The request names units, or asks for all units, and says whether the failed hook runs again. The controller records the resolved marker. The unit agent follows it (see {ref}`reading resolved markers <unit-resolved-marker>`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request names units or asks for all units, never both.
  - A request that asks to retry gets the resolve mode `retry-hooks`. Any other request gets `no-hooks`.
  - A unit can be resolved only when its agent status is `error`. A marker that exists is replaced.
  - A request for all units marks every unit whose agent status is `error` and ignores the others.
  - A failure for one named unit does not stop the others.
- *Errors:* The client receives the first as the error of the call and the others in the result for the unit:
  - **`cannot resolve all units and specific units`** (code `bad request`): *Trigger:* The request names units and asks for all units. *Remediation:* Name units, or ask for all units.
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the name of an existing unit.
  - **`resolving unit: checking unit "<uuid>" status: unit is not in error state`**: *Trigger:* The unit's agent status is not `error`. *Remediation:* Resolve only units that are in error.
  - **`resolving unit: checking unit "<uuid>" status: unit agent status not found`**: *Trigger:* The unit has no agent status record. *Remediation:* Wait for the unit agent to report a status, then resolve.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(unit-removing)=
### Removing units

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>` > {ref}`Remove a unit <remove-a-unit>`
```

Removing units makes units dying and schedules their removal on a machine model. The request names units by tag. It can ask to destroy the units' storage, to force the removal, to wait before the force applies, and to do a dry run. The unit agent of each unit then runs the `stop` and `remove` hooks and leaves its relations. Juju deletes the unit's records. A Kubernetes model removes units by setting the scale (see {ref}`scaling applications <application-scaling>`). Removing an application removes its units (see {ref}`removing applications <application-removing>`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The remove block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A model that runs on Kubernetes refuses the request.
  - A unit of a subordinate application cannot be removed on its own. Removing the relation removes the subordinate units.
  - A dry run changes nothing. It returns the storage that the removal would destroy and the storage that it would detach.
  - The unit becomes `dying` at once. Its storage attachments become dying too. The storage that the unit owns becomes dying when the request destroys storage. A machine becomes dying when the removed units were the last units on it and no container runs on it. A removal job is scheduled for each of them.
  - The time to wait applies only to a forced removal. A removal that is not forced ignores it.
  - A removal that is not forced makes the unit `dead` once it has no relation in scope and no storage attachment, and then deletes it. Otherwise the job runs again later. A removal that is not forced waits for the unit's subordinate units to be gone before it deletes the unit. A forced removal deletes the unit regardless.
  - A forced removal does not wait for the unit's relations and storage. It leaves the unit's relations itself.
  - Before deleting the unit, the job revokes its leadership, deletes the secrets that it owns, and, for a unit of the controller application in the controller model, deletes the controller node.
  - Deleting the unit deletes its tasks and annotations, its pod and ports, its status, workload version, agent version, presence, resolved marker, state, ports opened, resources, storage directives, and secret consumer records. The charm is deleted afterward when no other application or unit uses it.
- *Errors:* The client receives each in the result for the unit, except the first and the last:
  - **`removing units on a non-container model not supported`** (code `not supported`): *Trigger:* The model runs on Kubernetes. *Remediation:* Set the scale of the application.
  - **`unit "<name>" is a subordinate, to remove use remove-relation. Note: this will remove all units of "<application>"`**: *Trigger:* The unit belongs to a subordinate application. *Remediation:* {ref}`Remove the relation <relation-removing>`.
  - **`application <name> not found`** and **`unit "<name>" not found`** (code `not found`): *Trigger:* The application or the unit does not exist. *Remediation:* Use the name of an existing unit.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the remove block is on. These fail the whole call. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(unit-showing-status)=
### Showing unit statuses

```{ibnote}
See also: {ref}`Juju | Manage units <manage-units>`
```

Showing unit statuses reports the workload status and the agent status of each unit in the status of a model. A unit agent can read the workload status of its own unit too. The status page describes the meaning of each value (see {ref}`unit status <unit-status>`).

The operation's required access, rules, and errors are:

- *Required access:* For a client, {ref}`read access <user-access-model-read>` to the model, or superuser access to the controller. For a unit agent, its own unit.
- *Rules:*
  - A unit whose agent is not logged in (it has no agent presence) shows the agent status `lost` with the message `agent is not communicating with the server`. It shows the workload status `unknown` with a message that points to the debug log, unless the workload is `error` or `terminated`.
  - A unit whose agent is `allocating` is treated as present.
  - A unit whose agent status is `error` shows the agent as `idle` and the workload as `error`, with the agent's message and data.
  - On a Kubernetes model, the status of the pod can replace the workload status. A workload that is `terminated`, `blocked`, or `maintenance` always shows. A pod that is `blocked` replaces the others. A pod that is `waiting` replaces a workload that is `active`. A pod that is `running` replaces a workload that is `unset`, or `waiting` with a message that Juju set, and otherwise the workload shows.
- *Errors:* The caller receives each in the result for the unit:
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* A unit agent asked about a unit that does not exist. *Remediation:* Ask about the unit that the agent runs.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* A unit agent asked about another unit, or a client lacks read access. *Remediation:* Ask about the unit that the agent runs, or ask a model admin.

(unit-logging-in)=
### Logging in

A unit agent logs in to the controller as its unit, with the password of the unit. The login records the unit's presence.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The password must match the hash that the controller keeps for the unit (see {ref}`setting passwords <unit-setting-passwords>`).
  - A successful login writes the unit's agent presence. The record is deleted when the connection ends. A connection from a controller to a model does not change the presence of the model's units.
  - A facade that serves unit agents refuses every other kind of caller.
- *Errors:* The agent receives each as the error of the login:
  - **`invalid entity name or password`** (code `unauthorized access`): *Trigger:* The password is invalid or does not match, or the unit does not exist. *Remediation:* Set a new password with the controller's credentials, or start the agent with the password it stored.
  - **`unit authentication: invalid request`**: *Trigger:* The password is empty. *Remediation:* Send the password.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* A caller that is not a unit agent used a facade for unit agents. *Remediation:* Log in as a unit.

(unit-setting-passwords)=
### Setting passwords

A unit agent changes the password of its unit. The request gives the unit and the new password. The controller keeps a hash of it.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The password has at least 24 bytes.
  - The controller stores a SHA-512 hash of the password and the algorithm that made it. It replaces the earlier hash.
- *Errors:* The agent receives each in the result for the unit:
  - **`password is only <n> bytes long, and is not a valid Agent password: invalid password`**: *Trigger:* The password is shorter than 24 bytes. *Remediation:* Use a password of 24 bytes or more.
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-setting-status)=
### Setting unit statuses

A unit agent reports two statuses of its unit: its own agent status and the workload status that a charm sets with the `status-set` hook tool (see {ref}`status-set <hook-command-status-set>`). The request gives the unit, the status, a message, and data. The controller records the time of the change and adds it to the status history.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The agent status is one of `executing`, `idle`, `error`, `failed`, and `rebooting`. The agent sets `executing` when it runs a hook and `idle` when it has nothing to do. The agent status `error` needs a message. Juju sets `allocating` when it creates the unit and shows `lost` when the agent is not present, so the agent cannot set them.
  - The workload status is one of the eight values. The hook tool accepts `maintenance`, `blocked`, `waiting`, and `active`.
  - A status replaces the earlier status of its kind, with its message and data.
  - Setting the status of the application is described in {ref}`setting application status <application-setting-status>`.
- *Errors:* The agent receives each in the result for the unit:
  - **`setting status "error" without message`** and **`setting status "<status>" is not allowed`**: *Trigger:* The agent status `error` has no message, or the status is `lost` or `allocating`. *Remediation:* Give a message, or set another status.
  - **`unknown agent status "<status>"`** and **`unknown workload status "<status>"`**: *Trigger:* The status is not one of the values. *Remediation:* Use one of the values.
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-reporting-agent-version)=
### Reporting agent versions

A unit agent reports the version and the architecture of its own binary, so that the controller knows which agents have upgraded. The unit agent of a machine model reports it through the `Upgrader` facade.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The version must be valid and the architecture must be one that the model database knows.
  - The unit must not be dead.
  - The report replaces the earlier agent version.
- *Errors:* The agent receives each in the result for the unit:
  - **`agent version "<version>" supplied is not valid for tag "<tag>"`** (code `not valid`): *Trigger:* The version is not valid. *Remediation:* Report the version of the running binary.
  - **`architecture "<arch>" not support for tag "<tag>"`** (code `not supported`): *Trigger:* The architecture is unknown. *Remediation:* Report an architecture that Juju supports.
  - **`unit for tag "<tag>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-claiming-leadership)=
### Requesting unit leadership

A unit agent claims the lease of its application for its unit, to become or to stay the {ref}`leader unit <leader-unit>`. The request gives the application, the unit, and a duration in seconds. An agent that is not the leader can instead ask to be told when the lease expires.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit and its own application. An agent that logs in as an application can claim for that application.
- *Rules:*
  - The duration is at least 5 seconds and at most 5 minutes.
  - A lease that nobody holds is granted to the claimant. The holder of a lease extends it by claiming again. A claim on a lease that another unit holds is denied.
  - A unit agent claims for 60 seconds, which is twice its 30-second leadership guarantee, and renews the lease every 30 seconds. A leader that cannot renew the lease stops being the leader when the lease expires.
- *Errors:* The agent receives each in the result:
  - **`leadership claim denied`** (code `leadership claim denied`): *Trigger:* Another unit holds the lease. *Remediation:* Wait for the lease to expire, and ask again.
  - **`invalid duration`**: *Trigger:* The duration is outside the limits. *Remediation:* Claim for between 5 seconds and 5 minutes.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The unit or the application is not the one that the agent runs. *Remediation:* Claim for the unit that the agent runs.

(unit-resolved-marker)=
### Reading resolved markers

A unit agent reads the resolved marker of its unit and clears it once it has acted. When the marker is `retry-hooks`, the agent runs the hook that failed again. When the marker is `no-hooks`, it skips that hook. The marker is set by {ref}`setting resolved markers <unit-resolving>`.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - A unit with no marker has the mode `none`.
  - The agent clears the marker before it runs or skips the hook.
- *Errors:* The agent receives each in the result for the unit:
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`unit "<uuid>": unit not resolved`**: *Trigger:* The agent cleared a marker that does not exist. *Remediation:* Read the marker first.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-saving-state)=
### Writing unit state

A unit agent stores the state that it needs to resume after a restart, and reads it back. The request can give the state of the agent's operations, its storage, its secrets, the charm's state items, and the relation state items. The charm state is what the charm sets with the `state-set` hook tool (see {ref}`state-set <hook-command-state-set>`).

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - Only the parts that the request gives are written. The others stay as they are.
  - The charm state items that the request gives replace all earlier charm state items.
  - The relation state items replace all earlier ones, and only items for relations that still exist are kept.
  - A unit that has stored nothing has empty state.
- *Errors:* The agent receives each in the result for the unit:
  - **`getting unit UUID for "<name>": unit not found`**: *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-reading-details)=
### Reading unit details

A unit agent reads facts about its unit: the machine it is assigned to, its public and private addresses, its availability zone, the URL of its charm, its principal, and whether it has subordinates. The charm reads some of these through hook tools (see {ref}`unit-get <hook-command-unit-get>`).

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The availability zone of a unit comes from its machine. A unit with no machine, or a machine with no zone, has the empty zone.
  - A unit with no principal answers that it has none.
- *Errors:* The agent receives each in the result for the unit:
  - **`unit machine not assigned`** (code `not assigned`): *Trigger:* The unit has no machine. *Remediation:* Wait for the machine to be assigned.
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`unable to find machine for unit "<name>"`** (code `not found`): *Trigger:* The machine of the unit does not exist. *Remediation:* Wait for the machine to be recorded.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-setting-charm)=
### Setting the running charm

A unit agent records the charm that its unit now runs, after the agent has upgraded the unit's charm. The request gives the unit and the charm URL.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The unit must not be dead.
  - A request for the charm that the unit already runs changes nothing.
  - A change of charm updates the unit's storage to the new charm's storage directives and unlinks resources that do not belong to the new charm.
- *Errors:* The agent receives each in the result for the unit:
  - **`unit "<name>" not found`** and **`charm <charm> not found`** (code `not found`): *Trigger:* The unit or the charm does not exist. *Remediation:* Use the unit that the agent runs and a charm that the model holds.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-ending)=
### Destroying units

A unit agent ends its own unit, or the subordinates of its unit. The agent asks for the unit to be destroyed, for example when the last relation to a principal unit ends. When Juju makes a unit `dying`, the agent runs the `stop` and `remove` hooks, waits until the unit has no subordinates, deletes the content of the secrets that the unit owns, and then marks the unit `dead`. The agent reads the life of a unit with `Life`.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - Destroying a unit is the same as {ref}`removing units <unit-removing>` without destroying storage, forcing, or waiting.
  - A subordinate unit destroys itself when no alive relation with its principal's application is left.
  - Destroying all subordinates destroys each subordinate unit of the unit in the same way.
  - Marking a unit dead needs the unit to be `dying` first. A unit that is already `dead` stays `dead`.
- *Errors:* The agent receives each in the result for the unit:
  - **`unit "<name>" not found`** (code `not found`): *Trigger:* The unit does not exist. *Remediation:* Use the unit that the agent runs.
  - **`entity still alive`**: *Trigger:* The agent marked an `alive` unit dead. *Remediation:* Destroy the unit first.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of the unit that the agent runs. *Remediation:* Use the unit that the agent runs.

(unit-and-model-migration)=
## Units and model migration

A {ref}`migration <model-migrating>` moves the units of a model with their names, password hashes, principals, workload versions, statuses, agent versions, and unit state. A migration is refused while any unit is not logged in to the controller, has an agent that is not `idle` or `executing`, or has a workload that is not `active` or in a state from which it is expected to become `active`. The error lists each such unit, after `model unit(s) are not ready for migration:`. The target controller claims a new lease for each leader, for one minute from the import.
