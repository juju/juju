(cloud-manual)=
# The Manual cloud and Juju

This document describes details specific to using the Manual (`manual`) cloud with Juju.

```{important}

The Manual (`manual`) cloud is a cloud you create with Juju from existing machines.

The purpose of the Manual cloud is to cater to the situation where you have machines (of any nature) at your disposal and you want to create a backing cloud out of them.

If this collection of machines is composed solely of bare metal you might opt for a {ref}`cloud-maas`. However, recall that such machines would also require [IPMI hardware](https://docs.maas.io/en/nodes-power-types) and a MAAS infrastructure. In contrast, the Manual cloud can make use of a collection of disparate hardware as well as of machines of varying natures (bare metal or virtual), all without any extra overhead/infrastructure.

```


When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Requirements

- At least two pre-existing machines (one for the controller and one where charms will be deployed).<br> - The machines must be running on Ubuntu.<br> - The machines must be accessible over SSH from the terminal you're running the Juju client from  using public key authentication (in whichever way you want to make that possible using generic Linux mechanisms).<p> (`sudo` rights will suffice if this provides root access. If a password is required for `sudo`, juju will ask for it on the command line.) <p> - The machines must be able to `ping` one another.


## Notes on `juju add-cloud`

Type in Juju: `manual`

Name in Juju: User-defined.

Enter the SSH connection information for the machine where a Juju controller will be bootstrapped, e.g., `username@<hostname or IP>` (where we assume `username` is `ubuntu`) or `<hostname or IP>`.

## Notes on `juju add-credential`


### Authentication types

No preset auth-types. Just make sure you can SSH into the controller machine.


## Notes on `juju bootstrap`


The machine that will be allocated to run the controller on is the one specified during the `add-cloud` step. <p>**If you encounter an error of the form "initializing ubuntu user: subprocess encountered error code 255 (ubuntu@{IP}: Permission denied (publickey).)":** <br> Edit your `~/.ssh/config` to include the following: <br> `Host <TARGET_IP_ADDRESS>` &nbsp;&nbsp;&nbsp;&nbsp; <br>&nbsp;&nbsp;&nbsp;&nbsp;`IdentityFile ~/.ssh/id_ed25519`<br> &nbsp;&nbsp;&nbsp;&nbsp; `ControlMaster no`<br> See more: https://bugs.launchpad.net/juju/+bug/2030507 .

## Notes on `juju deploy`


With any other cloud, the Juju client can trigger the creation of a backing machine (e.g. a cloud instance) as they become necessary. In addition, the client can also cause charms to be deployed automatically onto those newly-created machines. However, with a Manual cloud the machines must pre-exist and they must also be specifically targeted during charm deployment.


(Note: A MAAS cloud must also have pre-existing backing machines. However, Juju, by default, can deploy charmed operators onto those machines, or add a machine to its pool of managed machines, without any extra effort.)

Machines must be added manually, unless they are LXD. Example: <p>  `juju add-machine ssh:bob@10.55.60.93` <br> `juju add-machine lxd -n 2`


Further notes: <br> - Juju machines are always managed on a per-model basis. With a Manual cloud the `add-machine` process will need to be repeated if the model hosting those machines is destroyed. <br> -   To improve the performance of provisioning newly-added machines consider running an APT proxy or an APT mirror. See more: {ref}`take-your-deployment-offline`.

## Cloud-specific model configuration keys

N/A

## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                                                                                  |
|----------------------------------------|------------------------------------------------------------------------------------------------------------------|
| conflicting:                           |                                                                                                                  |
| supported?                             |                                                                                                                  |
| - {ref}`constraint-allocate-public-ip` | &#10005;                                                                                                         |
| - {ref}`constraint-arch`               | &#10003;  <br> Valid values: For controller: `{ref}`host arch]`. For other machines: arch from machine hardware. |
| - {ref}`constraint-container`          | &#10003;                                                                                                         |
| - {ref}`constraint-cores`              | &#10003;                                                                                                         |
| - {ref}`constraint-cpu-power`          | &#10005;                                                                                                         |
| - {ref}`constraint-image-id`           | &#10005;                                                                                                         |
| - {ref}`constraint-instance-role`      | &#10005;                                                                                                         |
| - {ref}`constraint-instance-type`      | &#10005;                                                                                                         |
| - {ref}`constraint-mem`                | &#10003;                                                                                                         |
| - {ref}`constraint-root-disk`          | &#10003;                                                                                                         |
| - {ref}`constraint-root-disk-source`   | &#10005;                                                                                                         |
| - {ref}`constraint-spaces`             | &#10005;                                                                                                         |
| - {ref}`constraint-tags`               | &#10005;                                                                                                         |
| - {ref}`constraint-virt-type`          | &#10005;                                                                                                         |
| - {ref}`constraint-zones`              | &#10003;                                                                                                         |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| {ref}`placement-directive-machine`               | TBA      |
| {ref}`placement-directive-subnet`                | &#10005; |
| {ref}`placement-directive-system-id`             | &#10005; |
| {ref}`placement-directive-zone`                  | TBA      |

