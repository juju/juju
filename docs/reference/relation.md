---
myst:
  html_meta:
    description: "Juju relation reference: what a relation is, its kinds (peer, subordinate, cross-model), the records the model keeps about it, its status, the data its units exchange, and the operations that add, remove, suspend, list, and use relations."
---

(relation)=
# Relation

```{ibnote}
See also: {ref}`Juju | Manage relations <manage-relations>`
```

A **relation** is a connection between two {ref}`applications <application>` of a {ref}`model <model>` through one {ref}`endpoint <application-endpoint>` of each, or between an application and itself through one of its own endpoints. The {ref}`units <unit>` on the two sides of a relation use it to exchange data and to learn when a unit of the other side joins, changes its data, or leaves.

(relation-data-model)=
## Relation data model

```{ggarch}
:file: ../juju.ggarch
:view: Relation data model
:caption: The model database keeps one record for each relation. A relation has at most one status record. A status takes one of a fixed set of values, and a value has many statuses. An endpoint record belongs to one relation. A relation has an endpoint record for each application endpoint that takes part. A unit in scope (a unit that has entered the relation) belongs to one endpoint record. An endpoint record has many units in scope. Each unit in scope has many unit settings. Each endpoint record has many application settings. When a unit leaves, its settings are copied to archived settings that belong to the relation. The unit, the application endpoint, the life and scope values, the departed units, and the ingress and egress CIDRs are described in the text and are not boxes in the figure.
:alt: Eight boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Relation is in the middle. Status is above it (1 to 1) and Status value is above Status (m to 1). Archived setting is to its left (m to 1). Endpoint is to its right (m to 1). Unit in scope is to the right of Endpoint (m to 1). Application setting is below Endpoint (m to 1). Unit setting is below Unit in scope (m to 1).
```

(relation-kinds)=
The relation data model is the set of records that the {ref}`model database <database>` keeps about the relations of a model. The endpoints of a relation decide its kind: regular, peer, subordinate, or cross-model. The records, with the rules that apply to each, are:

- **Relation:** The relation's ID, its life (alive, dying, or dead; Juju deletes a dying relation and never marks it dead), its scope (`global` or `container`), and whether it is suspended and why.
  - *Rules:*
    - A relation has a UUID that is not empty.
    - A relation ID appears once in the model.
- **Endpoint:** One end of a relation: the application endpoint that takes part. The application endpoint pairs an application with one of the endpoints of its charm, as the {ref}`application <application-endpoint>` page describes. Each endpoint has a role (`provider`, `requirer`, or `peer`), an interface, and a scope of `global` or `container`. The endpoints of a relation decide its kind, which is one of:

  (regular-relation)=
  - **Regular relations** are the base case: a relation with two endpoints, one `provider` and one `requirer`, with the same interface, on two different applications of the same model. Both applications are principal applications. The key of a regular relation lists the requirer first and the provider second.

  (peer-relation)=
  - **Peer relations** are relations with one endpoint, whose role is `peer`. An application has a relation to itself, so its units are the units of the relation. Juju creates one peer relation for each peer endpoint of an application when it creates the application. The units of an application can use a peer relation to share settings, because each unit can read the settings of the others (see {ref}`reading relation data <relation-reading-data>`). A peer relation cannot be removed on its own (see {ref}`removing relations <relation-removing>`).

  (subordinate-relation)=
  - **Subordinate relations** are relations with two endpoints in which the {ref}`charm <subordinate-charm>` of one application is a subordinate charm and the relation is container-scoped. A subordinate application is one that runs on the same machine as the principal application it accompanies. A relation is container-scoped when either of its endpoints is container-scoped. When a unit of the principal application enters the scope of the relation, Juju creates a unit of the subordinate application on the machine of the principal unit, unless that principal unit already has one. The {ref}`implicit juju-info endpoint <the-implicit-juju-info-relation-endpoint>` is often the endpoint of a subordinate relation.

  (cross-model-relation)=
  - **Cross-model relations** (also called CMRs) are relations with two endpoints in which one endpoint belongs to a remote application. A remote application represents an application of another model, possibly on another controller or cloud. Cross-model relations connect, for example, databases on bare metal with applications in Kubernetes. The {ref}`offer <offer>` page describes how an application is made available to other models. A cross-model relation can be suspended and resumed. It can have egress CIDRs. Removing it also removes the units that stand for the remote side.
  - *Rules:*
    - A relation lists an application endpoint once.
