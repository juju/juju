// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	//"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	//"launchpad.net/juju-core/cmd/envcmd"
)

type UserCommand struct {
	*cmd.SuperCommand
}

const userCommandDoc = `
"juju user" is used to manage the user accounts and access control in
the Juju environment.

`
const userCommandPurpose = "manage user accounts and access control"

func NewUserCommand() cmd.Command {
	usercmd := &UserCommand{
		SuperCommand: cmd.NewSuperCommand(cmd.SuperCommandParams{
			Name:        "user",
			Doc:         userCommandDoc,
			UsagePrefix: "juju",
			Purpose:     userCommandPurpose,
		}),
	}
	//usercmd.Register(envcmd.Wrap(&AddKeysCommand{}))
	return usercmd
}
