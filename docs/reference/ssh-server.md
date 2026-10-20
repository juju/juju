---
myst:
  html_meta:
    description: "SSH server reference: the service in a controller that proxies users' SSH sessions to machines and Kubernetes units, the records it keeps, the changes it follows, and the operations and errors a client meets."
---

(juju-ssh)=
# SSH server

```{ibnote}
See also: {ref}`Juju | Access a machine via SSH <access-a-machine-via-ssh>`, {ref}`Juju | Manage SSH keys <manage-ssh-keys>`
```

The **SSH server** is the service in a {ref}`controller <controller>` that lets a {ref}`user <user>` open an SSH session, forward a port, or copy files on a {ref}`machine <machine>` or in the container of a Kubernetes {ref}`unit <unit>`, with no direct network path from the client to the target. Users' keys stay off the machines, so the controller is the one place that decides who has access and sees who connects. Every controller node runs one SSH server, on port 17022 by default, and it starts and stops with the controller. It ends the user's SSH connection itself, so that Juju can check who connects and to what, and then opens its own connection to the target and passes the traffic between the two.

(ssh-server-data-model)=
## SSH server data model

```{ggarch}
:file: ../juju.ggarch
:view: SSH server data model
:caption: The model database keeps three kinds of host key for the SSH server and one kind of request. A machine reports many host keys of its own (a machine has many, a key belongs to one machine), and the SSH server keeps one virtual host key for each machine and one for each Kubernetes unit, which it generates when it first needs them. A connection request asks one machine to call the controller back (a machine can have many requests, a request names one machine) and carries an expiry after which the controller removes it. The machine and the unit are described on their own pages and are shown for context: all boxes are records, and the two with a second, outlined arrow mark in the corner are the records that other pages describe. The controller database also keeps two records of its own, the SSH server's host key and its port, which have no relations and are not drawn.
:alt: Six boxes of the same size inside a group named Model database, each with a filled table mark in the corner, and two of them (Machine and Unit) also with an outlined arrow mark for records that other pages describe. Machine host key is to the left of Machine (m to 1). Machine virtual host key is above Machine (1 to 1). Connection request is below Machine (m to 1). Unit virtual host key is above Unit (1 to 1), and Unit stands to the right of Machine without a line to it.
```

The SSH server data model is the set of records that the {ref}`controller database <database>` and the {ref}`model database <database>` keep so that the SSH server can prove its identity to clients, listen on the right port, and reach machines that do not accept connections from the controller. A host key is the key by which an SSH server proves its identity to a client; a client remembers it and warns when it changes. The records, with the rules that apply to each, are:

- **Controller host key:** The host key that the SSH server presents on the outer connection of every session (the user's connection to the SSH server, see {ref}`connecting to machines <ssh-server-connecting-machine>`), kept in the controller database with its public half.
  - *Rules:*
    - The controller keeps at most one.
    - Its algorithm is one of `ssh-rsa`, `ecdsa-sha2-nistp256`, and `ssh-ed25519`.
- **Server port:** The TCP port that the SSH server listens on, kept in the controller database.
  - *Rules:*
    - The controller keeps at most one.
- **Machine host key:** A host key of the SSH server that runs on a machine itself, as the machine's {ref}`agent <agent>` reported it.
  - *Rules:*
    - A machine has any number of them, and each belongs to one machine.
- **Machine virtual host key:** The host key that the SSH server presents on the inner connection of a session (the connection that runs through the outer one to the target) to a machine, so that each machine has an identity of its own.
  - *Rules:*
    - A machine has at most one.
- **Unit virtual host key:** The same for a unit in a Kubernetes model, which has no machine.
  - *Rules:*
    - A unit has at most one.
- **Connection request:** A one-time request that asks a machine to call the controller back. It names the machine, a tunnel ID (a UUID), an expiry, the addresses of the controller node that holds the waiting session, the port of the machine's SSH server (0 lets the agent read it from the machine's SSH server configuration), and a one-time public key for this connection.
  - *Rules:*
    - The tunnel ID is unique.
