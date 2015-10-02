// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Networking = `
Juju provides a set of features allowing the users to have better and
finer-grained control over the networking aspects of the environment
and service deployments in particular. Not all cloud providers support
these enhanced networking features yet, in fact they are currently
supported on AWS only. Support for MaaS and OpenStack is planed and
will be available in future releases of Juju.

Juju network spaces (or just "spaces") represent sets of disjunct
subnets available for running cloud instances, which may span one
or more availability zones ("zones"). Subnets can be part of one
and only one space. All subnets within a space are considered "equal"
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

Existing subnets can be added to the environment using

$ juju subnet add <CIDR>|<subnet-provider-id> <space> [<zone1> <zone2> ...]

Like spaces they can be listed by the subcommand "list". See
also "juju space help" for more information.

The commands "add-machine" and "deploy" allow the specification of a
spaces constraint for the selection of a matching instance. It is done by
adding:

--constraints spaces=<allowedspace1>,<allowedspace2>,^<disallowedspace>

The constraint controls which subnet the new instance will be started in.
This instance has to have distinct IP addresses on any subnet of each allowed
space in the list and none of the subnets associated with one of the disallowed
spaces which are prefixed with a caret ("^").

For more information, see "juju help constraints".

Please note, Juju supports the described syntax but currently ignores all but
the first allowed space in the list. This behavior will change in a future release.
Also, only the EC2 provider supports spaces as described, with support for MaaS
and OpenStack coming soon.
`
