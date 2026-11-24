---
myst:
  html_meta:
    description: "Juju controller architecture, API addresses and access, configuration, audit logging, and lifecycle operations."
---

(controller)=
# Controller

A Juju **controller** is the control plane of a Juju deployment: the central service that manages {ref}`models <model>` and keeps deployment state in a {ref}`database <database>`. Each instance of the controller runs a {ref}`controller agent <controller-agent>`, the process that runs the Juju API server. The controller applies changes requested by clients. Other agents connect to the controller to carry out work in its models.

```{ibnote}
See also:

- {ref}`Juju CLI | Manage controllers <manage-controllers>`
- {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
- {ref}`JAAS | Manage Juju controllers <jaas:manage-juju-controllers>`
```

## Controller deployment and topology

(controller-bootstrap)=
### Bootstrap

```{ibnote}
See also:

- {ref}`Juju CLI | Bootstrap a controller <bootstrap-a-controller>`
- {ref}`Terraform Provider for Juju | Bootstrap a controller <tfjuju:bootstrap-a-controller>`
- {ref}`JAAS | Bootstrap a controller for JIMM <jaas:add-a-juju-controller>`
```

(controller-charm)=
(controller-application)=

````{container}
```{ggarch}
:file: ../juju.ggarch
:view: Controller application on one machine
:caption: On a machine cloud, bootstrap creates the `controller` model, deploys the `controller` application with its initial unit, `controller/0`, and assigns the unit to a controller machine. The controller agent on that machine runs the API server and Dqlite in the same process.
:alt: On the left, the controller model contains the controller application and its initial unit, controller/0. An arrow shows the unit's assignment to the controller machine on the right. The machine contains the controller agent, which contains the API server and Dqlite.
```

````

