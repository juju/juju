---
myst:
  html_meta:
    description: "How Juju's SSH proxy works: architecture, Kubernetes vs machines, authentication, and availability."
---

(explanation-juju-ssh)=
# Juju SSH proxy


Introduced in Juju 4.1, Juju has changed how you access a debug shell in a machine or Kubernetes unit. 

Previously Juju intended for users to connect directly to machines or the Kubernetes API server by placing user's SSH keys directly on machines or issuing users with a scoped Kubernetes access token.

Juju 4.1 includes an SSH server in the controller that runs on port 17022, by default. Clients can use the existing `juju ssh` and `juju scp` commands to connect and have their connection proxied to the final destiation.

<!--
Simple diagram here showing client -> controller (decrypt/encrypt) -> machine
-->

The new SSH server terminates the SSH connection, allowing Juju to inspect any commands before they reach the final destination. Below we explain the architecture and what to expect when you use `juju ssh` or another SSH client.


<!--
Add see-more link to the guide on how to use `juju ssh` or OpenSSH client.
-->

```{ibnote}
See also: {ref}`juju-architecture`, {ref}`juju-security`
```

(juju-ssh-proxy-architecture)=
## SSH proxy architecture

The goal of proxying SSH connections through the controller is two-fold:
1. Allow users to debug machines/units without direct network connectivity.
2. Avoid storing user credentials across machines without a centralised way to see who has access and when they login.

<!--
Expanded view, showing the two SSH servers in Juju
-->

In the above expanded view, we illustrate how the Juju SSH server works and how the client verifies the authenticity of the SSH server and the final destination.

The controller serves an SSH server that only accepts connection requests to another destination, in the same way SSH remote port-forwarding works. This initial connection serves as a tunnel for another SSH connection that is also terminated at the Juju controller. With this setup, Juju is able to inspect the SSH traffic before forwarding it.

When a client runs `juju ssh` we use Juju's API server to verify the expected host keys exchanged, based on the target we are connecting to. This step is not possible when using another SSH client and will require you to manually verify the presented host keys.

The first connection exchanges the server's fixed host key and the second connection exchanges a host key based on the target destination.

The Juju controller will establish a connection to the target machine or the Kubernetes API server and proxy requests/responses.

### Kubernetes vs Machine targets

Establishing a connection to Kubernetes units and machines varies but should require no changes to user's network setup.

For Kubernetes units, the Juju controller establishes a direct connection to the Kubernetes API server. Access to the API server is already required to use the Kubernetes cluster as a Juju cloud, so no new connectivity is required.

<!--
Simple example of controller to K8s API server to pod.
-->


For machines, the Juju controller uses a "reverse-tunnel" approach.

<!--
illustration of reverse-tunnel with controller going to machine and a DB block that the machine watches.
-->

The diagram above shows that machines establish a connection to the Juju controller's API server (by default port 17070) through which the controller can establish an SSH connection back to the machine. This means that the machine does not need to expose a running SSH server and no new security group/firewall rules need to be created.

(juju-ssh-proxy-authentication)=
## Authentication and authorization

Authentication continues to use user public keys. Upload your public key to Juju, specifically to the model where you want SSH to work. You will authenticate as your Juju user and the session will use the `ubuntu` user on the target destination.

Permission to SSH is now enforced in Juju's SSH server, requiring admin access to the model.

<!--
Add see more links to juju add-ssh-keys command
-->

(juju-ssh-proxy-availability)=
## Availability and limitations

SSH access to a machines/units now requires the Juju controller to be running and accessible. For machines, it also requires that the Juju agent service is running. If the agent service cannot start, `juju ssh` will not be able to connect to the machine and some alternatives below should be explored:
1. Use provider specific access - e.g. use AWS EC2, use `lxc shell`, or other provider specific tooling to connect to the machine.
2. Use cloud-init config to add an administrator public key to machines that can be used to SSH directly in special cases.

<!--
Add see more link to cloud-init model config key.
-->