- **Unit in scope:** A unit that has entered the relation. It belongs to the endpoint of the unit's application.
  - *Rules:*
    - A unit appears once for an endpoint.
- **Unit setting:** One key and value that a unit set in the relation.
  - *Rules:*
    - A unit has each key once.
    - A key is not empty.
    - A value is not empty.
- **Application setting:** One key and value that an application set in the relation, kept per endpoint.
  - *Rules:*
    - An endpoint has each key once.
    - A key is not empty.
    - A value is not empty.
- **Status:** The status of the relation, with a message and the time of the last change. The status changes when a relation is suspended or resumed (see {ref}`suspending relations <relation-suspending>`) and when a unit sets it (see {ref}`setting relation status <relation-setting-status>`).
  - *Rules:*
    - A relation has at most one status.
    - The status is one of `joining`, `joined`, `broken`, `suspending`, `suspended`, and `error`.
- **Archived setting:** One key and value from the settings that a unit had in the relation when it left, kept under the unit's name.
  - *Rules:*
    - A unit name has each key once for a relation.
- **Departed unit:** The UUID and name of a unit that left the relation. A database trigger writes the record when the unit's record in scope is deleted, so that watchers can name the unit after it is gone.
  - *Rules:*
    - A unit appears once for a relation.
- **Ingress and egress CIDRs:** The network ranges, in CIDR notation, that a cross-model relation takes traffic from (ingress) and sends traffic to (egress). A record cannot be updated. It can be inserted and deleted.
  - *Rules:*
    - A relation lists a CIDR once.

The life and the scope of a relation come from lookup tables. A unit in scope refers to a {ref}`unit <unit>`.

A relation is a row in the table `relation`. Its endpoints are rows in `relation_endpoint`. Its units in scope are rows in `relation_unit`. Its status is a row in `relation_status` whose values come from the lookup table `relation_status_type`. The unit settings are rows in `relation_unit_setting`. The application settings are rows in `relation_application_setting`. The archived settings are rows in `relation_unit_setting_archive`. The departed units are rows in `relation_unit_departure`. The CIDRs are rows in `relation_network_ingress` and `relation_network_egress`. The view `v_relation_status` shows each status by name. The view `v_relation_endpoint` shows each endpoint with its application, endpoint name, interface, and role. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(the-implicit-juju-info-relation-endpoint)=
### The implicit `juju-info` relation endpoint

Every charm has an endpoint named `juju-info`, with the role `provider`, the interface `juju-info`, and the scope `global`. Juju adds it to a charm that does not declare an endpoint of that name. The endpoint lets a {ref}`subordinate charm <subordinate-charm>` attach to a principal application that exposes no other suitable interface, so it is mostly useful for {ref}`machine charms <machine-charm>`.

