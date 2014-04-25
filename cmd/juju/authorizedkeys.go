// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

var authKeysDoc = `
"juju authorized-keys" is used to manage the ssh keys allowed to log on to
nodes in the Juju environment.

`

type AuthorizedKeysCommand struct {
	*cmd.SuperCommand
}

func NewAuthorizedKeysCommand() cmd.Command {
	sshkeyscmd := &AuthorizedKeysCommand{
		SuperCommand: cmd.NewSuperCommand(cmd.SuperCommandParams{
			Name:        "authorized-keys",
			Doc:         authKeysDoc,
			UsagePrefix: "juju",
			Purpose:     "manage authorized ssh keys",
			Aliases:     []string{"authorised-keys"},
		}),
	}
	sshkeyscmd.Register(&AddKeysCommand{})
	sshkeyscmd.Register(&DeleteKeysCommand{})
	sshkeyscmd.Register(&ImportKeysCommand{})
	sshkeyscmd.Register(&ListKeysCommand{})
	return sshkeyscmd
}

func (c *AuthorizedKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SetCommonFlags(f)
}
