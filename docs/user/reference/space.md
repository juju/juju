(space)=
# Space

> See also: {ref}`manage-spaces`


A Juju **(network) space** is a logical grouping of {ref}`subnets <subnet>` that can communicate with one another.

A space is used to help segment network traffic for the purpose of:
* Network performance
* Security
* Controlling the scope of regulatory compliance


## Spaces as constraints and bindings

Spaces can be specified as {ref}`constraints <constraint>`---to determine what subnets a machine is connected to---or as {ref}`bindings <binding>`---to determine the subnets used by application relations. 

A binding associates an {ref}`application endpoint <endpoint>` with a space. This restricts traffic for the endpoint to the subnets in the space. By default, endpoints are bound to the space specified in the `default-space` model configuration value. The name of the default space is "alpha".


Constraints and bindings affect application deployment and machine provisioning as well as the subnets a machine can talk to.


Endpoint bindings can be specified during deployment with `juju deploy --bind` or changed after deployment using the `juju bind` command.

## Support for spaces in Juju providers


Support for spaces by the different Juju providers falls into one of three cases.

<!-- Joe says: There are 3 situations: (1) Spaces inherited from the substrate (MAAS). (2) Subnets inherited from the substrate, to be grouped at the discretion of the Juju administrator (LXD, OpenStack, AWS). (3) Subnets are only discovered as machines are added (Manual). -->

### Spaces inherited from the substrate


This is the case for MAAS, as described below.

#### MAAS
 
The concept of spaces is native to MAAS and its API can be used to modify the space/subnet topology. As such, Juju does not permit editing of spaces in a MAAS model. MAAS spaces and subnets are read and loaded into Juju when a new model is created.

If spaces or subnets are changed in MAAS, they can be reloaded into Juju via Juju's `reload-spaces` command.

```{important}

The `reload-spaces` command does not currently pull in all information. This is being worked upon. See [LP #1747998](https://bugs.launchpad.net/juju/+bug/1747998).

```

For other providers, `reload-spaces` will fall back to refreshing the known subnets if subnet discovery is supported. One scenario for this usage would be adding a subnet to an AWS VPC that Juju is using, and then issuing the `reload-spaces` command so that the new subnet is available for association with a Juju space.

### Subnets inherited from the substrate

This is the case for EC2, OpenStack and Azure, and LXD. Inherited subnets are then grouped into spaces at the discretion of the Juju administrator. 

#### EC2
 
Machines on Amazon EC2 are provisioned with a single network device. At this time, specifying multiple space constraints and/or bindings will result in selection of a *single intersecting* space in order to provision the machine.

#### OpenStack and Azure

The OpenStack and Azure providers support multiple network devices. Supplying multiple space constraints and/or bindings will provision machines with NICs in subnets representing the *union* of specified spaces.

#### LXD

LXD automatically detects any subnets belonging to bridge networks that it has access to. It is up to the Juju user to define spaces using these subnets.

### Subnets discovered progressively

This is the case for the Manual provider, as described below.

#### Manual

For the Manual provider, space support differs somewhat from other providers. The `reload-spaces` command does not discover subnets. Instead, each time a manual machine is provisioned, its discovered network devices are used to update Juju's known subnet list.

Accordingly, the machines to be used in a manual provider must be provisioned by Juju before their subnets can be grouped into spaces. When provisioning a machine results in discovery of a new subnet, that subnet will reside in the _alpha_ space.

 
<!--(2) Subnets inherited from the substrate, to be grouped at the discretion of the Juju administrator. -->

<!--FROM https://discourse.charmhub.io/t/network-spaces-and-lxd/6325/3?u=tmihoc
Spaces are supported on LXD insofar as LXD can provision on subnets that you carve into spaces.


We use the LXD network API to detect subnets that are bridge networks. This may we pick up subnets not managed by LXD. On my local machine for example, it detects my Docker bridge and others.

[This patch](https://github.com/juju/juju/pull/12721) was the last in a series that added the support.
-->



<!--
Definition from MAAS: https://maas.io/docs/maas-concepts-and-terms-reference .

But: There are some differences between 

https://discourse.maas.io/t/spaces-vs-fabrics-and-what-they-do/5656
-->

<!--A Juju **(network) space** is a group of one or more subnets that are visible to the Juju controller. -->

<!--
a network partition, protected by firewall rules.
M Vitaly:

- You cannot define a space without defining a subnet.

- The notion of space comes from MAAS. But the notion here is a bit different than it is there. In MAAS, two IP addresses in the same space can reach one another, even if they are from different subnets. In Juju, the definition is that a space is a group of subnets visible to the Juju controller. Visible != reachable. 

-->
