// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Payloads = `
Payloads are defined by charm authors to give more information about the purpose
of running processes and containers for a given machine. This operation information
can help when making descions about your infrastructure.

You can see a list of all of the registered payloads for a system using list-payloads.
The 'juju list-payloads' command itself accepts a filter options similar to the
'juju status' command.

Examples:

  juju list-payloads unit/0
  juju list-payloads machine/0
  juju list-payloads lxd
`
