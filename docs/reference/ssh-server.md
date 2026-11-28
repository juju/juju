---
myst:
  html_meta:
    description: "The Juju controller's SSH server: connections, access, host keys, machine and Kubernetes destinations, and availability."
---

(juju-ssh-proxy)=
(juju-ssh)=
(ssh-server)=
# SSH server

```{ibnote}
See also: {ref}`Juju CLI | Access a machine via SSH <access-a-machine-with-the-juju-cli>`, {ref}`OpenSSH | Access a machine via SSH <verify-ssh-host-fingerprints>`
```

```{versionadded} 4.1.0
```

```{ggarch}
:file: ../juju.ggarch
:view: Native jump connection
:caption: Both SSH connections end on the same controller node. The jump server accepts the client's first connection. Through it the client opens a second connection to the virtual host on that node. The command runs on the chosen machine or in the chosen container.
:alt: Inside the controller's SSH server, the jump server presents the controller host key and forwards the client's inner SSH stream to the virtual host, which presents the target host key. Only one tunnel crosses from the client to the controller.
```

In Juju, the **SSH server** is the part of the {ref}`controller <controller>` that lets a client reach a {ref}`machine <machine>` or {ref}`unit <unit>` without a network route from the client to the workload. By default, when you run {ref}`juju ssh <command-juju-ssh>`, your client {ref}`connects to this server first <ssh-server-connections>`. To connect this way, you need a {ref}`public key registered for your Juju user in the destination model <ssh-key-authorization>` and {ref}`either admin access to that model or superuser access to the controller <ssh-server-access>`. The {ref}`target <ssh-server-targets>` determines where the command runs and whether the session supports SFTP or local port forwarding.

(ssh-proxy-architecture)=
(juju-ssh-proxy-architecture)=
(ssh-server-connections)=
## SSH server connections

```{ibnote}
See also: {ref}`Juju CLI | Use SSH and file copy when upgrading from 3.6 to 4.1 <juju-ssh-and-juju-scp-go-through-the-controller-by-default>`
```

By default, {ref}`juju ssh <command-juju-ssh>` and {ref}`juju scp <command-juju-scp>` connect first to the controller's **jump server**. It accepts client logins. An external service that authenticates to the controller API with a token can instead send the SSH connection through that API. Both paths reach the **virtual host** for the chosen target. This endpoint runs on the controller, not on the target.

At the jump server, the client authenticates as a Juju user and requests a target. Through that first connection, the client opens a second SSH connection to the virtual host. The controller terminates that connection and handles the session's commands and data rather than passing an opaque stream to the workload. Neither connection opens a shell on the controller itself.

When an external service uses the API, the controller checks that the service's token grants {ref}`admin access to the destination model <user-access-model-admin>` before forwarding the SSH connection to the virtual host. This **API relay** does not use the jump server or its public-key login. On either path, the {ref}`target <ssh-server-targets>` determines where the command runs.

(availability-and-limitations)=
(juju-ssh-proxy-availability)=
(ssh-server-availability)=
(ssh-server-listener)=
## SSH server listener

To connect to the controller's SSH jump server, a client must reach a controller node on its current SSH port (TCP 17022 by default). A user with at least {ref}`controller login access <user-access-controller-login>` can read that port from the controller API (`Controller.SSHServerPort`) without having access to a target. Use the reported port, not the original {ref}`ssh-server-port <controller-config-ssh-server-port>` bootstrap setting, to choose where to connect.

Reaching the listener does not establish that a session can start. For a machine target, the controller asks the machine agent to open a connection back to it. Check that {ref}`the agent can reach the controller node that made the request <ssh-server-machine-sessions>`. For a pod target, check the {ref}`Kubernetes API and container readiness <ssh-server-kubernetes-sessions>`. If the machine agent cannot run, use {ref}`agent-independent recovery <recover-access-to-a-machine>`.

When the charm that manages the controller supplies a new SSH port in the range 1–65535, the controller records it before restarting the listener. Controller-model status can display the default port if it cannot read the current value. Neither a successful update nor a status listing proves that the port is reachable. Changing {ref}`ssh-max-concurrent-connections <controller-config-ssh-max-concurrent-connections>` also restarts the listener. SSH connections {ref}`forwarded through the controller API <ssh-server-connections>` count against a separate limit of the same size; they do not use up slots on this listener.

**Errors:** The listener can refuse a new connection before login:

- *Error message:* `too many connections.` *Trigger:* The jump listener on that node has reached its connection limit. *What to do:* Retry after another connection closes.

(kubernetes-vs-machine-targets)=
(juju-ssh-kubernetes-vs-machines)=
(ssh-server-destinations)=
(ssh-server-targets)=
## SSH server targets

