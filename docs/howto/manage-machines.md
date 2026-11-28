---
myst:
  html_meta:
    description: "Complete guide to managing Juju machines: add, remove, upgrade, SSH access, and manual provisioning for cloud and bare metal infrastructure."
---

(manage-machines)=
# How to manage machines

```{ibnote}
See also: {ref}`machine`
```

This document shows how to manage machines.

(add-a-machine)=
## Add a machine

To add a new machine to a model, run the `add-machine` command, as below. `juju` will start a new machine by requesting one from the cloud provider.


```text
juju add-machine
```

The command also provides many options. By using them you can customize many things. For example, you can provision multiple machines, specify a base, choose to deploy on a LXD container *inside* a machine, apply various constraints to the machine (e.g., storage, spaces, ...) to override more general defaults (e.g., at the model level), etc.

Machines provisioned via `add-machine` can be used for an initial deployment (`deploy`) or a scale-out deployment (`add-unit`).

```{ibnote}
See more: {ref}`command-juju-add-machine`
```

````{note}
Issues during machine provisioning can occur at any stage in the following sequence: Provision resources/a machine M from the relevant cloud, via cloud-init maybe network config, download the `jujuagentd` binaries from the controller, start `jujuagentd`.

To troubleshoot, try to gather more information until you understand what caused the issue.

```{ibnote}
See more: {ref}`troubleshoot-your-deployment`
```

````

## Retry provisioning for a failed machine

To retry provisioning (a ) machine(s) (e.g., for a failed `deploy`, `add-unit`, or `add-machine`), run the `retry-provisioning` command followed by the (space-separated) machine ID(s). For example:

```text
juju retry-provisioning 3 27 57
```

```{ibnote}
See more: {ref}`command-juju-retry-provisioning`
```

## List all machines

To see a list of all the available machines, use the `machines` command:

```text
juju machines
```

The output should be similar to the one below:

```text
Machine  State    Address         Inst id        Base          AZ  Message
0        started  10.136.136.175  juju-552e37-0  ubuntu@22.04      Running
1        started  10.136.136.62   juju-552e37-1  ubuntu@22.04      Running
```

```{ibnote}
See more: {ref}`command-juju-machines`
```

## View details about a machine

To see details about a machine, use the `show-machine` command followed by the machine ID. For example:

```text
juju show-machine 0
```

````{dropdown} Example output

```text
model: localhost-model
machines:
  "0":
    juju-status:
      current: started
      since: 27 Oct 2022 09:37:17+02:00
      version: 4.0.15
    hostname: juju-552e37-0
    dns-name: 10.136.136.175
    ip-addresses:
    - 10.136.136.175
    instance-id: juju-552e37-0
    machine-status:
      current: running
      message: Running
      since: 27 Oct 2022 09:36:10+02:00
    base:
      name: ubuntu
      channel: "22.04"
    network-interfaces:
      eth0:
        ip-addresses:
        - 10.136.136.175
        mac-address: 00:16:3e:cc:f2:16
        gateway: 10.136.136.1
        space: alpha
        is-up: true
    constraints: arch=amd64
    hardware: arch=amd64 cores=0 mem=0M
```

````

```{ibnote}
See more: {ref}`command-juju-show-machine`
```

## View the status of a machine

To see the status of a machine, use the `status` command:

```text
juju status
```

This will report the status of the model, its applications, its units, and also its machines.

````{dropdown} Example output

```text
Model            Controller            Cloud/Region         Version  SLA          Timestamp
localhost-model  localhost-controller  localhost/localhost  4.0.15   unsupported  13:51:33+02:00

App       Version  Status   Scale  Charm     Channel  Rev  Exposed  Message
influxdb           waiting    0/1  influxdb  stable    24  no       waiting for machine

Unit        Workload  Agent       Machine  Public address  Ports  Message
influxdb/0  waiting   allocating  2                               waiting for machine

Machine  State    Address  Inst id  Base          AZ  Message
2        pending           pending  ubuntu@20.04      Retrieving image: rootfs: 4% (10.87MB/s)

```

````

```{ibnote}
See more: {ref}`command-juju-status`
```

## Manage constraints for a machine

```{ibnote}
See also: {ref}`constraint`
```

**Set values.** You can set constraint values for an individual machine when you create it manually, by using the `add-machine` command with the `constraints` flag followed by a quotes-enclosed list of your desired key-value pairs, for example:

```text
juju add-machine --constraints="cores=4 mem=16G"
```

````{dropdown} Example outcome

```text
$ juju add-machine --constraints="cores=4 mem=16G"
created machine 0
$ juju show-machine 0
model: test
machines:
  "0":
    juju-status:
      current: pending
      since: 20 Mar 2023 12:58:52+01:00
    instance-id: pending
    machine-status:
      current: pending
      since: 20 Mar 2023 12:58:52+01:00
    base:
      name: ubuntu
      channel: "22.04"
    constraints: cores=4 mem=16384M
```

````


```{ibnote}
See more: {ref}`command-juju-add-machine`, {ref}`add-a-machine`
```

**Get values.** You can get constraint values for an individual machine by viewing details about the command with the `show-machine` command, for example:

```text
juju show-machine 0
```

````{dropdown} Example output

