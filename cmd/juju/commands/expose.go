// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

func newExposeCommand() cmd.Command {
	return envcmd.Wrap(&exposeCommand{})
}

// exposeCommand is responsible exposing services.
type exposeCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
}

var jujuExposeHelp = `
Adjusts firewall rules and similar security mechanisms of the provider, to
allow the service to be accessed on its public address.

`

func (c *exposeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "expose",
		Args:    "<service>",
		Purpose: "expose a service",
		Doc:     jujuExposeHelp,
	}
}

func (c *exposeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run changes the juju-managed firewall to expose any
// ports that were also explicitly marked by units as open.
func (c *exposeCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.ServiceExpose(c.ServiceName), block.BlockChange)
}