The controller routes a session by its *virtual hostname*, not by the workload's IP address. The name identifies the machine, unit or container within a unit, includes the model UUID, and ends in `juju.local`. A {ref}`destination-model admin <user-access-model-admin>` or {ref}`controller superuser <user-access-controller-superuser>` can request this name from the model API. Obtaining a name does not mean the target is ready.

| Target | Where a shell or command runs | SFTP | Local TCP forwarding |
|---|---|---|---|
| Top-level machine, or a unit assigned to a machine | On the {ref}`machine <ssh-server-machine-sessions>` | {ref}`Supported <ssh-server-file-transfer>` | {ref}`Supported <ssh-server-port-forwarding>` |
| Container in a unit in a Kubernetes model | In that {ref}`container in the unit's pod <ssh-server-kubernetes-sessions>` | {ref}`Not supported <ssh-server-file-transfer>` | {ref}`Not supported <ssh-server-port-forwarding>` |

For Kubernetes exec, a client must request a virtual hostname that includes the container. A bare unit virtual hostname selects the machine handler instead. With {ref}`juju ssh <command-juju-ssh>`, give the unit name: the CLI supplies the container name unless you choose another.

**Errors:** When requesting a target for a connection through the controller's SSH server, a client can encounter these refusals:

- *Error message:* ends with `cannot SSH directly to nested machine "<machine>", connect to parent machine "<parent>" instead`. *Trigger:* The target is a nested machine ID. *What to do:* Connect to the named parent.
- *Error message:* `cannot SSH to machine "<machine>" in a Kubernetes model; specify a unit name, such as <app>/0`. *Trigger:* The target is a machine number in a Kubernetes model. *What to do:* Use a unit name with the Juju CLI. If requesting the virtual hostname yourself, specify both the unit and its container.

(ssh-server-host-keys)=
## SSH server host keys

When a client connects through the controller's SSH jump server, two endpoints present host keys: the jump server where the client logs in, and the endpoint on the controller for the chosen target (the **virtual host**). The jump server presents the **controller's host key**, shared by that controller's nodes. A user with at least {ref}`controller login access <user-access-controller-login>` can read its public key from the controller API. After a model moves to another controller, verify this first hop against the destination controller.

The virtual host presents a **virtual host key** for the target. A machine and the units it hosts share the machine's virtual key, which is distinct from the key presented by the machine's own SSH daemon. On Kubernetes the virtual key belongs to the unit, so two containers in the same unit present the same key. A {ref}`destination-model admin <user-access-model-admin>` or {ref}`controller superuser <user-access-controller-superuser>` can read the virtual host's public key from the model API.

For the Juju client's default checks, see {ref}`juju ssh <command-juju-ssh>`. When using OpenSSH yourself, {ref}`compare each fingerprint with the one for that hop <verify-ssh-host-fingerprints>` before accepting it.

**Errors:**

- *Error message:* The controller API error ends with `controller SSH host key not found`. *Trigger:* The API could not find the jump server's public host key. *What to do:* Ask the controller operator to investigate before trusting the endpoint. Replacing your user key will not establish the server's identity.

(authentication-and-authorization)=
(juju-ssh-proxy-authentication)=
(ssh-server-access)=
## SSH server access

```{ibnote}
See also: {ref}`Juju CLI | Use an SSH key <use-an-ssh-key>`, {ref}`Juju CLI | Add an SSH key <add-an-ssh-key>`, [Terraform Provider for Juju | Add an SSH key](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-ssh-keys/#add-an-ssh-key)
```

The jump server authenticates a Juju user before it authorizes a target. It first checks the public key offered by the client against the keys registered for that username. Once the client names a target, the server checks the user's access to the destination model and then checks the **same key** in that model. On this route the user's key authenticates to Juju, not to the machine's SSH daemon.

The Juju CLI first asks the model API for the target's virtual hostname. If the API denies that lookup, check the account's model or controller grant; the CLI has not yet offered a key to the jump server.

**Who can do this:** A {ref}`destination-model admin <user-access-model-admin>` or {ref}`controller superuser <user-access-controller-superuser>` using an {ref}`SSH key authorized for that user in the destination model <ssh-key-authorization>`. Model `write` alone is insufficient. Controller `superuser` does not waive the key requirement.

**Errors:** The jump server can refuse login or access to a target:

- *Error message:* The SSH client reports a public-key authentication failure before opening a target. *Trigger:* The jump server did not accept the offered key for the supplied Juju username. *What to do:* Check the account and {ref}`select its registered key <use-an-ssh-key>`.
- *Error message:* SSH channel refused: `unauthorized`. *Trigger:* The authenticated user lacks the grant for the destination. *What to do:* Use an account with destination-model `admin` or controller `superuser` access, or ask a model administrator to grant model `admin`.
- *Error message:* The refusal contains `public key "<key comment>" used to authenticate is not associated with model "<model UUID>"`. *Trigger:* The key accepted at login is not registered for this user in the destination model. *What to do:* {ref}`Register the corresponding public key <add-an-ssh-key>` in that model, not the comment printed in the error, or {ref}`select a key already registered there <use-an-ssh-key>`.

