(command-juju-add-machine)=
# `juju add-machine`
> See also: [remove-machine](#remove-machine), [model-constraints](#model-constraints), [set-model-constraints](#set-model-constraints)

## Summary
Provision a new machine or assign one to the model.

## Usage
```juju add-machine [options] [<container-type>[:<machine-id>] | ssh:[<user>@]<host> | <placement>] | <private-key> | <public-key>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--base` |  | Specify the operating system base to install on the new machine(s) |
| `--constraints` | [] | Specify the machine constraints to overwrite those available from `juju model-constraints` and provider's defaults |
| `--disks` |  | Specify the storage constraints for disks to attach to the machine(s) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-n` | 1 | Specify the number of machines to add |
| `--private-key` |  | Specify the path to the private key to use during the connection |
| `--public-key` |  | Specify the path to the public key to add to the remote authorized keys |
| `--series` |  | Specify the operating system series to install on the new machine(s). DEPRECATED use --base |

## Examples

Start a new machine by requesting one from the cloud provider:

	juju add-machine

Start 2 new machines:

	juju add-machine -n 2

Start a LXD container on a new machine instance and add both as machines:

	juju add-machine lxd

Start two machine instances, each hosting a LXD container, then add all four as machines:

	juju add-machine lxd -n 2

Create a container on machine 4 and add it as a machine:

	juju add-machine lxd:4

Start a new machine and require that it has 8GB RAM:

	juju add-machine --constraints mem=8G

Start a new machine within the "us-east-1a" availability zone:

	juju add-machine --constraints zones=us-east-1a

Start a new machine with at least 4 CPU cores and 16GB RAM, and request three storage volumes to be attached to it. Two are large capacity (1TB) HDD and one is a lower capacity (100GB) SSD. Note: 'ebs' and 'ebs-ssd' are storage pools specific to AWS.

	juju add-machine --constraints="cores=4 mem=16G" --disks="ebs,1T,2 ebs-ssd,100G,1"

Allocate a machine to the model via SSH:

	juju add-machine ssh:user@10.10.0.3

Allocate a machine specifying the private key to use during the connection:

	juju add-machine ssh:user@10.10.0.3 --private-key /tmp/id_rsa

Allocate a machine specifying a public key to set in the list of authorized keys in the machine:

	juju add-machine ssh:user@10.10.0.3 --public-key /tmp/id_rsa.pub

Allocate a machine specifying a public key to set in the list of authorized keys and the private key to used during the connection:

	juju add-machine ssh:user@10.10.0.3 --public-key /tmp/id_rsa.pub --private-key /tmp/id_rsa

Allocate a machine to the model. Note: specific to MAAS.

	juju add-machine host.internal


## Details

Add a new machine to the model.

The command operates in three modes, depending on the options provided:

  - provision a new machine from the cloud (default, see "Provisioning
    a new machine")
  - create an operating system container (see "Container creation")
  - connect to a live computer and allocate it as a machine (see "Manual
    provisioning")

The `add-machine` command is unavailable in Kubernetes clouds. Provisioning
a new machine is unavailable on the manual cloud provider.

Once the` add-machine` command has finished, the machine's ID can be
used as a placement directive for deploying applications. Machine IDs
are also accessible via `juju status` and `juju machines`.


**Provisioning a new machine**

When add-machine is called without arguments, Juju provisions a new
machine instance from the current cloud. The machine's specifications,
including whether the machine is virtual or physical, depend on the cloud.

To control which instance type is provisioned, use the `--constraints`  and
`--base` options. `--base` can be specified using the OS name and the version of
the OS, separated by `@`. For example, `--base ubuntu@22.04`.

To add storage volumes to the instance, provide a whitespace-delimited
list of storage constraints to the `--disks` option.

Add placement directives as an argument to `--to` to give Juju additional information
about how to allocate the machine in the cloud. For example, one can direct
the MAAS provider to acquire a particular node by specifying its hostname.


**Manual provisioning**

Call `add-machine`  with the address of a network-accessible computer to
allocate that machine to the model.

Manual provisioning is the process of installing Juju on an existing machine
and bringing it under Juju's management. The Juju controller must be able to
access the new machine over the network.


**Container creation**

If a operating system container type is specified (e.g. "lxd" or "kvm"),
then `add-machine` will allocate a container of that type on a new machine
instance. Both the new instance, and the new container will be available
as machines in the model.

It is also possible to add containers to existing machines using the format
`<container-type>:<machine-id>`. Constraints cannot be combined this mode.