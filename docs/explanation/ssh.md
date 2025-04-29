(juju-ssh)=
# Juju SSH

Juju 3.6 introduces a new way to SSH into machines and Kubernetes units.

Instead of connecting directly to machines, SSH connections are now routed through the Juju controller (or via JIMM if used). This centralization improves auditability and security, while keeping the user experience largely unchanged. 

Users can continue to run `juju ssh` or use standard SSH clients.

## Overview

The Juju controller now runs an SSH server capable of terminating SSH connections
and then forwards the traffic to the target machine or K8s unit.

When a client attempts to SSH to a target unit/machine, the command will resemble the following,
using OpenSSH syntax.

```
ssh -J <controller-address>:17022 ubuntu@<unit-number.app.modelUUID>
```

The ` -J` flag performs an "Jump" connection.
This is a process that may be familiar to those who connect to machines through a jump/bastion host,
effectively establishing two SSH connections to reach the target. Supported in some fashion by many SSH clients.

The Juju controller terminates both connections, allowing it to log the traffic for audit purposes
before forwarding it.

## Security

The main security enhancement of the SSH Proxy is the removal of direct SSH access to machines.

Instead, connections are proxied through the Juju controller, which:

- Limits the network surface exposed to the public.
- Avoids multiple copies of a user's keys on each machine they want to access.
- Allows the Juju controller to present stronger access controls beyond what SSH offers.
- Enables better management of SSH session policies.
- In the future, will allow for audit logging, giving admins visibility into changes.

Additional security features built-in include:

- Ephemeral key-pairs are generated for each SSH session from the controller to the unit.
- Public keys are pushed temporarily to units, and removed after the session ends.

## Host keys
Host keys are used in SSH to authenticate servers to clients and establish trust between them.

In the context of the SSH Proxy feature, Juju is responsible for managing and presenting the appropriate
host keys when terminating SSH connections.

There are two host key types in this system:

### Jump server sost key
The jump server is the first point of contact for the SSH client.

- The jump server presents a **fixed, persistent host key**.
- In a high availability (HA) deployment, all controller machines share the same host key so that the controller appears as a single trusted entity.
- Host keys are securely stored alongside other sensitive controller credentials (e.g., TLS certs).
- Users can provide a custom host key at bootstrap or rely on the default, auto-generated one.

### Unit host keys

When Juju terminates the SSH connection (at the controller level) on behalf of a unit or machine, it presents a "virtual" host key
to the SSH client.

These virtual host keys are generated for each machine or each K8s unit, providing a predictable experience. For example,
- Multiple units on the same machine will present with the same host key.
- Multiple containers in a Kubernetes pod will present with the same host key.

### Host key verification

Trust in host keys must be established:

- **Using `juju ssh`**:  
  Host keys are retrieved securely over HTTPS/WebSocket, and added to the SSH client’s `known_hosts` file. Trust is managed automatically where possible.

- **Using raw `ssh`**:  
  Users must manually verify and trust host keys via out-of-band methods as is commonly done with plain SSH (e.g., administrator emails, secure config distribution).

## Auditing

The SSH Proxy lays the groundwork for robust auditing features.

While full session logging is not yet available, the architecture supports future enhancements like:

- Capturing metadata: session start/end times, source IPs, users, and target units.
- Session monitoring and inspection.
- Differentiation between machine and Kubernetes unit access for fine-grained logging.

Routing connections through the controller ensures a single point where audit hooks and middleware can be introduced without affecting user workflows.

## User authentication & authorization

**Authentication methods:**

- Users always authenticate with public/private SSH key-pairs (uploaded via `juju add-ssh-key`).

**Authorization**

- Users must be authorized to access a machine/unit by having `admin` level model access to the model
containing the machine/unit (enabled via `juju grant`).

## JAAS support

Read more about JAAS (Juju for the enterprise) at the [JAAS docs](https://canonical-jaas-documentation.readthedocs-hosted.com/en/latest/).

In JAAS environments, JIMM acts as an SSH jump server that transparently tunnels SSH connections to the Juju controller.

Clients connect to the JAAS SSH server in exactly the same way as a regular Juju controller.
Behind the scenes, JAAS communicates the user's request to the controller and establishes a tunnel to the Juju controller's
SSH server. The client is then able to establish a session to the Juju controller through JAAS.

This design ensures end-to-end security, ensuring that JIMM is unable to descrypt or inspect session data.