- **Machine and unit:** The records on the far end of the host keys and requests. Other pages own them: see {ref}`machine <machine>` and {ref}`unit <unit>`.

The controller's host key is a row in the table `controller_ssh_host_key` and its port is a row in `controller_ssh_server_port`, and each table holds at most one row. In the model database, a machine's own host keys are rows in `machine_ssh_host_key`, its virtual host key is a row in `machine_virtual_ssh_host_key`, a Kubernetes unit's virtual host key is a row in `unit_virtual_ssh_host_key`, and a connection request is a row in `ssh_connection_request`. Both databases keep the lookup table `ssh_key_algorithm_type`, which holds the three algorithm names. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(ssh-server-change-streams)=
## SSH server change streams

SSH server change streams are the changes that the SSH server and the machine agents follow, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed), and the SSH server follows its port and the controller configuration, while each machine agent follows the connection requests that name its machine:

- **Server port:** The SSH server follows its port record. When the value changes, the server stops, waits until the old listener has released the port, and starts again on the new one. Stopping closes every connection that is open.
- **Controller configuration:** The SSH server follows the controller configuration. When {ref}`ssh-max-concurrent-connections <controller-config-ssh-max-concurrent-connections>` changes, it restarts in the same way with the new limit. Other keys leave the server running.
- **Connection requests:** A machine agent follows the connection requests that name its own machine, and hears about each addition or update.

(ssh-server-operations)=
## SSH server operations

SSH server operations are the actions that a {ref}`client <client>` and a machine {ref}`agent <agent>` can take through the SSH server and the controller API (the set of calls that clients and agents make to the controller): connecting to machines and to Kubernetes units, relaying sessions, finding the server, addressing targets, and checking host keys.

(ssh-server-connecting-machine)=
### Connecting to machines through the SSH server

```{ibnote}
See also: {ref}`Juju | Access a machine via SSH <access-a-machine-via-ssh>`
```

```{ggarch}
:file: ../juju.ggarch
:slides: SSH proxy connections | Connecting to a machine: authorization | Connecting to a machine: reverse tunnel
:caption: A connection to a machine runs as two SSH connections that both end at the controller. It first passes the controller's checks, and then reaches the machine through a tunnel that the machine opens.
:slide-captions: The controller terminates two SSH connections. The client opens an outer connection to the controller's SSH tunnel server, which has a fixed host key and authenticates the user. Through a channel of the outer connection, the client opens an inner connection to a target SSH server inside the controller, which presents the target's virtual host key. The target SSH server then proxies the session to the machine or unit. | The client opens the outer connection as its Juju user and presents a key, and the SSH tunnel server accepts it only if the key is one of the keys stored for that user. The client then asks for a channel to the target's virtual hostname. The server checks that the user has admin access to the model, or superuser access to the controller, and that the model authorizes the key, and it reads the target's virtual host key from the model database, creating it on first use. The client then runs the inner SSH handshake with the target SSH server through the channel. | The target SSH server stores a connection request with a key that is valid for this one connection and an expiry 60 seconds away. The machine agent hears of the request, adds the key to the machine's authorized keys, and opens a tunnel to the controller's API server, authenticating as the machine, which hands it to the waiting target SSH server. The agent pipes the tunnel to the machine's own SSH server. The target SSH server logs in through the tunnel as the user ubuntu with the one-time key, after checking the machine's host keys, and then passes the user's session, port forwards, and file transfers through.
:alt: Three slides. The first shows the client with an outer SSH tunnel to the SSH tunnel server on port 17022 inside the Juju controller, which forwards the inner SSH connection to the target SSH server inside the controller, which presents the virtual host key and proxies the session to the machine or unit. The second is a sequence diagram with the client, the SSH tunnel server, the controller database, and the model database: the client presents a key, the server reads the user's stored keys, the client asks for a channel to the target's virtual hostname, the server checks the user's access and the key's authorization and reads the target's virtual host key, and the channel opens. The third is a sequence diagram with the target SSH server, the model database, the machine agent, the API server, and the machine SSH server: the target server stores a connection request, the machine agent reads it, adds a one-time key, and opens a tunnel to the API server, which hands it to the target server, the agent pipes the tunnel to the machine's SSH server, the target server logs in as the user ubuntu, and then passes the user's traffic through.
```

