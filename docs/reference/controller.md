---
myst:
  html_meta:
    description: "Juju controller reference: what a controller is, the data it keeps about itself and its nodes, the workers it runs, the changes its processes follow, and the operations that bootstrap, configure, make highly available, and destroy it."
---

(controller)=
# Controller

```{ibnote}
See also: {ref}`Juju | Manage controllers <manage-controllers>`
```

A **controller** is the part of Juju that keeps the declared state of every {ref}`model <model>` and does the work that makes the {ref}`cloud <cloud>` match it: it runs the API server that {ref}`clients <client>` and {ref}`agents <agent>` connect to, keeps the database of everything Juju knows, and runs the workers that act on each model. A controller comes into being at {ref}`bootstrap <bootstrap-a-controller>` and holds one **controller model**, which holds the controller's own machines. On a machine cloud a controller is one or more **controller nodes**, for {ref}`high availability <high-availability>`, and on a Kubernetes cloud a node is a pod.

(controller-data-model)=
## Controller data model

```{ggarch}
:file: ../juju.ggarch
:view: Controller data model
:caption: The controller database keeps one record for each controller node. A node has one agent version record (a node has one, a record belongs to one node), one password record, and many addresses of three kinds: agent addresses, client addresses, and peer addresses. A node has many of each, and an address belongs to at most one node: an agent address or client address that belongs to no node is shared, such as the address of a Kubernetes service in front of the controller pods, and a peer address always belongs to a node. The controller's own identity and its configuration are single records that have no connection to a node, so the figure leaves them out.
:alt: Six boxes of the same size, each with a filled table mark in the corner, inside a group named Controller database. Controller node is in the middle. Node agent version is above it (1 to 1), Node password is below it (1 to 1), Client address is to its left (m to 1), Agent address is above Client address, and both point at Controller node through dashed lines (m to 1) because the node is optional, and Peer address is to its right (1 to m).
```

The controller data model is the set of records that the {ref}`controller database <database>` keeps about the controller itself, its nodes, and the addresses at which the nodes can be reached. The records, with the rules that apply to each, are:

- **Controller:** The controller's UUID, the UUID of its controller model, the agent version it should run, its API port, and its certificates, keys, and system identity.
  - *Rules:*
    - The database keeps at most one.
- **Controller configuration:** The controller's settings, as key and value pairs, such as the {ref}`API port <controller-config-api-port>` and the limits that {ref}`Configuring controllers <controller-configuring>` lists.
  - *Rules:*
    - A key appears once.
- **Controller node:** One controller machine or pod, with its life (alive, dying, or dead), the ID that Dqlite gave the node, and the address Dqlite listens on.
  - *Rules:*
    - Two nodes cannot share a Dqlite node ID, or a Dqlite address.
- **Node agent version:** The agent version and architecture that a node last reported. The architecture is one of `amd64`, `arm64`, `ppc64el`, `s390x`, and `riscv64`.
  - *Rules:*
    - A node has at most one.
- **Node password:** The hash of the password with which the node's agent logs in to the API. The hash algorithm is `sha512`.
  - *Rules:*
    - A node has at most one.
- **Agent address:** An address at which agents reach the API server, with a scope and a priority (lower is preferred). A new agent learns these addresses when it logs in.
  - *Rules:*
    - An address appears once for a node, and once among the shared addresses.
- **Client address:** An address at which users reach the API server, with a scope and a priority. A client receives these addresses when it logs in, and {ref}`Listing controller nodes <controller-listing-nodes>` returns them.
  - *Rules:*
    - An address appears once for a node, and once among the shared addresses.
- **Peer address:** An address at which controller nodes reach one another, with a scope and a priority.
  - *Rules:*
    - An address appears once for a node.

A controller is the one row of the table `controller`, and its settings are rows in `controller_config`. A controller node is a row in `controller_node`, and its records are rows in `controller_node_agent_version`, `controller_node_password`, `controller_agent_address`, `controller_client_address`, and `controller_peer_address`. The tables `controller_node` and `controller_node_agent_version` use the lookup tables `life` and `architecture`, and `controller_node_password` uses `password_hash_algorithm`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(controller-storage)=
## Controller storage

A Juju controller has two basic persistent storage needs: {ref}`database <database>` access and blob storage. By default, Juju will use the filesystem of the controller's supporting infrastructure.

