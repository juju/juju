// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.user")

const userCommandDoc = `
For details on the set of commands used to manage the user accounts and access control in
the Juju environment see:

    juju help users
`

const userCommandPurpose = "manage user accounts and access control"

// UserCommandBase is a helper base structure that has a method to get the
// user manager client.
type UserCommandBase struct {
	envcmd.ControllerCommandBase
}