Bootstrap creates a {ref}`model <model>` named `controller` and deploys an {ref}`application <application>` of the same name from the `juju-controller` {ref}`charm <charm>`. The application starts with one {ref}`unit <unit>`, `controller/0`. Each unit corresponds to a controller node, whose {ref}`controller agent <controller-agent>` runs the API server and a [Dqlite](https://canonical.com/dqlite) database node in the same process. Bootstrap also grants the initial user `admin` {ref}`controller superuser access <user-access-controller-superuser>`.

On a machine cloud, bootstrap provisions a machine for the controller. On Kubernetes, it creates a namespace named `controller-<controller name>`, a single-replica StatefulSet, and a Service that exposes the API and SSH ports. The default Service type depends on the provider.

On Kubernetes, bootstrap waits for the controller Service to have an address before it creates the StatefulSet. A timeout waiting for that address points to Service provisioning, not pod startup. Even after the controller pod is running, bootstrap can still fail to reach the controller API.

After a failed bootstrap, check whether cloud resources remain before retrying. On Kubernetes, a leftover controller namespace blocks a retry with the same name on the same cluster.

(controller-storage)=
### State and object storage

The controller keeps deployment state in a {ref}`database <database>` and stores binary data separately in an object store. The object store initially saves its data as files in the `objectstore` directory under the controller agent's data directory. No controller configuration key selects the storage backend.

(controller-nodes)=
### Controller nodes

(dqlite-cluster-of-the-controllers)=
(controller-high-availability)=

````{container}
```{ggarch}
:file: ../juju.ggarch
:view: Controller high availability
:caption: One Juju controller can span several nodes. In this machine-cloud example, three machines each run a controller agent with its own API server and Dqlite node. The dashed outline groups the Dqlite nodes into one shared database cluster.
:alt: Three controller machines sit side by side. Inside each machine, a controller agent box contains an API server above a Dqlite node. A dashed outline spans the machines and encloses the Dqlite nodes but not the API servers.
```

````

Each unit of the {ref}`controller application <controller-application>` corresponds to a node of the same controller. The node's ID is the number at the end of its unit name: `controller/0` has ID `0`.

Adding controller units is how you set up controller {ref}`high availability <high-availability>`; there is no separate HA setting. The units still belong to one Juju controller and, as their Dqlite nodes join, they share one database cluster. Each controller node runs on a separate machine on a {ref}`machine cloud <machine-cloud>` or in a separate pod of the controller StatefulSet on a {ref}`Kubernetes cloud <kubernetes-cloud>`.

Adding a second unit starts the transition to a multi-node cluster, but a two-node Dqlite cluster settles on one voter. With three healthy voters, the database can keep quorum after losing one of them. A requested unit count does not tell you how many nodes have joined or can vote.

A new Dqlite node waits for the controller charm to supply its bind address and at least one peer address. The bootstrap node can switch from loopback to its cluster address when the configuration lists more than one member and includes that node's address.

Machine status for the controller model reports a Dqlite role (`cluster-role`) of `voter`, `standby`, or `spare` for a node listed in the Dqlite cluster. Juju omits the role if the cluster does not list that node or if the machine is not a controller node. The legacy `has-vote` and `wants-vote` fields are both true for voters and standbys, but a standby does not vote. Use the role to tell them apart, not to judge whether the node is reachable. For the Juju CLI status display, see {ref}`juju status <command-juju-status>`.

If Dqlite is not running on a previously clustered node and the cluster configuration lists only that node, the controller reconfigures the cluster to contain only the local node and tries to restart it. The controller does not take this recovery action while Dqlite is running on the node.

(adding-a-controller)=
#### Adding nodes

```{ibnote}
See also:

- {ref}`Juju CLI | Make a controller highly available <make-a-controller-highly-available>`
- {ref}`Terraform Provider for Juju | Enable controller high availability after bootstrap <tfjuju:enable-controller-high-availability-post-bootstrap>`
```

Adding nodes requires {ref}`write access <user-access-model-write>` to the controller model and is prevented by a {ref}`change block <model-command-blocks>`. The CLI's `juju add-unit` takes the number to add on either cloud: it calls `AddUnits` on machine clouds and increases the application's desired scale on Kubernetes.

- *Error message:* `adding units to a container-based model is not supported`. *Trigger:* A direct API client calls `AddUnits` on a Kubernetes model connection and passes the permission and block checks. *What to do:* Use `ScaleApplications` instead.

(removing-a-controller)=
#### Removing nodes

```{ibnote}
See also: {ref}`Juju CLI | Reduce controller capacity <reduce-controller-capacity>`
```

Reducing capacity requires {ref}`write access <user-access-model-write>` to the controller model. On machine clouds, remove a unit or its host machine; a {ref}`remove or change block <model-command-blocks>` prevents removal. On Kubernetes, reduce the application's desired scale; a change block prevents that request.

```{warning}
Kubernetes scaling refuses to leave zero controller units. Machine-cloud removal does not run that check. Before removing nodes, check that enough voting nodes will remain for the availability you need.
```

These errors distinguish scaling the controller application from trying to remove it:

- *Error message:* `cannot scale controller application to 0 units`. *Trigger:* A scale request would leave no controller units. *What to do:* Keep at least one unit.
- *Error message:* `removing the controller application not supported`. *Trigger:* A request tries to remove the controller application itself. *What to do:* Reduce capacity using the operation for your cloud type described above. To remove the whole controller, see {ref}`controller destruction <destroying-a-controller>`.

(controller-model-blocks)=
(controller-model-creation)=
(controller-model-migration)=
(controller-hosted-model-upgrades)=
(controller-hosted-models)=
### Models hosted by the controller

A controller's **hosted models** are the {ref}`workload models <model>` it manages. Applications in a hosted model use resources on that model's cloud, which can differ from the controller's cloud. The `controller` model runs the controller itself and is not a hosted model.

## Controller API connections and access

(controller-api-addresses)=
(controller-api-addresses-for-agents)=
(controller-api-addresses-for-clients)=
### API addresses and node discovery

The controller publishes separate API address lists so that clients, agents, and the controller's own nodes know which addresses to use. The lists can overlap. The controller recomputes them when membership, configuration, or network addresses change. Agents update their connection configuration when usable agent addresses change.

On a machine cloud, the controller can narrow its choice of addresses for agents and peer nodes to the network {ref}`space <space>` named in {ref}`juju-mgmt-space <controller-config-juju-mgmt-space>`. During normal operation, if no address for a unit matches the space, the controller falls back to the unit's eligible addresses. If the named space does not exist, bootstrap ignores the setting, but the running controller cannot use it to refresh those lists. The setting does not affect client addresses or restrict network access. On Kubernetes, the controller ignores the setting: clients and agents share the Service addresses, while the controller uses per-node DNS names for peers at bootstrap and selects addresses from each controller pod at runtime.

At sign-in, the controller returns the client address list if the request identifies the caller as a user, and the agent address list otherwise. **Node discovery** is a separate way for a user with {ref}`controller login access <user-access-controller-login>` on a machine model connection to find individual controller nodes. It returns node IDs and their published client addresses in ID order, without shared endpoints. If no node has published an individual address, the list is empty.

Node discovery is not supported on a Kubernetes model connection:

- *Error message:* `facade "HighAvailability" not supported on container models`. *Trigger:* The client calls node discovery on a Kubernetes model connection. *What to do:* Use the addresses returned at login instead.

(controller-certificates-and-public-dns-name)=
### TLS certificates and public DNS

A TLS certificate lets clients verify the controller they reach. The controller can serve a certificate from its own certificate authority (CA), or use ACME to obtain one automatically for the name in {ref}`autocert-dns-name <controller-config-autocert-dns-name>`. The controller reports that name at sign-in. The {ref}`public-dns-address <controller-config-public-dns-address>` setting publishes an address instead; it does not configure certificate issuance. For automatic certificates, plan for HTTPS on TCP port 443 and the ACME HTTP challenge on TCP port 80.

(controller-version-and-identity-provider)=
### Health and version

The controller's HTTPS `/health` endpoint reports `starting`, `running`, or `stopping`, with HTTP 503 unless the server is `running`. It does not check cluster health, credentials, or access to a model. Users on a {ref}`controller-only connection <controller-api-versions>` can query the running controller binary's version and git commit. For version requirements, see {ref}`Juju component compatibility <juju-cross-version-compatibility>`. For the identity provider URL, see {ref}`registration and sign-in <controller-authentication>`.

(controller-authentication)=
### Registration and sign-in

```{ibnote}
See also:

- {ref}`Juju CLI | Register a controller <register-a-controller>`
- {ref}`Juju CLI | Log in to a controller <manage-a-users-login-status>`
```

Registering a controller gives your client the information it needs to recognize and connect to an existing controller. Signing in authenticates you to the controller. The controller checks your access for the API connection, which can select a model. A client can combine registration and sign-in in one workflow, but registration does not grant access.

When you register by invitation, your client supplies an initial password using a single-use activation key. The controller sets the password and consumes the key before sending its UUID and CA certificate, so a failed response does not necessarily mean the key is still usable.

For accounts managed by the controller (local users), the controller accepts a password or a login macaroon, a token used to verify the user's identity. A local user invited with an activation key needs an initial password before signing in with a password. The controller refuses sign-in for disabled accounts. A macaroon discharge challenge asks for additional proof; it does not establish a session.

For external sign-in, the controller reports the configured {ref}`identity provider URL <controller-config-identity-url>` to users on a {ref}`controller-only connection <controller-api-versions>`, or returns an empty value if no URL is configured. Token authentication requires a configured {ref}`login-token-refresh-url <controller-config-login-token-refresh-url>` from which the controller can obtain verification keys. Both settings are set at bootstrap.

The controller distinguishes a local user credential problem from a temporary limit on agent logins:

- *Error message:* `invalid entity name or password`. *Trigger:* In a local password sign-in, the account is unknown, removed, or disabled, or the password is invalid. *What to do:* Check the account and password; an administrator can check whether the account is enabled.
- *Error message:* `try again`. *Trigger:* The agent login rate limit is exhausted. *What to do:* Retry after capacity is available.

The controller applies changes to {ref}`agent-ratelimit-max <controller-config-agent-ratelimit-max>` and {ref}`agent-ratelimit-rate <controller-config-agent-ratelimit-rate>` without restarting. These keys do not limit user logins.

(connection-scopes)=
(controller-api-versions)=
(controller-access)=
### Access levels and grants

```{ibnote}
See also: {ref}`Juju CLI | Manage controller access <manage-controller-user-access>`
```

A controller **grant** gives a user a level of access to the controller: {ref}`login access <user-access-controller-login>` or {ref}`superuser access <user-access-controller-superuser>`. A grant to use a model or cloud is separate. For example, {ref}`add-model <user-access-cloud-add-model>` applies to a cloud, not to the controller.

The controller requires controller access for a **controller-only API connection**, which does not select a model. A **model-scoped API connection** selects one model and does not grant access to it. The controller checks model and controller access separately and requires controller access for a model-scoped connection unless you enable {ref}`allow-model-access <controller-config-allow-model-access>`.

For clients that connect directly to the API, the WebSocket path `/api` (or `/`) opens a controller-only connection. `/model/<uuid>/api` opens a model-scoped connection, even when the UUID belongs to the `controller` model. Operations such as reading controller configuration require a controller-only connection.

If a user cannot log in because of missing access:

- *Error message:* `permission denied`. *Trigger:* The user lacks a grant that the connection requires. *What to do:* Use an account with the required grant.

A controller `superuser` can change controller access and read it for any user; other users can read only their own. These calls return permission failures in the affected result entries.

(controller-ssh-server-host-key-and-port)=
### SSH server

The controller SSH server lets an authorized user connect through the controller to a machine or unit. The {ref}`SSH server reference <ssh-server>` covers how clients find the server, verify its identity, and reach a supported target.

## Controller configuration and audit

(reading-and-setting-controller-configuration)=
### Reading and updating configuration

```{ibnote}
See also:

- {ref}`Juju CLI | Configure a controller <configure-a-controller>`
- {ref}`Terraform Provider for Juju | Configure a controller after bootstrap <tfjuju:configure-a-controller-post-bootstrap>`
```

Controller configuration governs how the controller itself runs and is separate from an individual model's configuration. A user on a {ref}`controller-only connection <controller-api-versions>` can read all controller settings, including defaults.

The controller model's **cloud specification** describes its cloud and is separate from controller configuration. This read is available to users on a controller-only connection. The controller refuses entries for any other model, and includes the cloud credential only for a user with {ref}`controller superuser access <user-access-controller-superuser>` or {ref}`admin access to the controller model <user-access-model-admin>`.

Updating controller configuration requires controller superuser access. A {ref}`model change block <model-command-blocks>` does not prevent the update. The controller saves all changes together or none at all; it leaves settings you don't specify untouched and doesn't remove keys. The controller checks whether it can update each key and convert the value to the key's type, but doesn't repeat all of its bootstrap checks. The {ref}`list of controller configuration keys <list-of-controller-configuration-keys>` identifies which keys can change after bootstrap.

For a rejected configuration update, check the message to see whether the problem is with the key or its value:

- *Error message:* `unknown controller config setting "<key>"`. *Trigger:* The key is not a controller setting. *What to do:* Use a key from the key list.
- *Error message:* `can not change "<key>" after bootstrap`. *Trigger:* The key is recognized but is not mutable. *What to do:* Set it at bootstrap.
- *Error message:* `unable to coerce controller config key "<key>"`. *Trigger:* The value cannot be converted to the key's type. *What to do:* Supply a value of that type.

For {ref}`caas-image-repo <controller-config-caas-image-repo>`, the controller also checks that the repository and its authentication mode stay the same. Only existing authentication details can change. The following errors identify which check failed:

- *Error message:* `cannot parse caas-image-repo: ...`. *Trigger:* The supplied value cannot be parsed. *What to do:* Supply valid repository and authentication details.
- *Error message:* `caas-image-repo expected a string got ...`. *Trigger:* The supplied value is not a string. *What to do:* Supply a string.
- *Error message:* `cannot change caas-image-repo as it is not currently set`. *Trigger:* The controller has no existing value. *What to do:* Set the repository at bootstrap.
- *Error message:* `cannot change caas-image-repo: repository read-only, only authentication can be updated`. *Trigger:* The repository differs from the current repository. *What to do:* Keep the repository unchanged.
- *Error message:* `cannot change caas-image-repo: unable to add authentication details`. *Trigger:* The existing repository has no authentication details. *What to do:* Keep the existing authentication mode.
- *Error message:* `cannot change caas-image-repo: unable to remove authentication details`. *Trigger:* The existing repository requires authentication details. *What to do:* Supply replacement authentication details.

Saving configuration does not mean every consumer has applied it:

- Changing {ref}`query-tracing-enabled <controller-config-query-tracing-enabled>`, {ref}`query-tracing-threshold <controller-config-query-tracing-threshold>`, or {ref}`dqlite-busy-timeout <controller-config-dqlite-busy-timeout>` causes the controller agent to restart.
- The HTTP server reads its idle, read, and write timeouts at startup. The controller reads the debug-log duration limit for each new debug-log request.
- An existing machine agent reads its logfile size and backup count from its own agent configuration at startup, not from the current controller settings.

(controller-audit-logging)=
### API audit logging

```{ibnote}
See also:

- {ref}`Juju CLI | Configure controller auditing <configure-controller-auditing>`
- {ref}`Terraform Provider for Juju | Configure controller auditing <tfjuju:configure-a-controller-post-bootstrap>`
- {ref}`Juju CLI | View the audit log on a controller machine <view-the-log-files>`
```

API audit logging records what authenticated users do over the controller API. When auditing is on, the controller writes those records to `audit.log` in the controller agent's log directory. It groups the activity on one connection into a **conversation**. It does not record agent or anonymous sessions as user audit conversations, and it includes request arguments only when argument capture is on.

The controller skips a conversation if the user calls only excluded methods. If the user calls any method outside the exclusion list, the controller writes the conversation, including the excluded calls. Excluding a method does not guarantee that it never appears in the log.

Changes to whether the controller audits calls, captures arguments, or excludes methods affect new logins, not existing connections. The controller sets the rotation size and number of backups when it first creates the audit log target. It reuses that target if you turn auditing off and back on, without applying any new rotation settings to the existing target. See {ref}`audit-log-exclude-methods <controller-config-audit-log-exclude-methods>` and the other audit entries in the {ref}`list of controller configuration keys <list-of-controller-configuration-keys>`.

## Controller upgrades, backup and destruction

(controller-upgrade)=
### Upgrading the controller

```{ibnote}
See also:

- {ref}`Juju CLI | Upgrade a controller's patch version <upgrade-a-controllers-patch-version>`
- [JAAS | Upgrade a Juju controller](https://documentation.ubuntu.com/jaas/latest/howto/manage-juju-controllers/#upgrade-a-juju-controller)
```

A controller upgrade sets the agent version the controller nodes should run. A user with {ref}`controller superuser access <user-access-controller-superuser>` or {ref}`write access <user-access-model-write>` to the controller model can request an upgrade. A {ref}`change block <model-command-blocks>` on that model prevents the upgrade. The controller accepts only its current major and minor version series and will not lower the target version. Before it can raise the target, nodes below the current target must catch up, and every controller node architecture must have an agent binary for the requested version. {ref}`Workload model agents <model-agent-upgrades>` have a separate upgrade target.

If you don't specify a version, the controller selects the highest available patch in its current series from the requested agent binary stream, or the controller model's stream if you don't specify one. It then checks that version against the requirements above. A **dry run** checks the target and reports the version without changing it. When the controller accepts an upgrade, it sets the target and responds without waiting for the upgrade to finish. Completion requires all nodes to finish their own steps before the shared upgrade can finish.

A failed upgrade request does not always leave the target unchanged. The controller updates the controller model's agent target first. If the subsequent update of the controller target fails, the new agent target can already be in use. Check the controller model's target and the running node versions before retrying.

If the controller refuses the upgrade, the response distinguishes an incompatible target version, missing binaries, and nodes that haven't caught up:

- *Error message:* `controller agent binaries are not available for version "<version>"`. *Trigger:* Binaries for the explicitly requested version are missing for at least one controller node architecture. *What to do:* Choose a patch with binaries for every controller node architecture.
- *Error message:* `controller upgrading is blocked for reason: <reason>`. *Trigger:* At least one node has not reached the current controller target. *What to do:* Wait for the named nodes to catch up; check their status if they do not.
- *Error message:* `cannot upgrade the controller to version "<version>" because it is lower than the current running version`. *Trigger:* The explicitly requested version is lower than the current target. *What to do:* Choose that target or a later patch in the same series.
- *Error message:* `cannot upgrade the controller to a version that is more than a patch version increase`. *Trigger:* The explicitly requested version is not lower than the target, but its major or minor version differs. *What to do:* Choose a patch in the current series or migrate the models to a new controller.

During upgrade maintenance, user connections can call model status, agent binary lookup, ping, and SSH discovery and key-related API methods. Non-controller agents and anonymous logins can be refused; controller-machine agents can still connect for maintenance.

An API call that is restricted during maintenance fails as follows:

- *Error message:* `upgrade in progress`. *Trigger:* A user calls an API method that is unavailable during an upgrade. *What to do:* Wait for the upgrade to finish before retrying. This refusal does not, by itself, mean credentials need to change.

(controller-backup-and-recovery)=
### Backing up the controller

```{ibnote}
See also:

- {ref}`Juju CLI | Back up a controller <back-up-a-controller>`
- {ref}`Juju CLI | Provision a replacement controller machine from a backup <provision-a-replacement-controller-machine-from-a-backup>`
```

A controller backup is an archive that the controller creates and sends to the requesting client. A user with {ref}`controller superuser access <user-access-controller-superuser>` can request one from a machine or Kubernetes controller.

The controller exports the controller database and each model database separately. The archive is not a single point-in-time snapshot of the controller and its models.

The controller does not keep a copy for later download. If the transfer is interrupted, the client must request a new backup.

The controller refuses the legacy backup creation and download operations:

- *Error message:* `create-backup from clients older than 4.1 is not supported; use a 4.1 or newer client`. *Trigger:* A client calls the legacy backup creation operation. *What to do:* Use a Juju 4.1 or newer client.
- *Error message:* `downloading backups from clients older than 4.1 is not supported; use a 4.1 or newer client`. *Trigger:* A client calls the legacy backup download operation. *What to do:* Create and save a new backup with a Juju 4.1 or newer client.

(destroying-a-controller)=
### Destroying the controller

```{ibnote}
See also:

- {ref}`Juju CLI | Destroy a controller <destroy-a-controller>`
- {ref}`Terraform Provider for Juju | Remove a controller <tfjuju:remove-a-controller>`
- [JAAS | Remove a Juju controller](https://documentation.ubuntu.com/jaas/latest/howto/manage-juju-controllers/#remove-a-juju-controller)
```

Destroying a controller schedules removal of its controller model. If it still hosts workload models, you must choose to destroy them too or remove them separately first. The controller requires {ref}`controller superuser access <user-access-controller-superuser>` on a {ref}`controller-only connection <controller-api-versions>`. It checks which hosted models have been activated, including any already marked dying.

When removing a hosted model with persistent storage, the controller requires you to choose whether to release the storage from Juju management or destroy it. This choice does not apply to the controller model: removal destroys its storage.

The controller checks for {ref}`model command blocks <model-command-blocks>` on hosted models before it checks whether you chose to destroy those models. A block stops the operation even if you did not choose to destroy the hosted models. The controller also checks for a block on its own model before checking that model's cloud credential. Forcing destruction skips the credential check, not the block checks.

After these checks, the controller marks its own model as dying and schedules its removal, then processes hosted models one by one. The controller refuses to remove a model during the import phase of migration and does not change that model's life state. A successful response does not mean that the controller has removed the resources. The controller waits to remove its own model until the remaining models are gone.

```{warning}
A failed destruction request does not roll back earlier changes. If you have not chosen what to do with a hosted model's persistent storage, the controller may already have scheduled removal of its own model and earlier hosted models. It marks the affected hosted model as dying if it was alive, then returns the storage error without scheduling a removal job for that model in this request. Check the remaining models and decide what to do with persistent storage before retrying.
```

If a destroy request fails, use the message to identify what blocked it and what to check before retrying:

- *Error message:* `found blocks in controller models`. *Trigger:* An activated hosted model has a block. *What to do:* Clear the blocks before retrying.
- *Error message:* `cannot destroy controller with hosted models`. *Trigger:* The controller still has an activated hosted model, but you did not choose to destroy hosted models as part of the operation. *What to do:* Remove the hosted models separately or choose to destroy them along with the controller.
- *Error message:* The message set with the block, or `the operation has been blocked` if no message was set. *Trigger:* The controller model has a destroy, remove, or change block. *What to do:* Remove the block.
- *Error message:* `invalid cloud credential, use --force`. *Trigger:* The controller model has no valid cloud credential, and you did not request forced destruction. *What to do:* Restore a valid credential or explicitly force destruction in your client.
- *Error message:* `hosted model "<uuid>" has persistent storage`. *Trigger:* A hosted model has persistent storage, and you did not choose whether to release or destroy it. *What to do:* Check which models are already dying, then decide whether to release or destroy their persistent storage before retrying.