Connecting to machines through the SSH server opens a session (a shell or one command), a port forward, or a file transfer on a machine, or on the machine that runs a unit, through the SSH server. Every connection runs as two SSH connections that both end at the controller. The outer connection ends at the SSH tunnel server, the part of the SSH server that listens on the port. It authenticates the user and asks the server, with SSH's direct TCP forwarding (a request to open a channel to a named host and port), for a channel to a **virtual hostname**, a name under `juju.local` that identifies a target in a model. The inner connection runs through that channel and ends at the target SSH server, a second server inside the controller that presents the target's virtual host key. A machine's virtual hostname is `<machine>.<model UUID>.juju.local`, where the slashes of a nested machine's name become dashes, and a unit's is `<unit number>.<application>.<model UUID>.juju.local`. The machine calls the controller, so only the machine needs a network path, the one to the controller's API addresses. The machine's SSH server can stay closed to the network, and the firewall rules that machine agents already need suffice.

The operation's required access, rules, and errors are:

- *Required access:* The user needs a {ref}`key stored for them <ssh-key>` that the model authorizes, and either {ref}`admin access <user-access-model-admin>` to the model or {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - The user name of the outer connection is the Juju user, such as `admin` or `alice@external`, and the key must be one that the user has stored.
  - The access and the key are checked against the model that the virtual hostname names, once for each requested channel.
  - A session logs in to the machine as the user `ubuntu`, whatever user name the client gives on the inner connection.
  - A target can be a machine or a unit that runs on a machine, and a unit presents its machine's virtual host key. A machine target that names a nested machine (a container machine on another machine, such as `1/lxd/0`) is refused: connect to its parent machine.
  - The server serves a shell or one command, local port forwarding, and the SFTP subsystem, which carries file copies.
  - The server accepts at most {ref}`ssh-max-concurrent-connections <controller-config-ssh-max-concurrent-connections>` connections at once (100 by default) and refuses the rest.
  - A request expires 60 seconds after the server stores it, and storing or watching requests removes expired ones. The machine has 60 seconds to call back, and the server also waits, within the same 60 seconds, until the machine's own host keys are on record. The tunnel is accepted only from the machine that the request names, and the one-time key is removed from the machine when the tunnel closes.
  - A connection to a machine needs the machine's agent to run. If the agent cannot start, use the cloud's own tools or the cloud-init user data to reach the machine instead (see {ref}`access-a-machine-via-ssh`).
- *Errors:* An SSH client shows each of the following in its own way. The first reaches the client before the SSH protocol starts, the next four as the failure to authenticate or to open the channel, and the last as text on the session's standard error with exit status 1:
  - **`too many connections.`**: *Trigger:* The server already holds the maximum number of connections. *Remediation:* Close idle sessions, or raise `ssh-max-concurrent-connections`, which a controller administrator can update.
  - **The server rejects the key and the client reports that public key authentication failed:** *Trigger:* The user name is not a Juju user, or the key is not one of the user's stored keys. *Remediation:* Connect as the Juju user and add the key to that user.
  - **`unauthorized`**: *Trigger:* The user has neither admin access to the model nor superuser access to the controller. *Remediation:* Ask for admin access to the model.
  - **`failed to authorize access to destination: public key "<comment>" used to authenticate is not associated with model "<UUID>", add the key to the model or specify a different key`**: *Trigger:* The key is stored for the user but the model does not authorize it. *Remediation:* Authorize that key on the model, or offer a different key.
  - **`Failed to parse destination address`** and **`failed to resolve destination: <cause>`**: *Trigger:* The channel names something that is not a virtual hostname, or a target that the server cannot resolve, such as a machine or unit that does not exist or a nested machine (`cannot SSH directly to nested machine "1/lxd/0", connect to parent machine "1" instead`). *Remediation:* Use the virtual hostname that {ref}`addressing <ssh-server-addressing>` returns.
  - **`failed to connect to machine: <cause>`**: *Trigger:* The cause is `machine "<name>" does not exist`, `destination model "<UUID>" is not machine based model`, or `waiting for tunnel: context deadline exceeded`, which means the machine did not call back within 60 seconds. *Remediation:* Check the name, use a Kubernetes target for a Kubernetes model, or check that the machine's agent runs and can reach the controller's API addresses.

(ssh-server-connecting-kubernetes)=
### Connecting to Kubernetes units through the SSH server

```{ibnote}
See also: {ref}`Juju | Access a unit via SSH <access-a-unit-via-ssh>`
```

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy Kubernetes
:caption: The controller turns the SSH session into an exec stream through the Kubernetes API server, which runs the command in the target container inside the unit's pod.
:alt: The controller opens an exec stream to the Kubernetes API server, which runs commands in the target container inside a unit pod.
```

Connecting to Kubernetes units through the SSH server opens a session, a port forward, or a file transfer in a container of a unit's pod, through the SSH server. The server reaches the pod through the {ref}`Kubernetes cloud's <kubernetes-cloud>` API server, which the controller already needs to manage the cloud, so no machine calls back.

The operation's required access, rules, and errors are:

- *Required access:* The same as for connecting to machines.
- *Rules:*
  - The target must be a container, so the virtual hostname is `<container>.<unit number>.<application>.<model UUID>.juju.local`. A hostname without the container is not served in a Kubernetes model.
  - The server finds the unit's pod in the namespace of the model (for the controller's own model, the controller's namespace) and runs the command there with the model's cloud credential. A session without a command runs `/bin/sh`. The session runs as the user that the container runs as.
  - The server forwards signals and window size changes to the container, and serves local port forwarding and the SFTP subsystem.
- *Errors:* The errors of connecting to machines that arise before the session starts apply unchanged. The rest reach the client as text on the session's standard error with exit status 1:
  - **`failed to connect to machine: destination model "<UUID>" is not machine based model`**: *Trigger:* The target is a unit without a container in a Kubernetes model. *Remediation:* Name the container.
  - **`resolving Kubernetes exec information: <cause>`**: *Trigger:* The server cannot find the pod, or the model is not a Kubernetes model (`model "<UUID>" is not a K8s model`). *Remediation:* Check that the unit exists and that its pod runs.
  - **`getting Kubernetes cloud spec: <cause>`**, **`getting Kubernetes executor: <cause>`**, and **`executing command in Kubernetes pod: <cause>`**: *Trigger:* The model's cloud specification or credential is unavailable or invalid, or the Kubernetes API server refused the exec. *Remediation:* Check the model's {ref}`credential <credential>` and the cause.

(ssh-server-relay)=
### Relaying sessions to the SSH server

Relaying sessions to the SSH server lets an intermediary, such as JIMM (a service that sits in front of Juju controllers), carry a user's SSH session to a target over an upgraded HTTPS connection, so that the intermediary authenticates the user. The intermediary calls the API server's endpoint `GET /ssh-relay/<virtual hostname>`, and the session then ends in the target SSH server exactly as it does after the outer connection.

The operation's required access, rules, and errors are:

- *Required access:* A bearer JWT (a signed token) whose claims give the user admin access to the model that the virtual hostname names.
- *Rules:*
  - The intermediary does the authentication, so the key checks of the outer connection do not apply.
  - The endpoint admits at most {ref}`ssh-max-concurrent-connections <controller-config-ssh-max-concurrent-connections>` relays at once, counted apart from the connections of the SSH tunnel server.
- *Errors:* The caller receives each as an HTTP response, except the last, which the user's client receives as text before the SSH banner:
  - **`failed to parse destination hostname`** (400): *Trigger:* A name that is not a virtual hostname. *Remediation:* Use a virtual hostname.
  - **`missing relay JWT`** (401): *Trigger:* No token. *Remediation:* Send the token in the Authorization header.
  - **`forbidden`** (403): *Trigger:* The token does not give admin access to the model. *Remediation:* Issue a token with admin access.
  - **`too many concurrent relay connections`** (503): *Trigger:* The limit is reached. *Remediation:* Retry later, or raise the limit.
  - **`juju ssh relay: cannot reach destination`**: *Trigger:* The server could not resolve the target. *Remediation:* Check the target.

(ssh-server-finding)=
### Finding the SSH server

Finding the SSH server returns the port that the SSH server listens on and the controller's host key. The server listens on every address of each controller node, so a client combines the port with an address of the controller.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Login access <user-access-controller-login>` to the controller, because users who connect through the server are not necessarily controller administrators.
- *Rules:*
  - The port comes from the stored port record. It is set at run time over the controller's control socket, so read it here and not from the controller configuration key {ref}`ssh-server-port <controller-config-ssh-server-port>`.
- *Errors:*
  - **`permission denied`**: *Trigger:* The caller has no access to the controller. *Remediation:* Ask for login access.

(ssh-server-addressing)=
### Addressing SSH server targets

Addressing SSH server targets turns a machine or unit into the names a client needs to reach it: its virtual hostname, and its public, private, or all addresses.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model or {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A machine gives the machine form of the virtual hostname, and a unit gives the unit form, or the container form when the call names a container.
  - The addresses of a unit are those of its machine, and the list of all addresses has each value once, sorted with the most public first.
  - The proxy setting reports the model configuration key {ref}`proxy-ssh <model-config-proxy-ssh>` (default `false`). The SSH server ignores it.
- *Errors:* The client receives each as an error of the call:
  - **`permission denied`**: *Trigger:* The caller lacks the required access. *Remediation:* Ask for it.
  - **`cannot SSH to machine "<id>" in a Kubernetes model; specify a unit name, such as <app>/0`**: *Trigger:* A machine in a Kubernetes model. *Remediation:* Name a unit.
  - **`unsupported entity: "<tag>"`**: *Trigger:* A name that is neither a machine nor a unit. *Remediation:* Name a machine or a unit.
  - **`unit "<name>" not found`**: *Trigger:* A unit that does not exist. *Remediation:* Check the name.

(ssh-server-host-keys)=
### Checking SSH server host keys

Checking SSH server host keys returns the keys that a client should expect, so that it can verify a server instead of trusting it on first use: the controller's host key for the outer connection, the target's virtual host key for the inner connection, and a machine's own host keys.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Login access <user-access-controller-login>` to the controller for the controller's host key, and {ref}`admin access <user-access-model-admin>` to the model or {ref}`superuser access <user-access-controller-superuser>` to the controller for the others.
- *Rules:*
  - Asking for a target's virtual host key creates an `ssh-ed25519` key if the target has none yet.
  - A machine's own host keys are those its agent reported once when it started, from `/etc/ssh/ssh_host_*_key.pub`, and a report replaces the earlier keys. The list is empty until the agent has reported. The host keys of a unit are those of its machine.
  - A client that is not aware of Juju sees the two host keys and has to compare them with these by hand.
- *Errors:* The client receives each as the error of the request:
  - **`could not parse hostname`**: *Trigger:* A name that is not a virtual hostname. *Remediation:* Use the name that {ref}`addressing <ssh-server-addressing>` returns.
  - **`permission denied`**: *Trigger:* The caller lacks the required access. *Remediation:* Ask for it.

## SSH server and model migration

When a {ref}`model <model>` is {ref}`migrated <model-migrating>` to another controller, none of the SSH server's records move with it. The target controller presents its own host key on the outer connection, and it creates a virtual host key for each target when a client first connects, so a client that remembers host keys sees them change.
