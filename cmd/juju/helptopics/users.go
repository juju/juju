// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Users = `

Juju has understanding of two different types of users:
local users, those stored in the database along side the environments and
entities in those environments; and remote users, those whose authenticiation
is managed by an external service and reserved for future use.

When a Juju System is bootstrapped, an initial user is created when the intial
environment is created. This user is considered the administrator for the Juju
System. This user is the only user able to create other users until Juju has
full fine grained role based permissions.

All user managment functionality is managed though the 'juju user' collection
of commands.

The primary user commands are used by the admin users to create users and
disable or reenable login access.

The change-password command can be used by any user to change their own
password, or, for admins, the command can change another user's password and
generate a new credentials file for them.

The credentials command gives any use the ability to export the credentails
they are using to access an environment to a file that they can use elsewhere
to login to the same Juju System.

See Also:
    juju help system
    juju help user
`
