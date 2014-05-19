// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
)

// UnsetCommand sets configuration values of a service back
// to their default.
type UnsetCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	Options     []string
}

const unsetDoc = `
Set one or more configuration options for the specified service to their
default. See also the set commmand to set one or more configuration options for
a specified service.
`

func (c *UnsetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset",
		Args:    "<service> name ...",
		Purpose: "set service config options back to their default",
		Doc:     unsetDoc,
	}
}

func (c *UnsetCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	c.Options = args[1:]
	if len(c.Options) == 0 {
		return errors.New("no configuration options specified")
	}
	return nil
}

// Run resets the configuration of a service.
func (c *UnsetCommand) Run(ctx *cmd.Context) error {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()
	return apiclient.ServiceUnset(c.ServiceName, c.Options)
}
