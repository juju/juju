// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Networking = `
Beside the standard networking of the cloud providers Juju comes up with
a software defined networking. This way better physical isolation as
well as redundancy for high availability can be established. To do so
Juju introduced spaces.

Spaces are sets of disjunct networks orthogonal to zones of the cloud
provider. The subnets inside one space are routable to one another
without any firewalls. Opposite to this all connections between different
spaces are assumed to go through firewalls or other filters. Here the
needed ports have to be required by the charms. This way Juju provides
a fine-grained access control to the deployed services.

As an example an environment with three areas of trust shall be setup:

- the "dmz" space as the public interface to the Internet,
- the "cms" space for a content management, and
- the "database" space as backend.

Here in the "dmz" space HAProxy runs. It is proxying Joomla the instances
in the "cms" space. The backend MySQL for Joomla is running in the "database"
space. Both backend spaces only have private subnets and cannot be directly
accessed from the internet. Opposite to this the "dmz" space additionally
provides public addresses. The firewall rules between "dmz" and "cms"
allow the proxying as well as the rules between "cms" and "database"
spaces allow Joomla to use the database. Future extensions of the environment
may use the same database but new spaces for other application parts.
So it is ensured that while these parts can share the database but
not directly influence each other.

Due to the ability of spaces to span multiple zones services can be
distributed to these zones. This way high available environments can be
realized.

Spaces are created like this:

$ juju space create <name> [ <CIDR1> <CIDR2> ... ] [--private|--public]

It can be done without initial subnets, these can be added later by
calling:

$ juju space update <name> <CIDR1> [ <CIDR2> ... ]

Other space subcommands are "list", "rename", and "remove". Subnets
are created and added to spaces like this:

$ juju subnet create <CIDR> <space> <zone1> [<zone2> ...] [--vlan-tag <integer>]
[--private|--public]

Additionally existing subnets can be added using

$ juju subnet add <CIDR>|<subnet-provider-id> <space> [<zone1> <zone2> ...]

Like spaces they can be listed by the subcommand "list" and removed
by "remove".

The commands "add-machine" and "deploy" allow the specification of a
spaces constraint for the selection of a matching instance. It is done by
adding

--constraints spaces=<allowedspace1>,<allowedspace2>,^<disallowedspace>

The constraint controls which instance is chosen for the new machine or
service unit. This instance has to have distinct IP addresses on any subnet
of each allowed space in the list and none of the subnets associated with one
of the disallowed spaces which are prefixed with a carret.

Currently MaaS and AWS are supported providers for this networking model.
`
