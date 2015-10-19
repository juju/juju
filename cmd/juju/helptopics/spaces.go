// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Spaces = `
Juju provides a set of features allowing the users to have better and
finer-grained control over the networking aspects of the environment
and service deployments in particular. Not all cloud providers support
these enhanced networking features yet, in fact they are currently
supported on AWS only. Support for MaaS and OpenStack is planed and
will be available in future releases of Juju.

Juju network spaces (or just "spaces") represent sets of disjoint
subnets available for running cloud instances, which may span one
or more availability zones ("zones"). Any given subnet can be part of
one and only one space. All subnets within a space are considered "equal"
in terms of access control, firewall rules, and routing. Communication
between spaces on the other hand (e.g. between instances started in
subnets part of different spaces) will be subject to access restrictions
and isolation.

Having multiple subnets spanning different zones within the same space
allows Juju to perform automatic distribution of units of a service
across zones inside the same space. This allows for high-availability
for services and spreading the instances evenly across subnets and zones.

As an example, consider an environment divided into three segments with
distinct security requirements:

- The "dmz" space for publicly-accessible services (e.g. HAProxy) providing
  access to the CMS application behind it.
- The "cms" space for content-management applications accessible via the "dmz"
  space only.
 -The "database" space for backend database services, which should be accessible
  only by the applications.

HAProxy is deployed inside the "dmz" space, it is accessible from the Internet
and proxies HTTP requests to one or more Joomla units in the "cms" space.
The backend MySQL for Joomla is running in the "database" space. All subnets
within the "cms" and "database" spaces provide no access from outside the
environment for security reasons. Using spaces for deployments like this allows
Juju to have the necessary information about how to configure the firewall and
access control rules. In this case, instances in "dmz" can only communicate
with instances in "apps", which in turn are the only ones allowed to access
instances in "database".

Please note, Juju does not yet enforce those security restrictions, but having
spaces and subnets available makes it possible to implement those restrictions
and access control in a future release.

Due to the ability of spaces to span multiple zones services can be distributed
across these zones. This allows high available setup for services within the
environment.

Spaces are created like this:

$ juju space create <name> [ <CIDR1> <CIDR2> ... ] [--private|--public]

They can be listed in various formats using the "list" subcommand. See
also "juju space help" for more information.

$ juju space update <name> <CIDR1> [ <CIDR2> ... ]

Other space subcommands are "list", "rename", and "remove". Subnets
are created and added to spaces like this:

$ juju subnet create <CIDR> <space> <zone1> [<zone2> ...] [--vlan-tag <integer>]
[--private|--public]

Additionally existing subnets can be added using

$ juju subnet add <CIDR>|<subnet-provider-id> <space> [<zone1> <zone2> ...]

Like spaces they can be listed by the subcommand "list". See
also "juju space help" for more information.

The commands "add-machine" and "deploy" allow the specification of a
spaces constraint for the selection of a matching instance. It is done by
adding:

--constraints spaces=<allowedspace1>,<allowedspace2>,^<disallowedspace>

The constraint controls which instance is chosen for the new machine or 
unit. This instance has to have distinct IP addresses on any subnet of
each allowed space in the list and none of the subnets associated with one
of the disallowed spaces which are prefixed with a caret. Later changes
of the spaces constraint have to be done with care, fixed bindings of
services are coming son.

For more information regarding constraints in general, see "juju help constraints".

So to create the environment above the first step has to be done at the provider.
Here you create the subnets distributed over the available zones, e.g.

- For the "dmz" create 10.1.1.0/24 in zone A and 10.1.2.0/24 in zone B.
- For the "cms" create 10.1.3.0/24 in zone A and 10.1.4.0/24 in zone B.
- For the "database" create 10.1.5.0/24 in zone A and 10.1.6.0/24 in zone B.

Now the the spaces can be created:

$ juju space create dmz 10.1.1.0/24 10.1.2.0/24
$ juju space create cms 10.1.3.0/24 10.1.4.0/24
$ juju space create database 10.1.5.0/24 10.1.6.0/24

This allows to deploy the services to the three new spaces:

$ juju deploy haproxy --constraints spaces=dmz
$ juju deploy joomla --constraints spaces=cms,^dmz
$ juju deploy mysql --constraints spaces=database,^dmz

Adding additional units will use these constraints for their placement too.

Please note, Juju supports the described syntax but currently ignores all but
the first allowed space in the list. This behavior will change in a future release.
Also, only the EC2 provider supports spaces as described, with support for MaaS
and OpenStack coming soon.
`