A subordinate charm consumes the `juju-info` endpoint by declaring an endpoint with the role `requirer`, the interface `juju-info`, and the scope `container`. The container scope puts the unit of the subordinate application on the machine of the principal unit. The usual name for that endpoint is `juju-info`, but any name works. General-purpose machine subordinates that have no application-specific data to exchange use this endpoint, for example [`ntp`](https://charmhub.io/ntp), [`telegraf`](https://charmhub.io/telegraf), [`nrpe`](https://charmhub.io/nrpe), and [`canonical-livepatch`](https://charmhub.io/canonical-livepatch).

When a request names only the two applications and several pairs of their endpoints can connect, the request is ambiguous and fails. A subordinate that declares an endpoint matching an endpoint of the principal, besides its `juju-info` endpoint, is such a case. Name the endpoint of each application to select the pair (see {ref}`adding relations <relation-adding>`).

(relation-identification)=
### Relation identification

A relation is named by its **relation ID**, the number in the data model above, or by its **relation key**. Juju assigns relation IDs in increasing order from one sequence for the whole model. The key lists the endpoints as `<application>:<endpoint>`. The key of a relation with two endpoints lists them separated by a space, the requirer first and the provider second. The key of a peer relation has one endpoint. In a hook, a relation is also named `<endpoint>:<relation ID>`.

(relation-change-streams)=
## Relation change streams

Relation change streams are the notifications that the {ref}`agent <agent>` of a unit follows, so that it can run the relation {ref}`hooks <hook>` at the right time. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). A unit's agent follows the relations of its unit and the units on the other side of each relation:

- **Relations of a unit:** `WatchUnitRelations` reports the relations of a unit and the changes of their life and suspended state. For a principal unit, these are all the relations of its application. For a subordinate unit, these are the relations with the application of its principal unit.
- **Units of a relation:** `WatchRelationUnits` reports, for one unit and one relation, the units of the other side that enter and leave the scope, and the changes to their settings and to the application settings of the other side.

(relation-operations)=
## Relation operations

Relation operations are the actions that {ref}`clients <client>` and the {ref}`agents <agent>` of units can take on relations through the controller API (the set of calls that clients and agents make to the controller). Clients add, remove, suspend, resume, and list relations. Agents enter and leave a relation, read and write its data, set its status, and look relations up. Offering an application to other models is described on the {ref}`offer <offer>` page. The hooks that Juju runs in response are described on the {ref}`hook <relation-hooks>` page.

(relation-adding)=
### Adding relations

```{ibnote}
See also: {ref}`Juju | Manage relations <manage-relations>`
```

Adding relations creates a relation between two endpoints of two applications of a model. The request names two endpoints as `<application>` or `<application>:<endpoint>`.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - A request names exactly two endpoints. A name without an endpoint selects the endpoint of that application that can connect to the other side.
  - The answer lists the endpoint of each application with its name, role, interface, whether it is optional, its limit, and its scope.
  - Two endpoints can be related when they belong to different applications, have the same interface, and have opposite roles. A peer endpoint cannot be added this way.
  - A request that matches more than one pair of endpoints is refused. A request that matches none is refused.
  - A pair with a container-scoped endpoint is a match only when at least one of the two applications is a subordinate.
  - Both applications must be alive.
  - A relation with a container-scoped endpoint needs the bases (the operating system and channel) of the two applications to be compatible.
  - An endpoint with a limit takes no more relations than the limit.
  - A relation that exists between the two endpoints is not added again.
  - Both applications cannot be remote.
  - A request can add egress CIDRs. Egress CIDRs are accepted only for a cross-model relation. Each CIDR must be valid.
  - A new relation is alive, has the next relation ID, and has the status `joining`. Its scope is `container` when either endpoint is container-scoped and `global` otherwise.
