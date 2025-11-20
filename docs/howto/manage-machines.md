(manage-machines)=
# How to manage machines

<!--FIGURE OUT A GOOD PLACE FOR THIS:
An interactive pseudo-terminal (pty) is enabled by default. For the OpenSSH client, this corresponds to the `-t` option ("force pseudo-terminal allocation").

Remote commands can be run as expected. For example: `juju ssh 1 lsb_release -c`. For complex commands the recommended method is by way of the `run` command.
-->

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
Issues during machine provisioning can occur at any stage in the following sequence: Provision resources/a machine M from the relevant cloud, via cloud-init maybe network config, download the `jujud` binaries from the controller, start `jujud`.

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
      version: 3.0.0
    hostname: juju-552e37-0
    dns-name: 10.136.136.175
    ip-addresses:
    - 10.136.136.175
    instance-id: juju-552e37-0
    machine-status:
      current: running
      message: Running
      since: 27 Oct 2022 09:36:10+02:00
    modification-status:
      current: applied
      since: 27 Oct 2022 09:36:07+02:00
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
localhost-model  localhost-controller  localhost/localhost  3.0.0    unsupported  13:51:33+02:00

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
    modification-status:
      current: idle
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
      version: 3.1.0
    hostname: juju-6a1e1b-0
    dns-name: 10.136.136.239
    ip-addresses:
    - 10.136.136.239
    instance-id: juju-6a1e1b-0
    machine-status:
      current: running
      message: Running
      since: 07 Feb 2023 13:53:20+01:00
    modification-status:
      current: applied
      since: 20 Mar 2023 08:53:12+01:00
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

The `exec` command can take many other flags, allowing you to specify an output file, run the commands sequentially (since `juju v.3.0`, the default is to run them in parallel), etc.

```{ibnote}
See more: {ref}`command-juju-exec` (before `juju v.3.0`, `juju run`)
```

(access-a-machine-via-ssh)=
## Access a machine via SSH

There are two ways you can connect to a Juju machine: via `juju ssh` or via a standard SSH client. The former is more secure as it allows access solely from a Juju user with `admin` model access.

### Use the `juju ssh` command

First, make sure you have `admin` access to the model and your public SSH key has been added to the model.

```{important}
If you are the model creator, your public SSH key is already known to `juju` and you already have `admin` access for the model. If you are not the model creator, see {ref}`manage-users` and {ref}`user-access-levels` for how to gain `admin` access to a model and {ref}`manage-ssh-keys` for how to add your SSH key to the model.
```

<!--
<h3 id="heading--providing-access-to-non-initial-controller-admin-juju-users">Providing access to non-initial controller admin Juju users</h3>

In order for a non-initial controller admin user to connect with `juju ssh` that user must:

- be created (`add-user`)
- have registered the controller (`register`)
- be logged in (`login`)
- have 'admin' access to the model
- have their public SSH key reside within the model
- be in possession of the corresponding private SSH key

As previously explained, 'admin' model access and installed model keys can be obtained by creating the model. Otherwise access needs to be granted (`grant`) by a controller admin and keys need to be added (`add-ssh-key` or `import-ssh-key`) by a controller admin or the model admin.
-->

Then, to initiate an SSH session or execute a command on a Juju machine (or container), use the `juju ssh` command followed by the target machine (or container). This target can be specified using a machine (or container) ID or using the ID of the unit that it hosts. Both can be retrieved from the output of `juju status`. For example, below we `ssh` into machine 0 and inside of it run `echo hello`:

```text
juju ssh 0 echo hello
```

By passing further arguments and options, you can also run this on behalf of a different qualified user (other than the current user) or pass a private SSH key instead.

```{ibnote}
See more: {ref}`command-juju-ssh`
```

<!--
Alternatively, you can pass a private key. The easiest way to ensure it is used is to have it stored as `~/.ssh/id_rsa`. Otherwise, you can do one of two things:

1. Use `ssh-agent`

1. Specify the key manually

The second option above, applied to the previous example, will look like this:

```text
juju ssh 0 -i ~/.ssh/my-private-key
```
-->

### Use the OpenSSH `ssh` command

First, make sure you've added a public SSH key for your user to the target model.

```{important}
If you are the model creator, your public SSH key is already known to `juju` and you already have `admin` access for the model. If you are not the model creator, see {ref}`manage-users` and {ref}`user-access-levels` for how to gain `admin` access to a model and {ref}`manage-ssh-keys` for how to add your SSH key to the model.
```

Alternatively, for direct access using a standard SSH client, it is also possible to add the key to an individual machine using standard methods (manually copying a key to the `authorized_keys` file or by way of a command such as `ssh-import-id` in the case of Ubuntu).

Then, to connect to a machine via the OpenSSH client, use the OpenSSH `ssh` command followed by `<user account>@<machine IP address`, where the default user account added to a Juju machine, to which public SSH keys added by `add-ssa-key` or `import-ssh-key`, is `ubuntu`. For example, for a machine with an IP address of `10.149.29.143`, do the following:

```text
ssh ubuntu@10.149.29.143
```

```{ibnote}
See more: [OpenSSH](https://www.openssh.com/)
```

## Copy files securely between machines

The `scp` command copies files securely to and from machines.

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

```{important}
Juju cannot transfer files between two remote units because it uses public key authentication exclusively and the native (OpenSSH) `scp` command disables agent forwarding by default. Either the destination or the source must be local (to the Juju client).
```

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
