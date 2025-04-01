(placement-directive)=
# Placement directive

<!--TO DOCS MAINTAINERS:
To retrieve info about the keys, grep the `provider` directory in the code for `placement` (case insensitive); find all the providers that match; and go to each of those providers' `parsePlacement` method and look at the code. For example, here's the ec2 one: https://github.com/juju/juju/blob/137a772ed339b73b856e9adc0a5624976c2890b2/provider/ec2/environ.go#L389 (note the switch statement with two cases, `zone` and `subnet`). Then follow a couple of the functions through to get further details (e.g., about the ec2 subset query).
--->

<!-- > See also: {ref}`Binding <binding>`, {ref}`Constraint <constraint>`-->

In Juju, a **placement directive** is an option based on the `--to` flag that can be passed to certain commands to specify a deploy location, where the commands include {ref}`command-juju-add-machine` ,  {ref}`command-juju-add-unit`,  {ref}`command-juju-bootstrap`,  {ref}`command-juju-deploy`,  {ref}`command-juju-enable-ha`, and the location is  (1) an existing or a new machine or (2) a key-value pair specifying a subnet, system ID, or an availability zone. 

Example: `juju add-machine --to 1`, `juju deploy --to zone=us-east-1a`


The rest of this document gives details about the locations.

<!-- where the zone key may be used to override a `zones` {ref}`constraint <constraint>`.  -->

```{caution}

When the location is a key-value pair, its availability and meaning may vary from cloud to cloud. For details see {ref}`list-of-supported-clouds` > `<cloud name>`.

```

## List of placement directive locations

(placement-directive-machine)=
### `<machine>`

Depending on whether this is an existing machine or a new machine, this will be: 

- The existing machine ID.

**Examples:** `1` (existing machine `1`),  `5/lxd/0` (existing container `0` on machine `5`)

- A new machine, specifying a type or relative location.

**Examples:** `lxd` (new container on a new machine), `lxd:5` (new container on machine 5) 

> See more: {ref}`machine-designations`

(placement-directive-subnet)=
### `subnet=<subnet>`

<!--**Value:** The name of the subnet.-->


Available for Azure and AWS EC2.


(placement-directive-system-id)=
### `system-id=<system ID>`

<!--**Value:** The system id.-->

Available for MAAS.

(placement-directive-zone)=
### `zone=<zone>`

<!--**Value:** The name of the availability zone.-->

**Purpose:** To specify an availability zone.

```{important}

The `zone` placement directive may be used to override a `zones` {ref}`constraint <constraint>`.

```

**Example:** `zone=us-east-1a`

<!--

|key|value|Notes|
| --- | --- | --- |
|<a href="#heading--subnet"><h4 id="heading--subnet">`subnet`</h4></a>|`<subnet-name>`|If the query looks like a CIDR, then we will match subnets with the same CIDR. If it follows the syntax of a "subnet-XXXX" then we will match the Subnet ID. Everything else is just matched as a Name. <p> Available for Azure and AWS EC2.|
|<a href="#heading--system-id"><h4 id="heading--system-id">`system-id`</h4></a>|`<system-id>`| Available for MAAS. |
|<a href="#heading--zone"><h4 id="heading--zone">`zone`</h4></a>|`<availability-zone>`|If there's no '=' delimiter, assume it's a node name. <p> Available for Amazon AWS EC2, Google GCE, LXD, MAAS, OpenStack, VMware vSphere. <p> Can also be used as a {ref}`constraint <constraint>`. If used together, the placement directive takes precedence. </p> |

-->

<!-- For reference, I found these by grepping the `provider` directory in the code for `placement` (case insensitive), then finding all the providers that matched. Then going to each of those providers' `parsePlacement` method and looking at the code. For example, here's the ec2 one: https://github.com/juju/juju/blob/137a772ed339b73b856e9adc0a5624976c2890b2/provider/ec2/environ.go#L389 (note the switch statement with two cases, `zone` and `subnet`). Then I followed a couple of the functions through to get for example that comment about ec2 subset query.

azure:
    subnet: <subnet name>

ec2:
    zone: <availability zone>

    // If the query looks
    // like a CIDR, then we will match subnets with the same CIDR. If it follows
    // the syntax of a "subnet-XXXX" then we will match the Subnet ID. Everything
    // else is just matched as a Name.
    subnet: <subnet query>

gce:
    zone: <availability zone>

lxd:
    // If there's no '=' delimiter, assume it's a node name.

    zone: <availability zone>

maas:
    // If there's no '=' delimiter, assume it's a node name.

    zone: <availability zone>

    system-id: <system id>

openstack:
    zone: <availability zone>

vsphere:
    zone: <availability zone>

-->

<!--
flag with the syntax `--to` that can be passed to certain commands to specify a location---the argument of the flag. 

 specifies which unit to deploy an application to. Commonly used to deploy multiple applications in the same unit.


bootstrap --to

| command| `--to` argument |
|--|--|
| `bootstrap`, `deploy`, `add-machine` | zone, machine, instance, subnet |
| `deploy`, `add-unit`, `enable-ha`|  deployed machines|


2 cases:

1. with deploy and add-machine: for the purpose of provisioning a machine to be used. The argument of the placement directive is a zone, machine, instance (only MAAS), subnet (only AWS, GCE, Azure)


2. with deploy, add-unit and enable-ha: choosing which deployed machines you want to target. Argument is a machine ID.

Placement directives may vary by provider. In contrast, constraints do not. >> ACTUALLY, NOT TRUE

PD are instantaneous. Constraints designed to be for a set of things, they will apply to any new entity from that moment onward.

Constraint vs. binding: Bindings imply a space constraint. You're forcing anyone who's entering into a relation with you to have the same space constraint.
-->