- *Errors:* The client receives each with this prefix, except the first and the last: `adding relation between endpoints "<endpoint 1>" and "<endpoint 2>": `.
  - **`a relation should have exactly two endpoints`** (code `bad request`): *Trigger:* The request does not name two endpoints. *Remediation:* Name two endpoints.
  - **`parsing endpoint identifier "<name>": expected endpoint of form <application-name>:<endpoint-name> or <application-name>`**: *Trigger:* A name has more than one colon. *Remediation:* Write the name as `<application>` or `<application>:<endpoint>`.
  - **`cannot relate endpoints "<name 1>" and "<name 2>": no candidates for <name>: relation endpoint not found`**: *Trigger:* An application or an endpoint does not exist. *Remediation:* Check the names against the {ref}`relations <relation-listing>` and the endpoints of the applications.
  - **`cannot relate endpoints "<name 1>" and "<name 2>": no compatible endpoints found between applications`**: *Trigger:* The endpoints have different interfaces, the same role, or a container scope with no subordinate application. *Remediation:* Choose endpoints with the same interface and opposite roles.
  - **`cannot relate endpoints "<name 1>" and "<name 2>": ambiguous relation: "<name 1> <name 2>" could refer to "<pair>"; "<pair>"`**: *Trigger:* The names match more than one pair of endpoints. *Remediation:* Name the endpoint of each application.
  - **`already exists`** (code `already exists`): *Trigger:* The two endpoints are already related. *Remediation:* Use the existing relation.
  - **`quota limit exceeded`** (code `quota limit exceeded`): *Trigger:* The relation would exceed the limit of an endpoint. *Remediation:* {ref}`Remove a relation <relation-removing>` of that endpoint first.
  - **`cannot add relation "<name 1>", "<name 2>": relation <endpoint 1> <endpoint 2>: application <name> is not alive`**: *Trigger:* An application is dying. *Remediation:* Wait until the application is gone, then deploy it again.
  - **`cannot add relation "<name 1>", "<name 2>": relation <endpoint 1> <endpoint 2>: no compatible bases found for application "<name 1>" and "<name 2>"`**: *Trigger:* The bases of the two applications are not compatible for a container-scoped relation. *Remediation:* Deploy the subordinate application on the base of the principal.
  - **`unable to integrate two cross model applications together`**: *Trigger:* Both applications are remote. *Remediation:* Relate a remote application to an application of this model.
  - **`CIDR cannot be empty`** and **`CIDR "<cidr>" is not valid: <reason>`** (code `not valid`): *Trigger:* A CIDR is empty or not valid. *Remediation:* Write each CIDR in CIDR notation, such as `10.0.0.0/24`.
  - **`integrating via subnets for non cross model relations`** (code `not supported`): *Trigger:* The request has CIDRs and the relation is not cross-model. *Remediation:* Leave out the CIDRs.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. These have no prefix. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(relation-removing)=
### Removing relations

```{ibnote}
See also: {ref}`Juju | Manage relations <manage-relations>`
```

Removing relations ends a relation between two applications and deletes its records. The request names the relation by its two endpoints or by its relation ID. A request can force the removal and can give a time to wait for the units to leave before the removal is forced.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. Neither the remove block nor the change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) may be on.
- *Rules:*
  - A request names two endpoints, or no endpoint and a relation ID. A peer relation cannot be named.
  - Names without endpoints select the relation that exists between the two applications. When several relations exist between them, the request is refused.
  - The relation becomes dying at once. A dying relation accepts no new unit. The relation never becomes dead: it goes from dying to gone.
  - A removal that is not forced deletes the relation when no unit is in its scope. While more than one unit is in scope, the removal waits for them to leave.
  - A removal that is not forced also deletes the relation when exactly one unit is in scope, that unit is not alive, and its workload status is `error` or `blocked`. The unit is departed from the relation.
  - A forced removal departs every unit from the relation and then deletes it. The wait applies only to a forced removal. With a wait, Juju first tries the removal that is not forced, and forces it after the wait.
  - Deleting the relation deletes its endpoints, its application settings, the archived unit settings, the departed units, its status, the secret permissions scoped to the relation, and the relation state that the units keep for hooks.
  - A cross-model relation is removed in the same way, with the units that stand for its remote side. A request for a cross-model relation that is already gone succeeds.