<!-- ADD BACK IN WHEN WE MOVE THOSE VALUES INTO SECRETS.

However, either during bootstrap or later, you can (and, in a production-setting, should!) specify any S3-compatible object store you want (e.g., AWS S3, MicroCeph, MinIO, etc.) using the object-store-related controller configuration keys ({ref}`controller-config-object-store-type`, {ref}`controller-config-object-store-s3-endpoint`, {ref}`controller-config-object-store-s3-static-key`, {ref}`controller-config-object-store-s3-static-secret`, {ref}`controller-config-object-store-s3-static-session`, {ref}`controller-config-object-store-s3-region`).

Also, Juju will apply default S3 policy permissions, but you are free to change them, so long as they satisfy the following as a minimum (at least, during model creation):

```text
{
   "Version" : "2012-10-17",
   "Statement" : [
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:CreateBucket",
            "s3:PutBucketPolicy",
            "s3:PutBucketTagging",
            "s3:PutBucketVersioning",
            "s3:PutBucketObjectLockConfiguration"
         ],
         "Resource" : "arn:aws:s3:::*"
      },
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:ListBucket",
            "s3:ListBucketVersions",
            "s3:ListAllMyBuckets",
            "s3:GetBucketLocation",
            "s3:GetBucketPolicy",
            "s3:GetBucketTagging",
            "s3:GetBucketVersioning",
            "s3:GetBucketObjectLockConfiguration",
            "s3:GetObject",
            "s3:GetObjectLegalHold",
            "s3:GetObjectRetention",
            "s3:PutObject",
            "s3:PutObjectLegalHold",
            "s3:BypassGovernanceRetention",
            "s3:PutObjectRetention",
            "s3:DeleteObject"
         ],
         "Resource" : "arn:aws:s3:::*"
      }
   ]
}
``` -->

(controller-workers)=
## Controller workers

A **worker** is a long-running process inside an agent that does one job, such as serving the API or starting machines. An agent starts its workers with a dependency engine, which starts, stops, and restarts each worker as the workers it needs come and go. The controller's workers are the workers that a controller agent runs, and the engines that the controller agent starts for models and units. Each controller node runs a {ref}`unit agent <unit-agent>` and the code of the [`juju-controller`](https://charmhub.io/juju-controller) charm, and a {ref}`controller agent <controller-agent>` that runs the API server and one node of the embedded [Dqlite](https://canonical.com/dqlite) {ref}`database <database>`.

The interface that shows them is the engine report: on a controller machine the shell function `juju_engine_report` prints the state of every worker of the agent, and the same report is served at the route `/depengine` of the agent's introspection socket, a file in the agent's directory that only the machine's administrator can open. The names below are the names that the report uses, and the list below groups the workers by where they run: in the controller agent's own engine, on one controller only, and in the engines that the controller agent starts for each model and each unit:

- **The controller agent's own engine.** A controller agent is the machine agent of a controller machine (on Kubernetes, of a controller pod). Among others, its engine runs:
  - *Serving the API:* `api-server` and `http-server` serve the API, `api-remote-caller` keeps connections to the other controllers, `ssh-server` and `ssh-tunneler` serve and track {ref}`SSH sessions <juju-ssh>`, and `jwt-parser` parses JSON Web Tokens for the workers that need them.
  - *Keeping the database:* `db-accessor` runs this machine's Dqlite node and joins it to the cluster, and `upgrade-database-runner` applies schema upgrades when the controller upgrades.
  - *Keeping credentials current:* `certificate-watcher` loads the API certificate, and `secret-backend-rotate` rotates the tokens of {ref}`secret backends <secret-backend>`.
  - *Running models:* `model-worker-manager` starts one engine for each active model (see below).
  - *Operating the controller:* `backup-creator` creates backup archives, `controller-log-router` and `controller-log-sink` route and store the controller's logs, `upgrade-controller-steps-runner` runs the controller's upgrade steps, `external-controller-updater` keeps the records of other controllers up to date for cross-model relations, and `deployer` starts the agents of the units that run on the machine, such as the controller charm's unit.
