// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Placement = `
Placement directives provide users with a means of providing instruction
to the cloud provider on how to allocate a machine. For example, the MAAS
provider can be directed to acquire a particular node by specifying its
hostname.

See provider-specific documentation for details of placement directives
supported by that provider.

Examples:

  # Bootstrap using an instance in the "us-east-1a" EC2 availability zone.
  juju bootstrap --to zone=us-east-1a

  # Acquire the node "host01.maas" and add it to Juju.
  juju add-machine host01.maas

See also:
  juju help add-machine
  juju help bootstrap
`