- *Errors:* The client receives each without a prefix:
  - **`relation with endpoints [<endpoint 1> <endpoint 2>] or id <id> not found`** (code `not found`): *Trigger:* No relation matches. The endpoints are empty when the request names an ID. *Remediation:* {ref}`List the relations <relation-listing>` and use an existing one.
  - **`endpoints ["<name 1>" "<name 2>"] are ambiguous, specify relation endpoints or id`** (code `bad request`): *Trigger:* Several relations exist between the two applications. *Remediation:* Name the endpoints or the relation ID.
  - **`removing a peer relation`**: *Trigger:* The request names one endpoint. *Remediation:* Name two endpoints.
  - **`invalid endpoint length: <n>, must be 2 or 0 with relation ID provided`**: *Trigger:* The request names more than two endpoints. *Remediation:* Name two endpoints or a relation ID.
  - **`cannot remove a peer relation`**: *Trigger:* The relation ID is that of a peer relation. *Remediation:* Name a regular, subordinate, or cross-model relation.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the remove block or the change block is on. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(relation-suspending)=
### Suspending and resuming relations

Suspending relations stops a cross-model relation without removing it, and resuming relations starts it again. A request lists relations by ID. Each entry gives the ID, whether the relation is suspended, and a message. The call answers with one result for each entry.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - Only a cross-model relation can be suspended.
  - An entry that asks for the state the relation already has changes nothing.
  - Suspending marks the relation suspended, with the message as the reason, and sets the status `suspending` with the message. Resuming marks the relation not suspended and sets the status `joining` with no message. These changes are not checked against the {ref}`transition table <relation-status>`.
- *Errors:* The client receives each in the result for the entry, except the last:
  - **`relation <id> not found`** (code `not found`): *Trigger:* No relation has the ID. *Remediation:* {ref}`List the relations <relation-listing>` and use an existing ID.
  - **`relation must be a remote relation to be suspended`**: *Trigger:* The relation is not cross-model. *Remediation:* Choose a cross-model relation.
  - **`permission denied`** (code `unauthorized access`) and **the operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The caller lacks write access, or the change block is on. These fail the whole call. *Remediation:* Ask a model admin, or ask the operator to lift the block.

(relation-listing)=
### Listing relations

```{ibnote}
See also: {ref}`Juju | Manage relations <manage-relations>`
```

Listing relations returns the relations of a model as part of the model's status. Each relation has its ID, key, interface, scope, endpoints, and status. Each endpoint has its application, endpoint name, role, and whether the application is a subordinate. The status has its value, message, data, and the time of the last change.

The operation's required access and rules are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model, or {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A status request with filters lists only the relations whose applications both match the filters.

(relation-scope-operation)=
### Entering and leaving relation scopes

Entering scope records that a unit takes part in a relation, and leaving scope records that it no longer does. The unit's agent enters the scope before it runs the {ref}`relation-joined <hook-relation-joined>` hooks, and leaves it when the unit departs (see {ref}`relation-departed <hook-relation-departed>`).

The operation's required access, rules, and errors are:

- *Required access:* The agent of the unit. An agent that has logged in as a unit acts only for that unit. An agent that has logged in as an application acts for any unit of that application. No model access level or block applies.
- *Rules:*
  - A unit enters the scope of a relation when its application is an endpoint of the relation. The relation and the unit must be alive.
  - A unit that is already in scope stays as it is, with its settings. No error is returned, even when the unit or the relation is dying.
  - A unit that enters scope gets the settings `ingress-address`, the first ingress address of its network for the relation, and `egress-subnets`, the comma-separated list of its egress subnets. A setting with no value is left out.
  - A subordinate unit cannot enter a container-scoped relation with a principal application other than the application of its own principal unit. The request is ignored without an error.
  - Entering the scope of a container-scoped relation with a subordinate application creates the subordinate unit (see {ref}`subordinate relations <subordinate-relation>`).
  - Leaving scope copies the unit's settings to the archive of the relation, replacing the archive of that unit, deletes the unit's settings, and writes a record of the departed unit.
