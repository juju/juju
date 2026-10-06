---
myst:
  html_meta:
    description: "How Juju's SSH proxy works: architecture, Kubernetes vs machines, authentication, and availability."
---

(juju-ssh)=
# Juju SSH proxy


Juju 4.1 introduced changes in how you access a debug shell on a machine or Kubernetes unit. 

Previously Juju intended for users to connect directly to machines or the Kubernetes API server by placing users' SSH keys directly on machines or issuing users with a scoped Kubernetes access token.

Juju 4.1 includes an SSH server in the controller that, by default, runs on port 17022. Clients can use the existing `juju ssh` and `juju scp` commands to connect and have their connection proxied to the final destination.

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy overview
:caption: Juju controller terminating an SSH connection and proxying the traffic to the final destination.
:alt: A client connects via SSH to the controller, which proxies the session to a machine or unit.
```

The new SSH server terminates the SSH connection, allowing Juju to inspect any commands before they reach the final destination. Below we explain the architecture and what to expect when you use `juju ssh` or another SSH client.


```{ibnote}
See more: {ref}`access-a-machine-via-ssh`, {ref}`command-juju-ssh`, {ref}`command-juju-scp`
```

(juju-ssh-proxy-architecture)=
## SSH proxy architecture

The goal of proxying SSH connections through the controller is two-fold:
1. Allow users to debug machines/units without direct network connectivity.
2. Avoid storing user credentials across machines without a centralised way to see who has access and when they login.

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy connections
:caption: The controller terminates two SSH connections: an outer tunnel with the controller's fixed host key, and an inner session with a virtual host key.
:alt: The client connects to the SSH tunnel server inside the Juju controller. A downward arrow leads to the target SSH server below it, which proxies the session to a machine or unit.
```

In the above view, we can better see the operations occurring inside the Juju SSH server where the controller terminates two SSH connections.

The initial SSH connection is terminated by Juju's SSH server running on port 17022 and exists to authenticate the user and receive details for the user's intended target, using SSH direct TCP forwarding. The initial connection then serves as a tunnel for another SSH connection that is also terminated at the Juju controller. With this setup, Juju can serve a unique host key for each destination and still inspect the SSH traffic before forwarding it.

When a client runs `juju ssh` it uses Juju's API server to verify the expected host keys that will be exchanged. This step is not possible when using another SSH client and will require you to manually verify the presented host keys.

The first SSH connection exchanges the server's fixed host key and the second connection exchanges a "virtual" host key based on the target destination.

The Juju controller will establish a connection to the target machine or the Kubernetes API server and proxy requests/responses.

(juju-ssh-kubernetes-vs-machines)=
### Kubernetes vs Machine targets

Establishing a connection to Kubernetes units and machines varies but requires no changes to users' network setup.

#### Kubernetes 

For Kubernetes units, the Juju controller establishes a direct connection to the Kubernetes API server. Access to the API server is already required to use the Kubernetes cluster as a Juju cloud, so no additional connectivity is required.

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy Kubernetes
:caption: The controller translates the SSH session into a stream through the Kubernetes API server.
:alt: The controller opens an exec stream to the Kubernetes API server, which runs commands in the target container inside a unit pod.
```

#### Machines

For machines, the Juju controller uses a "reverse-tunnel" approach where the machine initiates a connection that the controller uses to establish an SSH session back to the machine.

```{ggarch}
:file: ../juju.ggarch
:view: SSH proxy machine reverse tunnel
:caption: The controller establishes an SSH connection to a machine over a tunnel initiated by the machine.
:alt: Two columns show the Juju controller and the machine. The controller's target SSH server stores a request in the model database below it. The machine agent watches for requests via the API and opens a reverse tunnel to the controller API server. Across the top, the target SSH server connects to the machine SSH server over that tunnel.
```

The benefit of this approach is that the machine does not need to expose its SSH server and the controller does not need new security group/firewall rules for machine agents.

(juju-ssh-proxy-authentication)=
## Authentication and authorization

Authentication continues to use user public keys. Upload your public key to Juju, specifically to the model where you want SSH to work. You will authenticate as your Juju user and the session will use the `ubuntu` user on the target destination.

Permission to SSH is now enforced in Juju's SSH server, requiring admin access to the model.

```{ibnote}
See more: {ref}`manage-ssh-keys`, {ref}`command-juju-add-ssh-key`
```

(juju-ssh-proxy-availability)=
## Availability and limitations

SSH access to machines/units now requires the Juju controller to be running and accessible. For machines, it also requires that the Juju agent service is running. If the agent service cannot start, `juju ssh` will not be able to connect to the machine and alternatives should be explored:
1. Use provider specific access e.g. use AWS EC2, use `lxc shell`, or other provider specific tooling to connect to the machine.
2. Use cloud-init config to add an administrator public key to machines that can be used to SSH directly in special cases.

```{ibnote}
See more: {ref}`model-config-cloudinit-userdata`
```