- **One controller at a time.** Some workers run on exactly one controller, the primary. The primary is the controller that holds the lease named `singular-controller` for the controller model; a lease is a claim that lasts a minute and that its holder renews, and when the holder stops, another controller claims it. The workers that run only on the primary are `api-address-setter` (publishes each node's API addresses, filtered by the {ref}`juju-mgmt-space <controller-config-juju-mgmt-space>` setting), `change-stream-pruner` (prunes the controller database's change stream), `external-controller-updater`, `secret-backend-rotate`, and `object-store-drainer`. Every other controller runs `object-store-guard`.
- **One engine for each model.** `model-worker-manager` follows the set of active models, starts an engine for each model, and stops it when the model is dead. An engine runs the workers that act on one model: `provider-tracker` opens the cloud for the model, `removal` removes entities, `migration-master` drives {ref}`migrations <model-migrating>`, `charm-revisioner` checks Charmhub for new charm revisions every 24 hours, `storage-provisioner` provisions storage, `secrets-pruner` prunes obsolete secret revisions, and `operation-pruner` prunes old action results. A machine model's engine also runs `compute-provisioner` (starts and stops machines), `firewaller`, and `instance-poller` (refreshes instance information from the cloud), and a Kubernetes model's engine runs `caas-application-provisioner`, `caas-firewaller`, `caas-model-operator`, and `caas-model-config-manager`. Every controller runs an engine for every active model, and each model has its own lease: the workers `provider-tracker`, `migration-master`, `charm-revisioner`, `operation-pruner`, and `change-stream-pruner`, and on Kubernetes `caas-model-operator` and `caas-model-config-manager`, run only on the controller that holds the model's lease, so the workers of different models may run on different controllers.
- **One engine for each unit.** `deployer` starts an engine for each unit on the machine. The engine runs the unit agent's workers, such as `uniter`, which runs the charm's hooks.

(controller-change-streams)=
## Controller change streams

Controller change streams are the notifications that tell the controller's workers and the agents of every model that something about the controller has changed, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed), and the controller's configuration, its set of nodes, and its API addresses each have one:

- **Controller configuration:** Workers follow the controller configuration and react to the keys that concern them: `api-address-setter` follows `juju-mgmt-space`, and the auditing and agent configuration workers follow their own keys.
- **Controller nodes:** `api-address-setter` follows the set of controller nodes and publishes the API addresses again when a node joins, leaves, or changes address.
- **API addresses:** Every machine and unit agent follows the API addresses and rewrites its own configuration file when they change, so that it can reach a controller after the control plane changes.

(controller-operations)=
## Controller operations

Controller operations are the actions that a {ref}`client <client>` can take on a controller through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): bootstrap, configure, make highly available, list nodes, list and remove blocks, and destroy. Operations on the SSH server are described on the {ref}`SSH server <juju-ssh>` page, the migration of a model on the {ref}`model <model-migrating>` page, and the access that users hold on the controller on the {ref}`user <user>` page.

(controller-bootstrapping)=
### Bootstrapping controllers