```text
model: controller
machines:
  "0":
    juju-status:
      current: started
      since: 01 Mar 2023 15:08:34+01:00
      version: 4.0.15
    hostname: juju-6a1e1b-0
    dns-name: 10.136.136.239
    ip-addresses:
    - 10.136.136.239
    instance-id: juju-6a1e1b-0
    machine-status:
      current: running
      message: Running
      since: 07 Feb 2023 13:53:20+01:00
    base:
      name: ubuntu
      channel: "22.04"
    network-interfaces:
      enp5s0:
        ip-addresses:
        - 10.136.136.239
        mac-address: 00:16:3e:4d:aa:69
        gateway: 10.136.136.1
        space: alpha
        is-up: true
    constraints: virt-type=virtual-machine
    hardware: arch=amd64 cores=0 mem=0M virt-type=virtual-machine
    controller-member-status: has-vote
```

````

```{ibnote}
See more: {ref}`command-juju-show-machine`
```

## Execute a command inside a machine

To run a command in a machine, use the `exec` command followed by the target machine(s) (`--all`, `--machine`, `--application` or `--unit`) and the commands you want to run. For example:

```text

# Run the 'echo' command in the machine corresponding to unit 0 of the 'ubuntu' application:
juju exec --unit ubuntu/0 echo "hi"

# Run the 'echo' command in all the machines corresponding to the 'ubuntu' application:
 juju exec --application ubuntu echo "hi"

```

The `exec` command can take many other flags, allowing you to specify an output file, run the commands sequentially (the default is to run them in parallel), etc.

```{ibnote}
See more: {ref}`command-juju-exec`
```

(access-a-machine-via-ssh)=
## Access a machine via SSH

```{ibnote}
See also: {ref}`ssh-server-access`
```

Before connecting, check your {ref}`access to the destination model <ssh-server-access>` and {ref}`add your public key <add-an-ssh-key>` if needed. You can connect using the Juju CLI or OpenSSH.

(access-a-machine-with-the-juju-cli)=
### Use the `juju ssh` command

To open a shell or run a command on a machine, use the machine ID or the name of a unit it hosts from `juju status`. For example, to run `echo hello` on machine 0:

```text
juju ssh 0 echo hello
```

```{ibnote}
See more: {ref}`command-juju-ssh`
```

(verify-ssh-host-fingerprints)=
### Use the OpenSSH `ssh` command

Before starting OpenSSH, display the two expected {ref}`host key fingerprints <ssh-server-host-keys>` for the target, for example machine 0:

```text
juju ssh --print-keys 0
```

Then print the OpenSSH command for the same target. If you use a nondefault private key for the jump server, include `--ssh-key /path/to/private-key` on this invocation so the printed command uses it:

```text
juju ssh --show-command 0
```

Run the printed command. If OpenSSH asks you to trust a key, compare the first hop with the **controller SSH jump server** fingerprint and the inner hop with the **target virtual host** fingerprint. Accept only a match for that hop.

```{ibnote}
See more: [OpenSSH](https://www.openssh.com/)
```

(recover-access-to-a-machine)=
### Recover access to a machine

If the machine agent cannot start, use access provided by the cloud, such as `lxc shell` for an LXD instance. To prepare a direct SSH recovery route in advance, provision machines with a separate administrator public key using {ref}`model-config-cloudinit-userdata`. Direct SSH also requires a network route to the machine.

(copy-files-between-machines)=
## Copy files securely between machines

```{ibnote}
See also: {ref}`ssh-server-file-transfer`
```

Use `juju scp` with a machine ID or the name of a machine-backed unit. For a copy between two remote machines, pass `-3` to OpenSSH `scp` to transfer the data through your client. For example:

```text
juju scp -- -3 0:/var/tmp/report.txt 1:/var/tmp/report.txt
```

```{caution}
Options specific to `scp` must be preceded by double dashes: `--`.
```

````{dropdown} Examples:

Copy 2 files from two MySQL units to the local backup/ directory, passing `-v` to scp as an extra argument:

```text
juju scp -- -v mysql/0:/path/file1 mysql/1:/path/file2 backup/
```

Recursively copy the directory `/var/log/mongodb/` on the first MongoDB server to the local directory remote-logs:

```text
juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs/
```

Copy a local file to the second apache2 unit in the model "testing". Note that the `-m` here is a Juju argument so the characters `--` are not used:

```text
juju scp -m testing foo.txt apache2/1:
```

````

```{ibnote}
See more: {ref}`command-juju-scp`
```

## Remove a machine

```{ibnote}
See also: {ref}`removing-things`
```

To remove a machine, use the `remove-machine` command followed by the machine ID. For example:

```text
juju remove-machine 3
```

```{important}
It is not possible to remove a machine that is currently hosting either a unit or a container. Either remove all of its units (or containers) first or, as a last resort, use the `--force` option.
```

```{note}
In some situations, even with the `--force` option, the machine on the backing cloud may be left alive. Examples of this include the Unmanaged cloud or if harvest provisioning mode is not set. In addition to those situations, if the client has lost connectivity with the backing cloud, any backing cloud, then the machine may not be destroyed, even if the machine's record has been removed from the Juju database and the client is no longer aware of it.
```

By using various options, you can also customize various other things, for example, the model or whether to keep the running cloud instance or not.

```{ibnote}
See more: {ref}`command-juju-remove-machine`
```
