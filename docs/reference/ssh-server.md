---
myst:
  html_meta:
    description: "Juju's SSH server: host identity, port discovery, connection limit, and access to machine and Kubernetes targets."
---

(juju-ssh)=
(juju-ssh-proxy)=
(ssh-server)=
# SSH server

```{ibnote}
See also:

- {ref}`Juju CLI and OpenSSH | Access a machine via SSH <access-a-machine-via-ssh>`
- {ref}`Juju CLI | Add an SSH key <add-an-ssh-key>`
- [Terraform Provider for Juju | Add an SSH key](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-ssh-keys/#add-an-ssh-key)
```

In Juju, the SSH server is a jump server that runs on each {ref}`controller <controller>` node. It lets clients reach machines and units over SSH without a direct network connection to the target.

(juju-ssh-proxy-architecture)=
(ssh-proxy-architecture)=
(controller-ssh-server-connections)=
## SSH server connections

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy connections
:caption: The client opens one network connection to the jump server. The jump server forwards the client's inner SSH stream through the outer tunnel to the target SSH server on the same controller. Both SSH connections end at the controller, so it can read the session contents before forwarding them to the machine or unit.
:alt: The client connects to the jump server, which presents the controller host key. Inside the controller, the jump server forwards the inner SSH stream to the target SSH server, which presents the destination's virtual host key and proxies the session to the machine or unit.
```

When a client connects through the jump server, the first SSH connection authenticates the user and carries a request for a destination. The jump server forwards a second SSH connection through that tunnel. The controller terminates both connections: the first uses the jump server's host key, and the second uses the {ref}`destination's virtual host key <controller-ssh-server-targets>`. The target SSH server then proxies the session to the machine or unit.

(juju-ssh-kubernetes-vs-machines)=
(kubernetes-vs-machine-targets)=
### SSH server paths to targets

Both SSH connections end at the controller. How the controller reaches the target from there depends on where the target runs.

``````{tabs}

`````{tab} Kubernetes

(kubernetes)=

````{container}

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy Kubernetes
:caption: Kubernetes exec carries the session's input and output between the controller and the target container. The controller does not open an SSH connection to the container.
:alt: Sequence showing the controller opening an exec stream through the Kubernetes API server to the target container in the unit pod. Session output flows back to the controller through the API server.
```

````

For a Kubernetes unit, the controller opens an exec stream through the Kubernetes API server to run the session in the target container in the unit's pod.

`````

`````{tab} Machines

(machines)=

````{container}

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy machine reverse tunnel
:caption: The controller records a tunnel request in the model database. The machine agent learns of the request through its API watcher, reads it, and opens the tunnel back to the controller. The agent connects the tunnel to the SSH service on the same machine.
:alt: Sequence with the controller, machine agent, and SSH server. The latter two run on the same target machine. The controller records a request. The agent reads it, opens a reverse tunnel to the controller, and connects it to the local SSH server. The controller uses the tunnel for SSH.
```

````

For a machine or machine-backed unit, the machine agent opens a **reverse tunnel** to the controller API server. The controller uses that tunnel to log in to the machine's SSH service as `ubuntu`, regardless of the Juju username. The machine does not need to expose its SSH port to the client.

`````

``````

(controller-ssh-server-endpoint)=
## SSH server endpoint and host key

A user with {ref}`controller login access <user-access-controller-login>` can read the SSH server's public host key and port from the controller API. The host key identifies the controller's SSH server; controller nodes share this key. If no key is supplied at {ref}`bootstrap <controller-bootstrap>`, Juju generates an Ed25519 key. A supplied key must be RSA, ECDSA P-256, or Ed25519.

The API returns the stored port, or `17022` if no port is stored. Use this value instead of the {ref}`ssh-server-port <controller-config-ssh-server-port>` bootstrap setting to choose where to connect. The API read does not confirm that a node is listening on that port.

After {ref}`model migration <model-migration-contract>`, the first SSH hop uses the destination controller's host key. Verify it against the key reported by the destination controller's API. For fingerprint verification with OpenSSH, see {ref}`access-a-machine-via-ssh`.

If the controller API refuses a port or host-key lookup, check the message:

- *Error message:* `permission denied`. *Trigger:* The caller lacks controller `login` access. *What to do:* Use an account with controller `login` access.
- *Error message:* `getting controller SSH server host public key: controller SSH host key not found`. *Trigger:* The controller has no stored SSH host key. *What to do:* Contact the controller operator. Do not connect through a jump server whose key you cannot verify.

(controller-ssh-server-connection-limit)=
## SSH server connection limit

Each controller node limits concurrent connections to its jump server according to {ref}`ssh-max-concurrent-connections <controller-config-ssh-max-concurrent-connections>`. A connection above the limit is closed before authentication. A change to the limit or the stored port restarts the jump server and can interrupt active sessions.

When the SSH server refuses a connection because it is at capacity:

- *Error message:* `too many connections.` *Trigger:* The server has reached its connection limit. *What to do:* Retry when a connection has closed.

(juju-ssh-proxy-authentication)=
(authentication-and-authorization)=
(controller-ssh-server-targets)=
## SSH server targets and permissions

The jump server authenticates a local Juju user by username and registered public key. It then checks access to the destination model: the user needs {ref}`admin access on that model <user-access-model-admin>` or {ref}`controller superuser access <user-access-controller-superuser>`. The key used to authenticate must also be registered for that user in the destination model, even for a controller superuser.

The jump server routes by **virtual hostname**, a Juju name that identifies a target and its model, rather than by the target's IP address. For a Kubernetes session, this name must also identify the target container. A user with either grant can read the virtual hostname from the model API. For an OpenSSH example, see {ref}`access-a-machine-via-ssh`.

The second SSH connection uses a virtual host key for the destination, separate from the jump server's host key. A machine-backed unit uses its backing machine's virtual host key. A Kubernetes unit uses its own virtual host key. A user with the required model or controller grant can read the destination's public virtual host key from the model API.

| Target | Sessions and commands | SFTP | Local port forwarding |
| --- | --- | --- | --- |
| Top-level machine or machine-backed unit | Supported | Supported | Supported, from the destination machine |
| Kubernetes unit | Supported through the Kubernetes API | Not supported | Not supported |

The jump server does not accept a nested machine as a direct machine target.

If the SSH server refuses access or cannot handle a request for the target, check the message:

- *Error message:* `unauthorized` (SSH channel refused). *Trigger:* The authenticated user has neither admin access to the destination model nor controller superuser access. *What to do:* Use an account with either grant.
- *Error message:* `failed to authorize access to destination: public key ... used to authenticate is not associated with model ...` (SSH channel refused). *Trigger:* The user has the required grant, but the key accepted by the jump server is not registered for that user in the destination model. *What to do:* Register the key there or use one already registered there.
- *Error message:* `cannot SSH directly to nested machine "<machine>", connect to parent machine "<parent>" instead`. *Trigger:* A client requests the virtual host key for a direct nested-machine target, or opens a jump channel to that target. The SSH refusal prefixes the message with `failed to resolve destination:`. *What to do:* Connect to the parent machine instead.
- *Error message:* `failed to connect to machine: waiting for tunnel: context deadline exceeded`. *Trigger:* The machine's reverse tunnel did not reach the controller in time. *What to do:* Check that the machine agent is running and can reach the controller API.
- *Error message:* `failed to connect to machine: waiting for machine SSH host keys: context deadline exceeded`. *Trigger:* The tunnel opened, but the controller did not get the machine's SSH host keys in time. *What to do:* Check that the machine's SSH service has host keys and that the machine agent can report them.
- *Error message:* `SFTP is not supported for Kubernetes targets` (exit status: `1`). *Trigger:* The client requests the SFTP subsystem for a Kubernetes unit. *What to do:* Use an access method supported by that target.
- *Error message:* `local forwarding is not supported for Kubernetes targets` (SSH channel refused). *Trigger:* The client requests local forwarding to a Kubernetes unit. *What to do:* Use an access method supported by that target.

(juju-ssh-proxy-availability)=
(availability-and-limitations)=
## SSH server availability

The client must be able to reach a running controller SSH server. For machine targets, the machine agent must also be running and connected to the controller so it can open the reverse tunnel. For fallback options, see {ref}`access a machine when the SSH proxy is unavailable <access-a-machine-without-the-ssh-proxy>`.