```{ibnote}
See also: {ref}`Juju | Bootstrap a controller <bootstrap-a-controller>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Bootstrapping a controller
:caption: Bootstrapping on a machine cloud runs in this order: the client asks the cloud for an instance, waits until it accepts SSH connections, and runs a script on it that downloads the agent and writes its configuration. On the instance, the first database node starts and the controller's records are written: the controller, the admin user, the cloud, the credential, and the controller model. The machine agent starts as a service, and its bootstrap worker seeds users and storage pools, discovers spaces, and deploys the controller charm, fetching it from Charmhub unless the client supplied one. The worker records the controller application and that bootstrap is complete. Meanwhile the client polls the address that the cloud reported, not an address from the controller, until the API answers.
:alt: A sequence diagram with five participants: the client, the cloud, the controller machine, the controller database, and Charmhub. The client asks the cloud for an instance and receives it, then waits for SSH and runs the configure script on the controller machine. The controller machine starts the first database node and writes the controller records to the controller database, starts the machine agent as a service, and runs the bootstrap worker. The machine fetches the controller charm from Charmhub, records the controller application and that bootstrap is complete in the controller database, and the client polls the controller machine's API address.
```

Bootstrapping controllers creates a controller on a cloud: an instance or pod, the controller's database and records, and the application that runs the controller charm. The caller needs no access on a controller, because none exists yet.

The operation's required access and rules are:

- *Required access:* A {ref}`credential <credential>` for the {ref}`cloud <cloud>`.
- *Rules:*
  - The client starts the instance and connects to it over SSH, so the client needs a network path to the instance.
  - The controller machine is machine `0`, and the controller application is named `controller`.
  - The records written before the controller starts are the controller configuration, the controller, the controller's SSH host key and port, the admin user with {ref}`superuser access <user-access-controller-superuser>`, the cloud, the credential, the cloud's defaults, the secret backends, and the controller model with its configuration and constraints.
  - The controller charm is the local file that the client supplied, or the `juju-controller` charm from Charmhub when the client supplied none.
  - On a Kubernetes cloud the client does not start an instance. It creates a StatefulSet (a Kubernetes workload with one pod for each controller node) with one pod, a service in front of it, and the configuration the pod reads. The first pod runs the same bootstrap steps, and its agent is named `controller-0`.

(controller-configuring)=
### Configuring controllers

```{ibnote}
See also: {ref}`Juju | Configure a controller <configure-a-controller>`, {ref}`Juju | List of controller configuration keys <list-of-controller-configuration-keys>`
```

Configuring controllers reads the controller's configuration and changes the settings that may change after bootstrap. The workers that follow a key act on a change as soon as it is stored.

The operation's required access, rules, and errors are:

- *Required access:* Reading needs only that the caller is logged in. Changing needs {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - A change can set only these keys, and every other key is fixed at bootstrap:
    - Log files: `agent-logfile-max-backups`, `agent-logfile-max-size`, `model-logfile-max-backups`, and `model-logfile-max-size`.
    - Rate limits: `agent-ratelimit-max` and `agent-ratelimit-rate`.
    - Timeouts and limits: `idle-connection-timeout`, `http-server-read-timeout`, `http-server-write-timeout`, `application-resource-download-limit`, `controller-resource-download-limit`, `max-agent-state-size`, `max-charm-state-size`, `max-debug-log-duration`, `migration-agent-wait-time`, `ssh-max-concurrent-connections`, and `dqlite-busy-timeout`.
    - Auditing: `auditing-enabled`, `audit-log-capture-args`, `audit-log-exclude-methods`, `audit-log-max-backups`, and `audit-log-max-size`.
    - Pruning: `max-prune-txn-batch-size`, `max-prune-txn-passes`, `prune-txn-query-count`, and `prune-txn-sleep-time`.
    - Others: `caas-image-repo`, `features`, `juju-mgmt-space`, `public-dns-address`, `query-tracing-enabled`, and `query-tracing-threshold`.
  - Each value must fit the key's type.
  - A change to `caas-image-repo` can only update the credentials of a private image repository that is already set: the repository cannot change, and credentials can be neither added nor removed.
  - Keys that the request leaves out keep their values.
- *Errors:*
  - **`unknown controller config setting "<key>"`** and **`can not change "<key>" after bootstrap`**: *Trigger:* The key does not exist, or it is not in the list above. *Remediation:* Set a key from the list.
  - **`unable to coerce controller config key "<key>": <reason>`**: *Trigger:* The value does not fit the key's type. *Remediation:* Correct the value.
  - **`cannot parse caas-image-repo: <reason>`**, **`cannot change caas-image-repo as it is not currently set`**, **`cannot change caas-image-repo: repository read-only, only authentication can be updated`**, **`cannot change caas-image-repo: unable to remove authentication details`**, and **`cannot change caas-image-repo: unable to add authentication details`**: *Trigger:* The value is not a valid repository, no repository is set, or the change breaks one of the `caas-image-repo` rules above. *Remediation:* Keep the repository and the use of credentials as they are, and change only the credentials.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.

(controller-ha)=
### Making controllers highly available

```{ibnote}
See also: {ref}`Juju | Make a controller highly available <make-a-controller-highly-available>`, {ref}`High availability <high-availability>`
```

Making controllers highly available adds controller nodes, so that the controller keeps working when a machine fails. A node is a unit of the controller application in the controller model, and the way to add one is the operation that adds a unit to any application. {ref}`High availability <high-availability>` shows what each node runs.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the controller model, and the change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on, as for adding any unit to a machine model. The `HighAvailability` facade's own call that would add nodes, `EnableHA`, is refused for every caller.
- *Rules:*
  - Juju sets no database roles. Dqlite chooses them: by default the first three nodes become voters (nodes that take part in leader elections), the next three become stand-bys (nodes that take a voter's place), and any further node is a spare, and the cluster leader checks the roles every 30 seconds.
  - A controller cannot be scaled down to zero units.
  - A new node joins the database cluster once the controller charm has written the node's database address to the node's controller configuration file, and until then the node's `dbaccessor` worker waits.
  - Removing a controller unit removes the node: the node's addresses, agent version, password, and upgrade record are deleted, and the node's record stays as a dead node, without its Dqlite node ID and address, so that a late worker cannot create it again.
  - The `HighAvailability` facade refuses a Kubernetes controller.
- *Errors:*
  - **`enable HA not supported`**: *Trigger:* A call to `EnableHA`. The client receives it with the code for an unsupported operation. *Remediation:* Add units to the controller application instead.
  - **`cannot scale controller application to 0 units`**: *Trigger:* A request that leaves the controller application with no units. *Remediation:* Keep at least one controller unit.
  - **`adding units to a container-based model is not supported`**: *Trigger:* Adding a unit to the controller application of a Kubernetes controller. The client receives it with the code for an unsupported operation. *Remediation:* None; adding controller nodes is available on machine clouds only.
  - **`high availability on kubernetes controllers not supported`**: *Trigger:* A call to the `HighAvailability` facade on a Kubernetes controller. The client receives it with the code for an unsupported operation. *Remediation:* None; the facade is available on machine clouds only.

(controller-listing-nodes)=
### Listing controller nodes

Listing controller nodes returns the controller nodes with the addresses at which a client can reach each one, in order of node ID. The list is empty while the controller has published no addresses.

The operation's required access is {ref}`login access <user-access-controller-login>` to the controller. A caller without it receives `permission denied`.

(controller-blocks)=
### Listing blocked models and removing blocks

Listing blocked models returns every model that has a block, with its block types, in order of name and then qualifier. Removing blocks removes the blocks of every model on the controller.

The operations' required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - Removing blocks acts on every model on the controller, and the request must say so.
- *Errors:*
  - **`not supported`**: *Trigger:* A request to remove blocks that does not say it means all models. *Remediation:* Set the request's all-models flag, or remove the blocks of one model as {ref}`restricting commands on a model <restrict-commands-on-a-model>` describes.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.

(controller-destroying)=
### Destroying controllers

```{ibnote}
See also: {ref}`Juju | Remove a controller <remove-a-controller>`
```

Destroying controllers removes a controller, its controller model, and, when the request says so, every model it hosts. The controller marks the controller model dying and removes each hosted model as {ref}`Destroying models <model-destroying>` describes.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller. The call must run on the controller model.
- *Rules:*
  - A block of any type on any hosted model refuses the request, and a destroy, remove, or change block on the controller model refuses it too. {ref}`Removing blocks <controller-blocks>` lifts them.
  - A request that does not ask to destroy the hosted models is refused while any hosted model exists.
  - Without the force option, the controller model must have a valid {ref}`credential <credential>`.
  - The request carries the force option, the maximum wait, and whether to destroy the storage of hosted models, and passes them to the removal of the controller model and of each hosted model.
  - A hosted model with persistent storage is removed only when the request says whether to destroy or release the storage.
- *Errors:*
  - **`current model is not the controller model`**: *Trigger:* The call ran on another model. *Remediation:* Run it on the controller model.
  - **`found blocks in controller models`**: *Trigger:* A hosted model has a block. The client receives it with the code for a blocked operation. *Remediation:* Remove the blocks.
  - **`cannot destroy controller with hosted models`**: *Trigger:* Hosted models exist and the request does not ask to destroy them. The client receives it with the code `controller has hosted models`. *Remediation:* Ask to destroy the hosted models in the request, or destroy them first.
  - **`invalid cloud credential, use --force`**: *Trigger:* The controller model's credential is invalid and the request has no force option. *Remediation:* Update the credential, or add the force option.
  - **`hosted model "<uuid>" has persistent storage`**: *Trigger:* A hosted model has persistent storage and the request does not say what to do with it. The client receives it with the code for persistent storage. *Remediation:* Say in the request whether to destroy or release the storage.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.
