---
myst:
  html_meta:
    description: "Juju controller upgrade reference: the record the controller keeps of an upgrade, its states and per-node progress, the change streams, and the operations that start an upgrade and carry it out."
---

(controller-upgrade)=
# Controller upgrade

```{ibnote}
See also: {ref}`Juju | Upgrade a controller <upgrade-a-controller>`
```

A **controller upgrade** is the move of all the nodes of a {ref}`controller <controller>` to one newer patch version of Juju: each node swaps the binary of its agent, the controller applies the schema patches to its databases, and every node runs the code-level upgrade steps. The {ref}`controller database <database>` keeps one record per upgrade, with the two versions, the state, and one row per node for its progress. A client starts an upgrade; the agents of the controller nodes carry it out.

(controller-upgrade-data-model)=
## Controller upgrade data model

```{ggarch}
:file: ../juju.ggarch
:view: Controller upgrade data model
:caption: The controller database keeps one record for each upgrade, with the state of the move, and one row for each controller node that takes part. An upgrade has one state (a state is held by many upgrades), and many node rows; a node row belongs to one upgrade and to one controller node, and a controller node has one row per upgrade. The controller node is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Four boxes of the same size in a row inside a group named Controller database, each with a filled table mark in the corner. Upgrade state is on the left, Upgrade is next to it (m to 1), then Controller node's upgrade (1 to m), and Controller node is on the right (m to 1). Controller node has an outlined arrow mark for a record that another page describes.
```

The controller upgrade data model is the set of records that the controller database keeps about upgrades. The records, with the rules that apply to each, are:

- **Upgrade:** The version the nodes ran before, the target version, and the state of the move.
  - *Rules:*
    - The state is one of `created`, `started`, `db-completed`, `steps-completed`, and `error`.
    - The controller keeps at most one upgrade that has not finished its steps and has not failed.
- **Upgrade state:** The state of the move, one of the five values above.
  - *Rules:*
    - A state value appears once.
- **Controller node's upgrade:** One node's part in the move: when the node finished its upgrade steps.
  - *Rules:*
    - A node has one row per upgrade.
    - Every node of the controller has its row from the start of the upgrade.
- **Controller node:** The node taking part. Another page owns it: see {ref}`controller <controller>`.

An upgrade is a row in the table `upgrade_info`. Its state is a row in the lookup table `upgrade_state_type`. A node's part is a row in `upgrade_info_controller_node`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(controller-upgrade-change-streams)=
## Controller upgrade change streams

Controller upgrade change streams are the notifications that tell the agents of the controller nodes that the upgrade records changed. A node's agent calls `WatchForUpgradeState` to hear that the upgrade reached a state, and `WatchForUpgradeReady` to hear that every node of the controller has registered for the upgrade, which is the moment the database part can start.

(controller-upgrade-operations)=
## Controller upgrade operations

Controller upgrade operations are the actions that start an upgrade and carry it out. A client with {ref}`superuser access <user-access-controller-superuser>` starts it; the agents of the controller nodes create the record and run the move. Setting the target {ref}`agent <agent>` version of a model, which upgrades the agents of that model, is described on the {ref}`agent version <agent-version>` page.

(controller-upgrade-starting)=
### Upgrading the controller

```{ibnote}
See also: {ref}`Juju | Upgrade a controller <upgrade-a-controller>`
```

Upgrading the controller starts the move of a controller's nodes to a newer patch version of Juju. The call names the target version or asks for the newest one available, checks that the controller can reach it, and sets the target versions.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller, or {ref}`write access <user-access-model-write>` to the controller model. A change block on the controller model refuses the call.
- *Rules:*
  - The target version must be a patch version of the current one: a higher major or minor version is refused.
  - The target version must not be below the version the controller runs.
  - Every node of the controller must run the current version.
  - Agent binaries for the target version must exist on the controller, for the architecture of every node, in the {ref}`agent stream <agent-version>` that the request names when it names one.
  - Without a target version, the call takes the highest patch version that has agent binaries.
  - The call sets the target agent version of the controller model and the target version of the controller.
  - A dry run runs the same checks and changes nothing.
- *Errors:* The client receives each in the result of the call:
  - **`controller agent binaries are not available for version "<version>"`**: *Trigger:* No agent binaries for the target version exist on the controller. The client receives it with the code `not found`. *Remediation:* Put the binaries of that version on the controller first.
  - **`controller upgrading is blocked for reason: controller nodes [<nodes>] are not running controller version "<version>"`**: *Trigger:* A node of the controller runs an older version than the others. The client receives it with the code `not supported`. *Remediation:* Bring every node to the current version.
  - **`cannot upgrade the controller to version "<version>" because it is lower than the current running version`**: *Trigger:* The target version is below the running version. The client receives it with the code `not supported`. *Remediation:* Name the running version or a higher patch version.
  - **`cannot upgrade the controller to a version that is more than a patch version increase`**: *Trigger:* The target version raises the major or minor version. The client receives it with no error code. *Remediation:* Upgrade one patch version at a time.
  - **`agent stream "<stream>" is not a recognised valid value`**: *Trigger:* The named agent stream does not exist. The client receives it with no error code. *Remediation:* Use a valid agent stream.
  - **`unauthorized to upgrade model`**: *Trigger:* The caller has neither superuser access to the controller nor write access to the controller model. The client receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.

(controller-upgrade-running)=
### Running the upgrade

```{ggarch}
:file: ../juju.ggarch
:view: Upgrade state
:caption: An upgrade is created in the state `created`, and moves to `started` when every node of the controller has registered for it. A started upgrade becomes `db-completed` when the schema patches are applied, or `error` when the database part fails. A `db-completed` upgrade becomes `steps-completed` when the last node finishes its upgrade steps.
:alt: A state diagram with five states. Created goes to started. Started goes to db-completed and to error. Db-completed goes to steps-completed.
```

Running the upgrade carries the move out, once every node has downloaded the new binary and restarted into it. The agents of the controller nodes do it: one node leads the database part; every node runs its own steps.

The operation's required access and rules are:

- *Required access:* The {ref}`controller agent <controller-agent>` of each node of the controller. No client calls it.
- *Rules:*
  - The first node to run creates the upgrade record, with a row for every node of the controller. A node that finds a record already there joins the upgrade instead of creating one.
  - Each node marks itself ready. When every node is ready, the leading node starts the upgrade and applies the schema patches, to the controller database first, then to the model databases.
  - The database part must finish within twenty minutes of the record's creation, or the upgrade fails.
  - After the databases, each node runs the code-level upgrade steps for the controller, and marks itself done when they succeed. A node waits for the others for at most ten minutes.
  - When every node is done, the upgrade is complete.
  - A failure in the database part or in the steps of a node puts the upgrade in the state `error`. A failed upgrade does not block a new one.