- *Errors:* The client receives each in the result for the unit:
  - **`cannot enter scope, unit or relation not alive`** (code `cannot enter scope`): *Trigger:* The unit or the relation is dying. *Remediation:* Add the relation again if it is dying. Deploy a new unit if the unit is dying.
  - **`cannot enter scope, subordinate unit exists but is not alive`** (code `cannot enter scope yet`): *Trigger:* The principal unit has a subordinate unit that is dying. *Remediation:* Wait until the subordinate unit is gone, then retry.
  - **`unit <tag> not found`** and **`relation <tag> not found`** (code `not found`): *Trigger:* The unit or the relation does not exist, when entering scope. *Remediation:* Use an existing unit and relation.
  - **`checking unit valid in relation: unit not in relation`**: *Trigger:* The unit's application is not an endpoint of the relation. *Remediation:* Use a relation of the unit's application.
  - **`relation unit not found`**: *Trigger:* The unit is not in the scope of the relation, when leaving scope. *Remediation:* Skip the call: the unit is already out of scope.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The tag is not that of a unit the caller may act for, or the relation does not exist. *Remediation:* Use the unit that the agent runs.

(relation-reading-data)=
### Reading relation data

```{ggarch}
:file: ../juju.ggarch
:view: Sharing relation settings
:caption: A change of settings reaches another unit in six steps. When the hook of unit A ends successfully, its unit agent commits the relation settings that the hook set. The controller stores them and a hash of them in one transaction in the model database, holding the application's lease when application settings change. A watcher that the controller keeps for the unit agent of unit B reports that the settings of unit A have a new version. The unit agent of unit B asks the controller for the settings of unit A and runs the relation-changed hook. A commit from a unit that is not unit A fails with permission denied. A commit of application settings from a unit that is not the leader fails because the lease is not held. Nothing is stored when the commit fails.
:alt: A sequence diagram with four participants: the unit agent of unit A, the controller, the model database, and the unit agent of unit B. The unit agent of unit A commits its relation settings to the controller when its hook ends. The controller stores the settings and their hash in the model database. The controller tells the unit agent of unit B that the settings of unit A have a new version. The unit agent of unit B asks the controller for the settings of unit A, receives them, and runs the relation-changed hook.
```

Reading relation data returns the settings of a unit or of an application in a relation. The settings are keys and values, all of them strings. The `relation-get` hook tool (see {ref}`relation-get <hook-command-relation-get>`) reads them. It takes an optional key, a unit or application name, a relation ID, and an option to read the application settings.

What a caller reads and writes depends on whether the unit is the leader of its application. The table shows, for a unit of an application in the relation, what it does with each set of settings:

| Caller | Own unit's settings | Another unit's settings | Own application's settings | Other application's settings |
|-|-|-|-|-|
| Unit | read, write | read | read | read |
| Leader unit | read, write | read | read, write | read |

In a peer relation the relation has one endpoint, so the application on the other side is the unit's own application. Every unit reads the settings of the other units and the application settings. Only the leader writes the application settings.

The operation's required access, rules, and errors are:

- *Required access:* The agent of the unit (see {ref}`entering and leaving relation scopes <relation-scope-operation>`).
- *Rules:*
  - A unit reads its own settings with a call that serves only that unit. It reads the settings of another unit, or of an application, with a call that names the unit or application.
  - A unit can read the settings of a unit by name, including a unit that has left. The settings of a unit that left come from the archive and stay until the relation is removed.
  - The leader of an application reads the application's own settings with a call that needs the lease. A unit that is not the leader reads them with the call that names the application.
  - A key that does not exist returns an empty value.
  - The hook tool needs a relation ID, which in a relation hook defaults to the relation of the hook. The default unit is the remote unit of the hook (`JUJU_REMOTE_UNIT`).
- *Errors:* The client receives each in the result for the unit:
  - **`relation unit not found`**: *Trigger:* The named unit is not in the relation and has no archived settings. *Remediation:* Name a unit that is, or was, in the relation.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller is not the unit, the relation does not exist, or the unit is not the leader and asked the call for its own application settings. *Remediation:* Use the unit that the agent runs, and read the application settings with the call that names the application.
  - **`no relation id specified`**, **`no unit or application specified`**, **`expected unit name, got application name "<name>"`**, and **`invalid unit name "<name>"`**: *Trigger:* The hook tool has no relation ID, has no unit or application to read, is given an application name without the option for the application settings, or is given an invalid unit name. These come from the hook tool. *Remediation:* Pass the relation ID with `-r`, and name a unit or, with the option for the application settings, an application.

