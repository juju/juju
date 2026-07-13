---
myst:
  html_meta:
    description: "Create an Unmanaged cloud in Juju from existing machines, including SSH requirements, bare metal setup, and cloud configuration without MAAS."
---

(cloud-unmanaged)=
# Unmanaged

In Juju, the Unmanaged cloud is a {ref}`machine cloud <machine-cloud>` that adopts existing machines via SSH, and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

(unmanaged-limitations)=
## Limitations

- **No infrastructure creation**: The Unmanaged cloud cannot create new machines. Existing Ubuntu/Debian machines must be adopted via `juju add-machine ssh:[user@]<host>`.
- **No start/stop operations**: Starting and stopping machines is managed outside Juju.
- **No auto-scaling**: Each machine must be added explicitly.
- **Destroy is limited**: Controller teardown is SSH-based; model teardown does not remove infrastructure.
- **No firewall control**: All firewall ops (`open-ports`, `close-ports`) are no-ops.
- **Spaces supported via progressive discovery**: `SupportsSpaces` is enabled, but `reload-spaces` does not discover subnets. Instead, each time a machine is provisioned via `juju add-machine`, its discovered network devices update Juju's known subnet list. New subnets land in the `alpha` space. Define spaces from these subnets after provisioning.
- **No cloud-specific storage**: Users must pre-configure storage or manage it outside Juju.

The Unmanaged cloud suits situations where you have machines of any nature at your disposal and want to bring them under Juju management without additional infrastructure. If your machines are solely bare metal, you might opt for a {ref}`MAAS cloud <cloud-maas>` instead -- though that requires [IPMI hardware](https://docs.maas.io/en/nodes-power-types) and a MAAS installation.

(unmanaged-requirements)=
## Requirements

The target machines must already exist and be reachable over SSH:

- Running Ubuntu or Debian.
- Accessible via SSH from the Juju client using public key authentication.
- SSH user must have sudo rights (passwordless sudo preferred).
- Machines must be able to ping one another.

(unmanaged-concepts)=
## Concepts

The following table shows how Unmanaged cloud behavior maps to Juju concepts:

| Unmanaged cloud concept | Juju |
| - | - |
| Any Ubuntu/Debian machine reachable via SSH | {ref}`machine <machine>` |
| Process on a machine | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |

(unmanaged-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # User-defined name
    type: manual
    auth-types:
      - <auth-type>                # See Authentication types below
    endpoint: <user>@<host-or-ip>  # SSH connection for controller bootstrap
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


A Unmanaged machine is any Ubuntu/Debian system reachable via SSH with sudo privileges. Machines are not created by Juju -- they must be provisioned externally and brought under Juju management via SSH.

(unmanaged-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  <your-unmanaged-cloud>         # Cloud name as defined above
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # empty (no attributes required)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
```


The Unmanaged cloud uses the `empty` authentication type: no credential attributes are required or stored. Juju creates an empty credential automatically when the cloud is added. Running `juju add-credential` is not needed.

Access to the machines is controlled entirely by SSH key configuration on the target hosts, not by Juju credentials.

(unmanaged-credential-authentication-types)=
### Authentication types

`empty` (the only type). No attributes.

(unmanaged-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

When adding the cloud, enter the SSH connection information for the machine where a Juju controller will be bootstrapped, e.g., `username@<hostname or IP>` (where we assume `username` is `ubuntu`) or `<hostname or IP>`.

Alternatively, you can pass the SSH target inline to `juju bootstrap`, skipping the `add-cloud` step entirely:

```text
juju bootstrap unmanaged/ubuntu@<IP> <controller-name>
```

(unmanaged-controller-bootstrap-behavior)=
### Bootstrap behavior

Bootstrap initializes the remote machine by creating an `ubuntu` user with passwordless sudo, detecting hardware characteristics via SSH, and configuring the Juju agent through cloud-init-style bash scripts executed remotely.

**SSH operations during bootstrap:**

1. **Ubuntu user setup**: SSH to `ubuntu@host` (or provided user). If `ubuntu` user doesn't exist, creates it with passwordless sudo via `sudo /bin/bash` script.
2. **Provisioning check**: Verifies no jujud service already exists (fails if found).
3. **Hardware detection**: SSH script reads `/etc/os-release`, `uname`, `/proc/meminfo`, `/proc/cpuinfo` to detect OS, architecture, memory, CPU cores.
4. **Machine configuration**: Generates cloud-init bash script and runs via `ssh ubuntu@host "sudo /bin/bash" < script`. Installs packages, downloads jujud binary, configures systemd service, enables auto-start.
5. **Bootstrap instance**: Instance ID: `"manual:"` (constant). Status: Always `Running`. Address derived from hostname/IP.

````{dropdown} Troubleshooting

**If you encounter an error of the form `initializing ubuntu user: subprocess encountered error code 255 (ubuntu@{IP}: Permission denied (publickey).)`:**

Edit your `~/.ssh/config` to include the following:

```text
Host <TARGET_IP_ADDRESS>
  IdentityFile ~/.ssh/id_ed25519
  ControlMaster no
```

```{ibnote}
See more: https://bugs.launchpad.net/juju/+bug/2030507
```
````

(unmanaged-controller-resources-created-at-bootstrap)=
### Resources configured at bootstrap

The Unmanaged cloud does not create infrastructure resources. It configures the existing controller machine:

- **Ubuntu user**: Created with passwordless sudo if doesn't exist.
- **Juju agent**: jujud binary downloaded and configured as systemd service.
- **Machine record**: Instance ID `"manual:"`, status `Running`, address from hostname/IP.

(unmanaged-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

````{important}
Machines must be pre-existing and must be specifically targeted. They must be added manually unless they are LXD. Examples:

- `juju add-machine ssh:bob@10.55.60.93`
- `juju add-machine lxd -n 2`

**Further notes:**

- Juju machines are always managed on a per-model basis. With an Unmanaged cloud the `add-machine` process will need to be repeated if the model hosting those machines is destroyed.
- To improve the performance of provisioning newly-added machines consider running an APT proxy or an APT mirror.

```{ibnote}
See more: {ref}`take-your-deployment-offline`
```
````

(unmanaged-machine-constraints)=
### Constraints

The Unmanaged cloud supports the following {ref}`constraints <constraint>`, which are limited to detectable hardware attributes:

**Compute**

- {ref}`constraint-arch`. For controller: the host architecture. For other machines: the architecture from the machine hardware.
- {ref}`constraint-container`
- {ref}`constraint-cores`. Detected from `/proc/cpuinfo`.
- {ref}`constraint-mem`. Detected from `/proc/meminfo`.

**Networking**

- {ref}`constraint-zones`

**Storage**

- {ref}`constraint-root-disk`

(unmanaged-machine-placement-directives)=
### Placement directives

The Unmanaged cloud supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`

(unmanaged-machine-how-machines-are-added)=
### How machines are added

Adding machines with `juju add-machine ssh:[user@]<host>` requires the target to be an existing Ubuntu system. Juju verifies it's not already provisioned, detects hardware, records it in state, and installs the agent via SSH.

**SSH operations when adding a machine:**

1. **Verify pre-existence**: SSH checks if machine already has jujud.
2. **Gather machine info**: Detects base, hardware characteristics. Generates instance ID `"manual:hostname"`.
3. **Install agent**: Runs provisioning script (same cloud-init model as bootstrap).

**Error on constraints-based placement**: The Unmanaged cloud rejects placement specs without explicit `ssh:[user@]<host>` scheme.

(unmanaged-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

The Unmanaged cloud has no cloud-specific storage providers. Users must pre-configure storage or manage it outside Juju.
