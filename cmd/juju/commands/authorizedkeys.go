// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/keymanager"
	"github.com/juju/juju/cmd/envcmd"
)

var authKeysDoc = `
"juju authorized-keys" is used to manage the ssh keys allowed to log on to
nodes in the Juju environment.

`

type AuthorizedKeysCommand struct {
	*cmd.SuperCommand
}

type AuthorizedKeysBase struct {
	envcmd.EnvCommandBase
}

// NewKeyManagerClient returns a keymanager client for the root api endpoint
// that the environment command returns.
func (c *AuthorizedKeysBase) NewKeyManagerClient() (*keymanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return keymanager.NewClient(root), nil
}

func NewAuthorizedKeysCommand() cmd.Command {
	sshkeyscmd := &AuthorizedKeysCommand{
		SuperCommand: cmd.NewSuperCommand(cmd.SuperCommandParams{
			Name:        "authorized-keys",
			Doc:         authKeysDoc,
			UsagePrefix: "juju",
			Purpose:     "manage authorised ssh keys",
			Aliases:     []string{"authorised-keys"},
		}),
	}
	sshkeyscmd.Register(envcmd.Wrap(&AddKeysCommand{}))
	sshkeyscmd.Register(envcmd.Wrap(&DeleteKeysCommand{}))
	sshkeyscmd.Register(envcmd.Wrap(&ImportKeysCommand{}))
	sshkeyscmd.Register(envcmd.Wrap(&ListKeysCommand{}))
	return sshkeyscmd
}

func (c *AuthorizedKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SetCommonFlags(f)
}