(relation-writing-data)=
### Writing relation data

Writing relation data sets and removes settings of a unit or of an application in a relation. The `relation-set` hook tool (see {ref}`relation-set <hook-command-relation-set>`) writes them. It takes `key=value` pairs, a relation ID, a file of settings, and an option to write the application settings.

The table shows what a unit of an application in the relation writes:

| Caller | Own unit's settings | Another unit's settings | Own application's settings | Other application's settings |
|-|-|-|-|-|
| Unit | write | no | no | no |
| Leader unit | write | no | write | no |

The operation's required access, rules, and errors are:

- *Required access:* The agent of the unit (see {ref}`entering and leaving relation scopes <relation-scope-operation>`). Writing the application settings needs the unit to be the leader of its application.
- *Rules:*
  - A unit writes only its own settings. The leader also writes the settings of its own application. No call writes the settings of another unit.
  - A key with an empty value is removed. A value is stored as a string.
  - A settings file is a YAML map of keys and values. A `key=value` pair overrides the same key from the file.
  - The keys `ingress-address` and `egress-subnets` of a unit are written only by Juju. Juju drops them from the unit's changes and refreshes them when the unit's network changes.
  - The unit must be in the scope of the relation.
  - The keys and values of one set of settings add up to 16 MiB at most.
- *Errors:* The client receives each in the result for the unit:
  - **`cannot write relation settings`**: *Trigger:* The tool was asked for the application settings by a unit that is not the leader. This comes from the hook tool. *Remediation:* Run the tool on the leader unit.
  - **`lease not held`**: *Trigger:* The unit is not the leader of its application, or lost the leadership while the changes were committed. The message can have a prefix. *Remediation:* Retry the hook.
  - **`update relation settings: setting relation settings for relation "<UUID>": relation unit/application not found for unit "<UUID>" and relation "<UUID>"`** (code `not found`): *Trigger:* The unit is not in the scope of the relation. *Remediation:* Write only while the unit is in the relation.
  - **`update relation settings: setting relation settings for relation "<UUID>": checking relation unit settings size: max allowed total size (16777216): quota limit exceeded`** (code `quota limit exceeded`): *Trigger:* The unit settings are larger than the limit. For the application settings, the text after the relation is `setting relation application settings: checking relation application settings size: max allowed total size (16777216): quota limit exceeded`. *Remediation:* Remove keys or store the data elsewhere.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The settings in the request belong to a unit other than the caller's, or the relation key is not valid. *Remediation:* Use the unit that the agent runs.

(relation-setting-status)=
### Setting relation status

Setting relation status changes the status of a relation on behalf of an application. The request gives the unit, the relation ID, the new status, and a message. The controller records the time of the change.

The operation's required access, rules, and errors are:

- *Required access:* The agent of the unit (see {ref}`entering and leaving relation scopes <relation-scope-operation>`). The unit must be the leader of its application.
- *Rules:*
  - The unit named in the request must be the caller.
  - The application of the unit must be an endpoint of the relation.
  - The change must be one that the {ref}`transition table <relation-status>` allows. The status `error` needs a message.
- *Errors:* The client receives each in the result for the entry. The statuses in the messages are shown by their IDs: 0 for `joining`, 1 for `joined`, 2 for `broken`, 3 for `suspending`, 4 for `suspended`, and 5 for `error`:
  - **`updating relation status for "<UUID>": cannot set status <new> when relation has status <current>: relation status transition not valid`**: *Trigger:* The table does not allow the change. *Remediation:* Choose a status that the table allows.
  - **`updating relation status for "<UUID>": cannot set status 5 without message`**: *Trigger:* The new status is `error` and the message is empty. *Remediation:* Give a message.
  - **`encoding relation status: unknown relation status "<status>"`**: *Trigger:* The status is not one of the six. *Remediation:* Use one of the six statuses.
  - **`lease not held`**: *Trigger:* The unit is not the leader of its application. The message can have a prefix. *Remediation:* Run the call on the leader unit.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The unit is not the caller, the relation does not exist, or the application of the unit is not an endpoint of the relation. *Remediation:* Use the unit that the agent runs. Use a relation of its application.

