// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.user")

const userCommandDoc = `
"juju user" is used to manage the user accounts and access control in
the Juju environment.

See Also:
    juju help users
`

const userCommandPurpose = "manage user accounts and access control"

// NewSuperCommand creates the user supercommand and registers the subcommands
// that it supports.
func NewSuperCommand() cmd.Command {
	usercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "user",
		Doc:         userCommandDoc,
		UsagePrefix: "juju",
		Purpose:     userCommandPurpose,
	})
	usercmd.Register(envcmd.WrapSystem(&AddCommand{}))
	usercmd.Register(envcmd.WrapSystem(&ChangePasswordCommand{}))
	usercmd.Register(envcmd.WrapSystem(&CredentialsCommand{}))
	usercmd.Register(envcmd.WrapSystem(&InfoCommand{}))
	usercmd.Register(envcmd.WrapSystem(&DisableCommand{}))
	usercmd.Register(envcmd.WrapSystem(&EnableCommand{}))
	usercmd.Register(envcmd.WrapSystem(&ListCommand{}))
	return usercmd
}

// UserCommandBase is a helper base structure that has a method to get the
// user manager client.
type UserCommandBase struct {
	envcmd.SysCommandBase
}
