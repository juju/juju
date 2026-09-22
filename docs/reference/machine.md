---
myst:
  html_meta:
    description: "Juju machine reference: compute resources on bare metal, VMs, and LXD containers. Understand machine provisioning and unit placement."
---

(machine)=
# Machine

```{ibnote}
See also: {ref}`manage-machines`
```

In Juju, a **machine** is a {ref}`compute resource <resource-compute>` requested implicitly (e.g., through {ref}`command-juju-deploy`, {ref}`command-juju-add-unit`, etc.) or explicitly (e.g., through {ref}`command-juju-add-machine`) from a machine {ref}`cloud <cloud>`.

```{important}

This definition suggests that, regardless of whether the cloud is a bare metal cloud, a virtual machine cloud, or a LXD container- or VM-based cloud, and regardless of whether the command targets a regular instance or rather a LXD container on a regular instance (all the commands above can apply to both), what's being provisioned is always, from the point of view of Juju, a 'machine'.

From the point of view of an end user, this is absolutely true, with one small caveat -- even though listed in `juju` outputs under 'Machines', and in general handled via the same CLI commands as a machine, a LXD container provisioned on top of a regular cloud instance will be named after its host machine; e.g., `0/lxd/5` = LXD container `5` on machine `0`.

```

(machines-and-units)=
## Machines and units

When you deploy an {ref}`application <application>` on a machine, there is usually one {ref}`unit <unit>` per machine. However, it is usually possible to optimise resources by deploying multiple units of the same or of different applications to the same machine.

(machines-and-system-containers)=
## Machines and system (LXD) containers

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: What a machine designation names: machine 0 and its LXD container are rows in the same machine table, the container linked to its host by a machine-parent record -- the designation is that containment path. Two provisioning paths: the controller (its compute provisioner) starts base machines; the host machine's agent provisions its own containers through the LXD broker and watches them via the API. Containers are machines: each runs its own machine agent, which hosts the unit agent.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

In Juju, they are both essentially the same -- 'machines'.  For example, most `juju` CLI commands that target machines can actually target system containers in the exact same way. The container is provisioned by its host machine's agent, not by the controller -- which is also why adding a container brings its host machine in as a machine of its own.

````{dropdown} Example

E.g., `juju add-machine lxd` starts a LXD container on a new machine and adds *both* as 'machines' -- the only difference being that the container 'machine' is prefixed with the ID of its host machine and the annotation `lxd`:

```text
$ juju add-machine lxd
created container 1/lxd/0

$ juju machines
Machine  State    Address         Inst id        Base          AZ  Message
0        started  10.154.118.110  juju-dadfb7-0  ubuntu@22.04      Running
1        pending                  pending        ubuntu@22.04      Creating container
1/lxd/0  pending                  pending        ubuntu@22.04
```

And if you then deploy an application to a LXD container (without specifying any particular container), that will again provision two machines:

```text
$ juju deploy postgresql --to lxd
Located charm "postgresql" in charm-hub, revision 288
Deploying "postgresql" from charm-hub charm "postgresql", revision 288 in channel 14/stable on ubuntu@22.04/stable
ubuntu@charm-dev:~/.local/share/juju$ juju machines
Machine  State    Address         Inst id              Base          AZ  Message
0        started  10.154.118.110  juju-dadfb7-0        ubuntu@22.04      Running
1        started  10.154.118.72   juju-dadfb7-1        ubuntu@22.04      Running
1/lxd/0  pending                  juju-dadfb7-1-lxd-0  ubuntu@22.04      Container started
2        started  10.154.118.209  juju-dadfb7-2        ubuntu@22.04      Running
2/lxd/0  pending                  pending              ubuntu@22.04      acquiring LXD image
```

````

(machine-designations)=
## Machine designations

In Juju, many different commands have a machine argument. The shape of this argument depends on whether the machine is existing vs. new and a regular cloud instance vs. a LXD container on top of a regular cloud instance. The argument can also contain combinations, in comma-separated format. The examples below illustrate all the various cases:

| shape of the machine argument | meaning|
|-|-|
|  | a new machine |
|`0`| machine 0 (an existing machine) |
|`0,4`| machines 0 and 4 (existing machines)|
| `lxd` | a new LXD container or (if specified with `virt-type=virtual-machine`) VM on a new machine |
| `lxd:25`| a new LXD container or (if specified with `virt-type=virtual-machine`) VM on the existing machine 25|
| `0/lxd/4`| the existing LXD container `4` on machine `0`|
|`3,0/lxd/2,lxd:5`| machine 3, existing LXD container 2 on machine 0, and a new LXD container on machine 5|
|`0/lxd/01`| invalid -- numbers are `0` or positive integers without leading zeros|
|`0/lxd/0/lxd/0`| invalid -- only one level of container nesting is supported|

These designations are for machine clouds only: the `--to` argument is rejected on Kubernetes models.

(machine-customisation)=
## Machine customisation
A machine's specific hardware can be customised via {ref}`constraints <constraint>`.

(machine-base)=
## Machine base

```{versionadded} 3.1.0
```

In Juju, a **base** is  a way to identify a particular operating system (OS) image for a Juju {ref}`machine <machine>`.

This can be done via the name of the OS followed by the `@` symbol and the channel of the OS that you want to target, specified in terms of `<track>` or, optionally, `<track>/<risk>`. For example, `ubuntu@22.04` or `ubuntu@22.04/stable`.

A 'base' replaces the older notion of 'series'.