(relation-status)=
#### Status of a relation

```{ggarch}
:file: ../juju.ggarch
:view: Relation status
:caption: A leader unit moves a relation between the statuses joining, joined, suspending, and suspended only along the arrows. A relation can also change to broken from every status. A relation can change to error, with a message, from every status. Broken accepts no change except to error. Error accepts a change to every other status. Setting the status that a relation already has is always accepted.
:alt: A state diagram of four statuses. Joining leads to joined, suspending, and suspended. Joined leads to suspending and suspended. Suspending leads to joining, joined, and suspended. Suspended leads to joining and joined.
```

The status of a relation shows how the relation stands with the units of its applications. A new relation starts as `joining`, whether a client or Juju creates it. The next statuses come from two places:

- **A leader unit:** After a unit of an application enters the relation, the {ref}`leader unit <leader-unit>` of the application sets `joined`. After the `relation-broken` hook of a suspended relation, the leader sets `suspended`.
- **A client:** Suspending a cross-model relation sets `suspending`, and resuming it sets `joining` (see {ref}`suspending relations <relation-suspending>`).

The changes that a leader unit makes are checked against these rules. The first column is the current status. The second column lists the statuses that can follow:

| Current status | Allowed next statuses |
|-|-|
| `joining` | `joined`, `suspending`, `suspended`, `broken`, `error` |
| `joined` | `suspending`, `suspended`, `broken`, `error` |
| `suspending` | `joining`, `joined`, `suspended`, `broken`, `error` |
| `suspended` | `joining`, `joined`, `broken`, `error` |
| `broken` | `error` |
| `error` | `joining`, `joined`, `suspending`, `suspended`, `broken` |

The status `error` needs a message. A change from `suspending` to `suspended` keeps the earlier message when the new message is empty. The status that a client sets on a cross-model relation is not checked against the table.

(relation-lookup)=
### Finding relations in hooks

Finding relations returns the relations a unit takes part in. The `relation-ids` tool (see {ref}`relation-ids <hook-command-relation-ids>`) lists the relation IDs of an endpoint name. The `relation-list` tool (see {ref}`relation-list <hook-command-relation-list>`) lists the units of a relation or, with an option, its remote application. The `relation-model-get` tool (see {ref}`relation-model-get <hook-command-relation-model-get>`) returns the UUID of the model of the application on the other side. The agent also asks for a relation by its key or ID, and for the scope state and suspended state of each relation of a unit.

The operation's required access and rules are:

- *Required access:* The agent of the unit (see {ref}`entering and leaving relation scopes <relation-scope-operation>`).
- *Rules:*
  - A relation is returned only to an agent whose application is an endpoint of the relation. The answer has the relation's ID, key, life, whether it is suspended, the unit's endpoint, and the application on the other side with its model UUID.
  - The model UUID of the other side is the UUID of the unit's own model unless the relation is cross-model.
  - `relation-ids` leaves out the relations whose life is dead and lists the IDs sorted as `<endpoint>:<ID>`.
  - `relation-list` lists the units that are in scope on the other side.
  - The tools that read a relation need a relation ID, which in a relation hook defaults to the relation of the hook.

## Relations and model migration

A {ref}`migration <model-migrating>` moves the relations of a model with their settings and the units in their scope. A relation that has a remote application keeps the UUID that both models agreed on for it. The migration renames its endpoints and its key with the names of the remote applications, so that each key stays unique in the model.
