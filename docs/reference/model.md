---
myst:
  html_meta:
    description: "Juju model reference: what a model is, the data the controller keeps about it, the change streams that agents and workers follow, and the operations that add, list, show, configure, upgrade, migrate, and destroy models."
---

(model)=
# Model

```{ibnote}
See also: {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

A **model** is the workspace on a {ref}`controller <controller>` that holds a deployment: its {ref}`applications <application>` and what they run on and use, such as {ref}`machines <machine>`, {ref}`storage <storage>`, {ref}`network spaces <space>`, {ref}`relations <relation>`, and {ref}`secrets <secret>`. A model is deployed into one {ref}`cloud <cloud>` with one {ref}`credential <credential>`, has its own {ref}`configuration <model-configuration>`, and is shared with other {ref}`users <user>` at an access level of read, write, or admin. Every controller has one **controller model**, named `controller`, which Juju creates at {ref}`bootstrap <bootstrap-a-controller>` to hold the controller's own machines; every other model is a **workload model**.

(model-data-model)=
## Model data model

```{ggarch}
:file: ../juju.ggarch
:view: Model data model
:caption: The controller database keeps one record for each model. The record holds the model's name and qualifier, which are unique together, its life, and whether it is active, and it names one model type (a type has many models), one cloud, optionally one region, and one credential, and one namespace, the name of the model's own database. A model has many migrations, each in one migration phase. The model database keeps its own records of the model: a read-only copy of its identity, its constraints, a copy of its life, and a marker that exists while the model migrates. The cloud, region, and credential are described on their own pages, and the figure shows them only for context: all boxes are records, and the three with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Two groups of boxes of the same size, each with a filled table mark in the corner. The Controller database group has Model in the middle. Above it is Model type (1 to m), to its left is Cloud (1 to m), with Region above and Credential below it, each with an outlined arrow mark, and the Model also points at Region and Credential through dashed lines because both are optional. To its right is Namespace (1 to 1). Below it is Migration (1 to m), and to the right of Migration is Migration phase (m to 1). The Model database group has Model identity copy in the middle, with Model constraints above it, Model life copy below it, and Migrating marker to its right, each 1 to 1.
```

The model data model is the set of records that describe a model in the {ref}`controller database <database>` and in the model's own {ref}`model database <database>`. The records, with the rules that apply to each, are:

- **Model:** The model's name, its qualifier, its type, its {ref}`cloud <cloud>`, {ref}`region <cloud-data-model>`, and {ref}`credential <credential>`, its life, and whether it is active. The qualifier is a {ref}`user <user>` name, such as `admin` or `alice@external`, that distinguishes models with the same name; the user who adds a model is normally its qualifier.
  - *Rules:*
    - A model's name and qualifier together are unique, so two users can each have a model with the same name.
    - A model has a life: alive, dying, or dead. See {ref}`Destroying models <model-destroying>`.
    - A model is inactive until its creation has finished. The controller starts workers (the processes that do the controller's work for a model) only for active models, and it cannot report the life of an inactive model.
- **Model type:** One of `iaas` or `caas`. The type follows the model's cloud: a model on a {ref}`Kubernetes cloud <kubernetes-cloud>` is `caas`, and a model on any other cloud is `iaas`. The type decides which workers run for the model and whether its {ref}`applications <application>` scale in pods or on {ref}`machines <machine>`.
- **Namespace:** The name of the model database, which holds the model's contents. A model has one namespace.
- **Model database:** The model database holds the model's {ref}`machines <machine>`, {ref}`applications <application>`, {ref}`units <unit>`, {ref}`relations <relation>`, and the rest of its contents. It also holds the model's configuration (a set of key and value pairs), its default {ref}`storage <storage>` pools, its target agent version (the version its {ref}`agents <agent>` should run, with the latest version known and the agent stream), and these records about the model itself:
  - **Model identity copy:** A read-only copy of the model's identity (its UUID, controller, name, qualifier, type, cloud, region, credential, and whether it is the controller model), so that processes of the model need not reach the controller database.
  - **Model constraints:** The model's {ref}`constraints <constraint>`: one record, or none when no constraints are set.
  - **Model life copy:** A copy of the model's life. The controller database record is the authority, and the copy can lag it.
  - **Migrating marker:** A record that exists while the model is migrating.
- **Cloud, region, and credential:** The records a model points at. Other pages own them: see {ref}`cloud <cloud>`, which also describes regions, and {ref}`credential <credential>`. The region and the credential are optional.
- **Secret backend:** The backend that stores the model's {ref}`secrets <secret>`, chosen from the controller's secret backends. See {ref}`secret backend <secret-backend>`.
- **Last login:** The last time each user connected to the model, which listings show as the user's last connection.
- **Migration and migration phase:** A record of each attempt to migrate the model to another controller, with the migration phase the attempt is in (one of a fixed list, see {ref}`Migrating models <model-migrating>`), the history of its phases, its latest status message, the reports of the model's {ref}`agents <agent>`, the credentials for the target controller, and, once the model has left the source, a redirect record that keeps the target's addresses and the model's users.
  - *Rules:*
    - A model has at most one active migration. A migration is active until it reaches `done`, `reap-failed`, or `abort-done`.
    - The target controller keeps an import record while it receives a model, and the record is in one of three phases: `importing`, `activating`, or `aborting`.

A model is a row in the table `model`, and its namespace, secret backend, and last logins are rows in `model_namespace`, `model_secret_backend`, and `model_last_login`. The model type comes from the lookup table `model_type`, and the migration records are rows in the tables whose names start with `model_migration`. The model database has its own tables `model`, `model_config`, `model_constraint`, `model_life`, `model_migrating`, `model_storage_pool`, and `agent_version`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(model-change-streams)=
## Model change streams

Model change streams are the notifications that tell a worker (a process of the {ref}`controller <controller>` that does its work for a model) or an {ref}`agent <agent>` that something about a model has changed, delivered through a watcher (an API object that tells its holder when something it follows has changed, so that nothing has to poll). Clients read model state through the controller API (the set of calls that clients and agents make to the controller); the streams below reach the controller's workers and the model's agents.

- **Active models:** The controller's model worker manager (the worker that starts the workers of each model) watches the set of active models and starts the workers of each model that joins it.
- **Migration phase:** Every agent of a model (machine and unit agents) follows the phase of the model's migration. The agents act on each phase and report success or failure back, as {ref}`Migrating models <model-migrating>` describes.
- **Credential:** An agent can follow the {ref}`credential <credential>` of the model's {ref}`cloud <cloud>` and learns when it changes or becomes invalid.
- **Target agent version:** Every machine and unit agent follows the model's target agent version and upgrades itself to it when it changes. See {ref}`Upgrading models <model-upgrading>`.
- **Logging configuration:** Agents follow the `logging-config` key of the model's configuration.

(model-operations)=
## Model operations

Model operations are the actions a {ref}`client <client>` can take on models through the {ref}`controller <controller>` API (the set of calls that clients and {ref}`agents <agent>` make to the controller): add, list, show, configure, change the {ref}`credential <credential>`, upgrade, migrate, and destroy. Granting and revoking a user's access to a model is described on the {ref}`user <user>` page.

(model-adding)=
### Adding models

```{ibnote}
See also: {ref}`Juju | Add a model <add-a-model>`
```

Adding models creates a model on the {ref}`controller <controller>`. When the call returns, the model's records exist and its workers (the processes that do the controller's work for the model) are running.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Add-model access <user-access-cloud-add-model>` to the {ref}`cloud <cloud>`, or {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A request that names no cloud uses the controller's default cloud.
  - A request that names no {ref}`region <cloud-data-model>` uses the default region when the cloud is the default cloud.
  - A request that names no {ref}`credential <credential>` uses the caller's default credential for that cloud. If the caller has none, the model is added without a credential, which only some clouds accept.
  - The name cannot be empty and the qualifier must be a valid user name. A caller who is not a superuser can add models only with their own name as the qualifier.
  - The name and qualifier together must be new.
  - The controller opens a provider for the cloud with the credential before it writes anything, and refuses the request if the provider cannot be opened.
  - The model's {ref}`agent <agent>` version defaults to the controller's version. A requested version cannot be higher than the controller's, and a lower one needs agent binaries for it on the controller. The agent stream defaults to the model default for the cloud or region, and then to `released`.
  - The controller discovers the cloud's {ref}`spaces <space>` for the new model.
  - The creator receives {ref}`admin access <user-access-model-admin>` to the new model.
- *Errors:*
  - **`model <qualifier>/<name> already exists`**: *Trigger:* A model with the same name and qualifier exists. The {ref}`client <client>` receives it with the code for an existing entity. *Remediation:* Use another name or qualifier.
  - **`cloud credential for new model is not valid`**: *Trigger:* The credential is marked invalid. *Remediation:* Update the credential, or name another.
  - **`cannot create model with invalid qualifier`** or **`cannot create model with qualifier "<qualifier>"`**: *Trigger:* The qualifier is not a valid user name, or the caller is not a superuser and named another user. *Remediation:* Use your own user name as the qualifier.
  - **`supplied agent version for new model "<name>" is not supported: <reason>`** and **`supplied agent stream for new model "<name>" is not supported: <reason>`**: *Trigger:* The requested agent version is higher than the controller's or has no binaries on the controller, or the stream is not a valid stream. The client receives them with the code for an invalid argument. *Remediation:* Request a supported version or stream.
  - **`cannot find default model cloud and credential`**: *Trigger:* The request names no cloud and the controller has no default. *Remediation:* Name the cloud.

(model-listing)=
### Listing models

```{ibnote}
See also: {ref}`Juju | View all the models available on a controller <view-all-the-models-available-on-a-controller>`
```

Listing models returns the models a {ref}`user <user>` can see, either as a plain list or as summaries with a model's status, counts of {ref}`machines <machine>`, cores, and {ref}`units <unit>`, and the user's access and last connection.

The operation's required access, rules, and errors are:

- *Required access:* None beyond being logged in, when the caller asks about themselves. A superuser can ask about any user.
- *Rules:*
  - A superuser's plain list contains every model on the {ref}`controller <controller>`. For any other caller, a list holds the models they have access to.
  - Summaries cover the models of the named user. Only a superuser can ask for the summaries of every model on the controller.
  - A model that is removed while a list is compiled is left out of it.
- *Errors:*
  - **`permission denied`**: *Trigger:* Asking for another user's models without superuser access, or for every model's summary as a non-superuser. *Remediation:* Ask a superuser, or list your own models.
  - **`user "<name>" does not exist`**: *Trigger:* Asking for summaries of a user that the controller does not know. *Remediation:* Check the name.

(model-showing)=
### Showing a model

```{ibnote}
See also: {ref}`Juju | View details about a model <view-details-about-a-model>`
```

Showing a model returns a model's details: its name, UUID, type, qualifier, life, {ref}`cloud <cloud>`, {ref}`region <cloud-data-model>`, and {ref}`credential <credential>` (with whether the credential is valid), its {ref}`agent <agent>` version, its status, its {ref}`users <user>`, and its supported features.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. A caller with {ref}`write <user-access-model-write>` or {ref}`admin <user-access-model-admin>` access, or a superuser, also receives the model's {ref}`machines <machine>` and its {ref}`secret <secret>` backends.
- *Rules:*
  - A superuser sees every user with access to the model, and any other caller sees only themselves.
  - While a migration is running, the details include the migration's latest status message and its start time.
- *Errors:*
  - **`permission denied`**: *Trigger:* The caller has no access to the model. *Remediation:* Ask a model administrator for access.
  - **`model not found`**: *Trigger:* The model does not exist or was removed while the call ran. *Remediation:* Check the model.

(model-configuring)=
### Configuring models

```{ibnote}
See also: {ref}`Juju | Configure a model <configure-a-model>`
```

Configuring models changes the settings of a model: its configuration keys, its {ref}`constraints <constraint>`, the defaults that new models start with, its {ref}`secret <secret>` backend, and its {ref}`credential <credential>`. The keys a model accepts are listed in the {ref}`list of model configuration keys <list-of-model-configuration-keys>`.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model for configuration keys, constraints, and the secret backend; {ref}`read access <user-access-model-read>` to read them. Setting or unsetting a key that the schema marks as secret needs {ref}`admin access <user-access-model-admin>`. Setting model defaults, which apply to new models of a {ref}`cloud <cloud>` or {ref}`region <cloud-data-model>`, needs {ref}`superuser access <user-access-controller-superuser>`. Changing the credential needs admin access to the model or superuser access.
- *Rules:*
  - A change block on the model (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) refuses changes to configuration keys, constraints, and the credential. A change block on the {ref}`controller <controller>` model refuses changes to the defaults.
  - Only a superuser can set a logging configuration that includes the `TRACE` level.
  - The {ref}`agent <agent>` stream is validated on its own and stored with the model's agent version information.
  - A model's constraints can name only spaces that exist in the model and a valid container type.
  - A model's secret backend must exist and must be valid.
- *Errors:*
  - **`config key ["<key>"] not valid: <reason>`**: *Trigger:* A key is unknown or its value fails validation. *Remediation:* Correct the key or value.
  - **`space not found for model constraint "<model>": <reason>`**: *Trigger:* A constraint names a space that does not exist. *Remediation:* Name an existing space.
  - **`invalid container type for model constraint "<model>"`**: *Trigger:* A constraint names an unknown container type. *Remediation:* Use a supported container type.
  - **`secret backend "<name>" not found`** and **`secret backend "<name>" not valid`**: *Trigger:* The named backend does not exist, or cannot back this model. *Remediation:* Name an existing backend that suits the model.
  - **`permission denied`**: *Trigger:* The caller lacks the access above. *Remediation:* Ask a model administrator or a superuser.

(model-upgrading)=
### Upgrading models

```{ibnote}
See also: {ref}`Juju | Upgrade a model <upgrade-a-model>`
```

Upgrading models raises the target agent version of a model, which makes the model's machine and unit {ref}`agents <agent>` upgrade themselves to it. A request for the controller model upgrades the controller together with its model.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model, or {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A change block on the model (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) refuses the request.
  - A request that names no version upgrades to the highest version that runs on the controller's nodes. A request can also change the agent stream, and it can ask for a dry run, which runs the checks, reports the version it would choose, and changes nothing.
  - The version cannot be lower than the model's current target version and cannot be higher than the controller's version, and the controller must hold agent binaries for it.
  - A workload model with machines on a base that is not a supported workload base cannot be upgraded.
  - When the call returns, the model's target agent version is set, and its agents upgrade afterwards.
  - A model upgrade cannot be aborted or reversed.
  - The controller model upgrades to a patch version only. Every controller node must already run the controller's current version, and the controller must hold binaries for the architecture of each node.
- *Errors:*
  - **`unauthorized to upgrade model`**: *Trigger:* The caller lacks the access above, or named another model. *Remediation:* Ask a model administrator or a superuser.
  - **`model agent binaries are not available for version "<version>"`** and **`controller agent binaries are not available for version "<version>"`**: *Trigger:* The controller holds no binaries for the version, or for a node's architecture. *Remediation:* Choose a version whose binaries are available.
  - **`model upgrading is blocked for reason: <reason>`**: *Trigger:* A machine uses an unsupported base; the reason reads `model has <n> machines using unsupported bases, the supported bases are: [<bases>]`. *Remediation:* Replace the machines with ones on a supported base.
  - **`controller upgrading is blocked for reason: controller nodes [<ids>] are not running controller version "<version>"`**: *Trigger:* A controller node has not yet reached the controller's current version. *Remediation:* Wait for the nodes to finish upgrading.
  - **`cannot upgrade the model agent to version "<version>" because it is lower than the current running version`** and **`cannot upgrade the controller to version "<version>" because it is lower than the current running version`**: *Trigger:* The version is lower than the current one. *Remediation:* Choose a higher version.
  - **`cannot upgrade the model to a version that is more than a patch version increase`**: *Trigger:* The version is higher than the controller's, or is not a valid version. The message names a patch increase, but the check compares the version with the controller's. *Remediation:* Upgrade the controller first, or choose a lower version.
  - **`cannot upgrade the controller to a version that is more than a patch version increase`**: *Trigger:* The version differs from the controller's in the major or minor number. *Remediation:* Choose a patch version, or {ref}`migrate <model-migrating>` the model to a controller of the newer version.
  - **`agent stream "<stream>" is not a recognised valid value`**: *Trigger:* The stream is not a valid agent stream. *Remediation:* Use a valid stream.
  - **`aborting model upgrades is not supported`**: *Trigger:* A request to abort an upgrade. *Remediation:* None; an upgrade cannot be aborted.

(model-migrating)=
### Migrating models

```{ibnote}
See also: {ref}`Juju | Migrate a model <migrate-a-model>`
```

```{ggarch}
:file: ../juju.ggarch
:slides: Model migration phase | Migration request | Migration quiesce and import | Migration validation | Migration success and reap
:caption: A migration runs the same phases in the same order every time: the source and the target check the request, the agents quiesce, the target imports the model, the agents validate against the target, and the agents switch to the target before the source removes the model.
:slide-captions: A migration starts in quiesce and moves through import, validation, success, log-transfer, and reap to done. A failure in quiesce, import, or validation moves it to abort and then to abort-done, with the model still on the source controller. A failure when reap removes the model from the source ends in reap-failed, and the model is already live on the target. No other change of phase is allowed. | The client asks the source controller for a migration. The source checks the caller's access and runs its prechecks, the target runs its prechecks, and the source answers with a migration ID. The rest runs without the client. | In `quiesce`, the model's agents stop their work and report that they are ready, and the prechecks run again. In `import`, the source sends the model, its charms, agent binaries, and resources to the target, which holds the model inactive. | In `validation`, the agents log in to the target and report. The target checks the model's machines against the cloud and prepares to activate the model. A failure up to here aborts the migration. | In `success`, the agents report and switch to the target, and the target takes over the cloud resources. The source then copies the logs and removes the model, keeping a redirect record.
:alt: Five slides. The first is a state diagram of ten phases. A row of quiesce, import, validation, success, log-transfer, and reap, each leading to the next. Reap leads to done and to reap-failed. Quiesce, import, and validation each also lead to abort, which leads to abort-done. The other four are sequence diagrams. The first has the client, the source controller, and the target controller: the client requests the migration, the source runs its prechecks, the target runs its prechecks, and the source returns a migration ID. The second has the source, the model agents, and the target: the agents quiesce and report ready, the prechecks run again, and the source sends the model to the target. The third has the source, the agents, the target, and the cloud: the agents log in to the target and report, and the target checks the machines against the cloud and is prepared to activate the model. The fourth has the source, the agents, and the target: the agents report and switch to the target, the target takes over the cloud resources, the source copies the logs, and removes the model.
```

Migrating models moves a model from one {ref}`controller <controller>`, the source, to another, the target. The source controller drives the migration through a sequence of phases, and the model's {ref}`agents <agent>` take part in several of them.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the source controller, and the address, the certificate, and a {ref}`user's <user>` credentials for the target controller.
- *Rules:*
  - The source and the target both run prechecks before the model is quiesced and again when the model is stable. A request can ask for the prechecks only.
  - Every user with access to the model must exist on the target, unless the request skips the user checks. When external users (those with an `@` in their name) have access, the two controllers must also use the same identity provider URL (the address of the service that authenticates external users).
  - The target must run Juju 4.1 or later, because an older controller cannot import a model in the format that the source sends.
  - Each phase writes a status message, which the model's details report as the migration status.
  - In `quiesce` and `validation`, an agent that reports failure, or that does not report within the `migration-agent-wait-time` controller configuration, aborts the migration.
  - A migration cannot be aborted after `validation`. A failure in `quiesce`, `import`, or `validation` aborts it: the source removes the imported copy from the target and the migration ends in `abort-done`, with the model still on the source.
  - The target holds an imported model inactive until the migration activates it. When the source removes the model, it keeps a redirect record with the target's addresses and the model's users, so that agents that were offline learn where the model went.
  - The source copies the model's logs to the target in `log-transfer`, unless the controller forwards logs to [Loki](https://grafana.com/oss/loki/).
  - A migration that fails to remove the model from the source ends in `reap-failed`, and the model is already live on the target.
- *Errors:* A precheck that fails when the migration is requested is returned as the call's error, with a prefix such as `source prechecks failed:`. A failure after that becomes the migration's status, `aborted, removing model from target controller: <reason>`, where the reason is one of the messages below. An error raised while the target imports the model has the prefix `model data transfer failed, failed to import model into target controller`, followed by a colon and the message.
  - **`model is <life>`**, **`model is being imported as part of another migration`**, and **`model has revoked credentials`**: *Trigger:* The model is dying or dead, is itself being imported, or has a {ref}`credential <credential>` that was revoked. *Remediation:* Wait for the removal or import to finish, or set a valid credential.
  - **`cannot initiate migration as the users granted access to the model do not exist on the destination controller…`** and **`cannot initiate migration as external users have been granted access to the model and the two controllers have different identity provider configurations…`**: *Trigger:* A user of the model is missing on the target, or external users have access and the identity providers differ. Both messages list the users involved. *Remediation:* Add the users to the target, or remove them from the model.
  - **`machine <id> is <life>`** and **`unit <name> hasn't joined relation "<key>" yet`**: *Trigger:* A machine is not alive, or a {ref}`unit <unit>` has not joined a {ref}`relation <relation>` of the model. *Remediation:* Wait, then retry.
  - **`fan networking not supported, remove fan-config "<value>" from migrating model config`**: *Trigger:* The model sets `fan-config` or `container-networking-method`. *Remediation:* Remove the key.
  - **`all charms now require a manifest.yaml file, this model hosts charm(s) with no manifest.yaml file: <list>`**: *Trigger:* The model runs a charm revision without a manifest. *Remediation:* Refresh the listed {ref}`applications <application>`.
  - **`controller: upgrade in progress`**: *Trigger:* The source controller is upgrading. *Remediation:* Wait for the upgrade.
  - **`model has higher version than target controller (<model> > <target>)`** and **`source controller has higher version than target controller (<source> > <target>)`**: *Trigger:* The target runs an older version than the model or the source. *Remediation:* Upgrade the target first.
  - **`model's cloud "<cloud>" not found on target controller`**: *Trigger:* The target does not know the model's cloud. *Remediation:* Add the cloud to the target.
  - **`model named "<name>" already exists`**, **`model with same UUID already exists (<uuid>)`**, and **`model is being migrated out of target controller`**: *Trigger:* The target holds a clashing model. *Remediation:* Rename or remove it, or wait.
  - **`target controller does not support the model migration format; upgrade the target controller to Juju 4.1 or later`**: *Trigger:* The target is older than 4.1. *Remediation:* Upgrade the target.
  - **`importing model "<name>" with uuid "<uuid>", user not found for name "<qualifier>"`**: *Trigger:* The user named by the model's qualifier does not exist on the target. *Remediation:* Add the user to the target controller.
  - **`setting cloud credential for model "<uuid>": not found cloud credential "<key>"`**: *Trigger:* The target has no credential with the cloud, owner, and name that the model uses. *Remediation:* Add that credential to the target, or give the model a credential that exists on both controllers.
  - **`machine sanity check failed, <n> error(s) found`**: *Trigger:* After the transfer, the target's machines differ from what the cloud reports. *Remediation:* Reconcile the cloud and Juju, then retry.
  - **`<step>, timed out waiting for agents to report`** and **`<step>, some agents reported failure`**: *Trigger:* Agents did not report in time or reported an error. *Remediation:* Raise `migration-agent-wait-time`, or read the agents' logs.
  - **`removing exported model failed: <reason>`**: *Trigger:* The source copy could not be removed after the model went live on the target. *Remediation:* None for the model, which is already live on the target; only the source's copy remains.

(model-destroying)=
### Destroying models

```{ibnote}
See also: {ref}`Juju | Destroy a model <destroy-a-model>`
```

Destroying models removes a model and everything in it, including the {ref}`cloud <cloud>` resources behind its machines. The {ref}`controller <controller>` marks the model dying and schedules the removal of its {ref}`relations <relation>`, {ref}`units <unit>`, {ref}`machines <machine>`, {ref}`applications <application>`, and {ref}`storage <storage>`; when all of it is gone, the model is dead and the controller deletes its database.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model, or {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A destroy block, a remove block, or a change block on the model (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) refuses the request.
  - The controller model is destroyed only with the force option.
  - A model that holds persistent storage is destroyed only when the request says whether to destroy or release the storage.
  - A request with the force option removes what the normal removal cannot, after waiting for the maximum wait time the request names.
  - A model that is being imported by a {ref}`migration <model-migrating>` is removed through the migration's abort, not through this operation.
  - A model stays dying until every one of its machines is gone, and a machine that hosts containers is removed after its containers.
  - The model's database is deleted last, after the rest of the model is removed.
- *Errors:*
  - **`model "<uuid>" has persistent storage`**: *Trigger:* The model holds persistent storage and the request does not say what to do with it. The {ref}`client <client>` receives it with the code for persistent storage. *Remediation:* Destroy or release the storage in the request.
  - **`force required for removal job`**: *Trigger:* Destroying the controller model without the force option. *Remediation:* Force the removal.
  - **`model has an active migration import claim`**: *Trigger:* A normal removal targets a model that is being imported. *Remediation:* Abort the import.
  - **`permission denied`**: *Trigger:* The caller is not an administrator of the model. *Remediation:* Ask a model administrator or a superuser.