(machines)=
(ssh-server-machine-destinations)=
(ssh-server-machine-sessions)=
## SSH server machine sessions

```{ibnote}
See also: {ref}`Juju CLI | Access a machine via SSH <access-a-machine-with-the-juju-cli>`, {ref}`OpenSSH | Access a machine via SSH <verify-ssh-host-fingerprints>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Machine SSH reverse tunnel
:caption: The machine agent opens a tunnel back to the controller node that requested it, then connects the tunnel to the SSH daemon on its own machine. The controller uses the returned stream for SSH; it does not dial the machine from outside.
:alt: In order, the controller requests a tunnel, the machine agent connects back to that controller and connects to its local SSH daemon, and the controller logs in to the daemon over the tunnel. The agent and daemon are on the same machine.
```

When a controller node requests a tunnel, the machine agent opens a connection back to that node (a reverse tunnel). The controller uses this connection to log in to the machine's SSH daemon as `ubuntu`, regardless of the user's Juju account. The session runs the requested command, or opens a shell if none was requested, with or without a pseudo-terminal. When the target is a unit, the controller must be able to resolve it to its backing machine. A missing, dead or unassigned unit cannot start a machine session.

The machine agent must be running and able to reach the originating controller's API. If the agent cannot start, use a {ref}`recovery route independent of the agent <recover-access-to-a-machine>`.

**Errors:** The controller can time out waiting for the agent to connect back or, once the tunnel is open, waiting for the machine's SSH host keys:

- *Error message:* ends with `waiting for tunnel: context deadline exceeded`. *Trigger:* The controller did not receive the agent's callback in time. *What to do:* Check the machine agent and its network route to the originating controller's API.
- *Error message:* ends with `waiting for machine SSH host keys: context deadline exceeded`. *Trigger:* The tunnel opened, but the controller did not receive the machine daemon's host keys in time. *What to do:* Check that the machine's SSH daemon has host keys and that the agent can report them. The {ref}`host key the controller presents to your client for this target <ssh-server-host-keys>` is a different key; it cannot identify the machine's daemon.

(kubernetes)=
(ssh-server-kubernetes-destinations)=
(ssh-server-kubernetes-sessions)=
## SSH server Kubernetes sessions

```{ibnote}
See also: {ref}`Juju CLI | Open a shell in a workload container <debug-a-k8s-charm-with-a-workload>`
```

For a Kubernetes unit, the controller looks up the unit's pod and asks the Kubernetes API to run the command in the selected container. The unit must exist, not be dead, and have an assigned pod. The controller must be able to reach the Kubernetes API. The container must be running and have `sh` to execute the command; it needs no SSH daemon.

If the client sends no command, the server requests `/bin/sh`. A client can request a different shell; see {ref}`juju ssh <command-juju-ssh>` for the Juju client's default. When a command exits, the server passes its exit status back to the client.

**Errors:** The session can fail before the command runs:

- *Error message:* contains `unit "<unit>" has no k8s pod`. *Trigger:* The unit has no assigned pod. *What to do:* Check the unit's status and retry once a pod is assigned.
- *Error message:* contains `container "<name>" not found`. *Trigger:* The unit's pod has no container with that name. *What to do:* Select a container that exists in that pod.
- *Error message:* contains `container "<name>" not running`. *Trigger:* The selected container exists but is not running. *What to do:* Check its status and retry when it is running.

(ssh-server-file-transfer)=
## SSH server file transfer

```{ibnote}
See also: {ref}`Juju CLI | Copy files securely between machines <copy-files-between-machines>`
```

For a machine or a machine-backed unit, the SSH server forwards SFTP requests to the SFTP subsystem on the machine. It does not support SFTP for Kubernetes targets.

**Errors:** The server refuses an SFTP request for a Kubernetes target:

- *Error message:* `SFTP is not supported for Kubernetes targets` (exit status `1`). *Trigger:* The client requests the SFTP subsystem for a Kubernetes target. *What to do:* Use {ref}`juju scp <command-juju-scp>` to copy between your computer and a container instead.

(ssh-server-port-forwarding)=
## SSH server port forwarding

For a machine or a machine-backed unit, local TCP forwarding carries a connection from the client through the session. The **machine** opens the connection to the requested host and port, so the destination must be reachable from that machine.

**Errors:** The controller's SSH server refuses local forwarding for Kubernetes targets:

- *Error message:* `local forwarding is not supported for Kubernetes targets` (SSH channel refused). *Trigger:* The client requests local TCP forwarding for a Kubernetes target. *What to do:* Use a machine target from which the destination is reachable. Choosing another container does not enable forwarding.
