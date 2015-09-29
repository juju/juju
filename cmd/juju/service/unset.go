// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// UnsetCommand sets configuration values of a service back
// to their default.
type UnsetCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	Options     []string
	api         UnsetServiceAPI
}

const unsetDoc = `
Set one or more configuration options for the specified service to their
default. See also the set command to set one or more configuration options for
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

// UnsetServiceAPI defines the methods on the client API
// that the service unset command calls.
type UnsetServiceAPI interface {
	Close() error
	ServiceUnset(service string, options []string) error
}

func (c *UnsetCommand) getAPI() (UnsetServiceAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run resets the configuration of a service.
func (c *UnsetCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()
	return block.ProcessBlockedError(apiclient.ServiceUnset(c.ServiceName, c.Options), block.BlockChange)
}
