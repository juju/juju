---
myst:
  html_meta:
    description: "Create a Manual cloud in Juju from existing machines, including SSH requirements, bare metal setup, and cloud configuration without MAAS."
---

(cloud-manual)=
# Manual

In Juju, Manual is a {ref}`machine cloud <machine-cloud>` that adopts existing machines via SSH. It behaves like all {ref}`machine clouds <machine-clouds>`, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

```{important}
The Manual cloud is a cloud you create with Juju from existing machines. Manual does not provision new infrastructure -- it brings existing Ubuntu/Debian systems under Juju management via SSH.

The purpose of the Manual cloud is to cater to the situation where you have machines (of any nature) at your disposal and you want to create a backing cloud out of them.

If this collection of machines is composed solely of bare metal you might opt for a {ref}`MAAS cloud <cloud-maas>`. However, recall that such machines would also require [IPMI hardware](https://docs.maas.io/en/nodes-power-types) and a MAAS infrastructure. In contrast, the Manual cloud can make use of a collection of disparate hardware as well as of machines of varying natures (bare metal or virtual), all without any extra overhead/infrastructure.
```

(manual-cloud)=
## The cloud

The Manual cloud in Juju.

(manual-cloud-definition)=
### Definition

Type in Juju: `manual`

Name in Juju: User-defined

A Manual machine is any Ubuntu/Debian system reachable via SSH with sudo privileges. Machines are not created by Juju -- they must be provisioned externally and brought under Juju management via SSH credentials.

(manual-cloud-requirements)=
### Requirements

- At least two pre-existing machines (one for the controller and one where charms will be deployed).
- The machines must be running Ubuntu.
- The machines must be accessible over SSH from the terminal you're running the Juju client from using public key authentication.
- SSH user must have sudo rights (passwordless sudo preferred, but Juju will prompt for password if needed).
- The machines must be able to ping one another.

(manual-credential)=
## Credentials

Credentials for the Manual cloud.

(manual-credential-authentication-types)=
### Authentication types

No preset authentication types. Ensure you can SSH into the controller machine using public key authentication. Juju uses standard SSH mechanisms (private key, optionally password auth, PTY enablement).

(manual-controller)=
## Controllers

Controllers bootstrapped on the Manual cloud.

When adding the cloud, enter the SSH connection information for the machine where a Juju controller will be bootstrapped, e.g., `username@<hostname or IP>` (where we assume `username` is `ubuntu`) or `<hostname or IP>`.

(manual-controller-bootstrap-behavior)=
### Bootstrap behavior

Bootstrap initializes the remote machine by creating an `ubuntu` user with passwordless sudo, detecting hardware characteristics via SSH, and configuring the Juju agent through cloud-init-style bash scripts executed remotely.

**SSH operations during bootstrap:**

1. **Ubuntu user setup**: SSH to `ubuntu@host` (or provided user). If `ubuntu` user doesn't exist, creates it with passwordless sudo via `sudo /bin/bash` script.
2. **Provisioning check**: Verifies no jujud service already exists (fails if found).
3. **Hardware detection**: SSH script reads `/etc/os-release`, `uname`, `/proc/meminfo`, `/proc/cpuinfo` to detect OS, architecture, memory, CPU cores.
4. **Machine configuration**: Generates cloud-init bash script and runs via `ssh ubuntu@host "sudo /bin/bash" < script`. Installs packages, downloads jujud binary, configures systemd service, enables auto-start.
5. **Bootstrap instance**: Instance ID: `"manual:"` (constant). Status: Always `Running`. Address derived from hostname/IP.

```{dropdown} Troubleshooting

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
```

(manual-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

Manual does not create infrastructure resources. It configures existing machines:

- **Ubuntu user**: Created with passwordless sudo if doesn't exist.
- **Juju agent**: jujud binary downloaded and configured as systemd service.
- **Machine record**: Instance ID `"manual:"`, status `Running`, address from hostname/IP.

(manual-model)=
## Models

Models connected to the Manual cloud.

(manual-model-cloud-specific-configuration-keys)=
(manual-model-configuration-keys)=
### Configuration keys

None.

(manual-machine)=
## Machines

Machines provisioned on the Manual cloud.

```{important}
With any other cloud, the Juju client can trigger the creation of a backing machine (e.g., a cloud instance) as they become necessary. However, with a Manual cloud the machines must pre-exist and they must also be specifically targeted during deployment.

Machines must be added manually, unless they are LXD. Examples:

- `juju add-machine ssh:bob@10.55.60.93`
- `juju add-machine lxd -n 2`

**Further notes:**

- Juju machines are always managed on a per-model basis. With a Manual cloud the `add-machine` process will need to be repeated if the model hosting those machines is destroyed.
- To improve the performance of provisioning newly-added machines consider running an APT proxy or an APT mirror.

```{ibnote}
See more: {ref}`take-your-deployment-offline`
```
```

(manual-machine-supported-constraints)=
(manual-machine-constraints)=
### Constraints

Constraints are limited to detectable hardware attributes:

- {ref}`constraint-arch`: For controller: the host architecture. For other machines: the architecture from the machine hardware.
- {ref}`constraint-container`
- {ref}`constraint-cores`: Detected from `/proc/cpuinfo`.
- {ref}`constraint-mem`: Detected from `/proc/meminfo`.
- {ref}`constraint-root-disk`
- {ref}`constraint-zones`

(manual-machine-supported-placement-directives)=
(manual-machine-placement-directives)=
### Placement directives

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`

(manual-machine-how-machines-are-added)=
### How machines are added

Adding machines with `juju add-machine ssh:[user@]<host>` requires the target to be an existing Ubuntu system. Juju verifies it's not already provisioned, detects hardware, records it in state, and installs the agent via SSH.

**SSH operations when adding a machine:**

1. **Verify pre-existence**: SSH checks if machine already has jujud.
2. **Gather machine info**: Detects base, hardware characteristics. Generates instance ID `"manual:hostname"`.
3. **Record in state**: Calls AddMachines API with detected specs.
4. **Install agent**: Runs provisioning script (same cloud-init model as bootstrap).

**Error on constraints-based placement**: Manual rejects placement specs without explicit `ssh:[user@]<host>` scheme.

(manual-machine-limitations)=
### Limitations

- **No infrastructure creation**: Manual cannot create new machines. Use `juju add-machine ssh:[user@]<host>` to adopt existing machines.
- **No StartInstance**: Returns `errNoStartInstance`.
- **No StopInstances**: Returns `errNoStopInstance`.
- **No firewall control**: All firewall ops (`open-ports`, `close-ports`) are no-ops.
- **No storage providers**: `StorageProviderTypes()` returns `nil`. Users must pre-configure storage or manage it outside Juju.
- **Limited network discovery**: Subnets, network interfaces, spaces routing not supported. Address detection uses DNS lookup to verify hostname is resolvable.
- **No scaling**: Manual cannot auto-scale. Each machine must be added explicitly.
- **Destroy is limited**: Killing controller kills jujud via SSH and stops juju-db service. Model destroy is no-op (nothing to destroy).

(manual-storage)=
## Cloud-specific storage providers

None. Manual has no storage support. Users must pre-configure storage or manage it outside Juju.

