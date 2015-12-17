// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/envcmd"
)

// ShowServiceClient has the API client methods needed by ShowServiceCommand.
type ShowServiceClient interface {
	ShowService(service string) error
	Close() error
}

// ShowServiceDeps is a type that contains external functions that ShowService
// depends on to function.
type ShowServiceDeps struct {
	// NewClient returns the value that wraps the API for showing service
	// resources from the server.
	NewClient func(*ShowServiceCommand) (ShowServiceClient, error)
}

// ShowServiceCommand implements the upload command.
type ShowServiceCommand struct {
	deps ShowServiceDeps
	envcmd.EnvCommandBase
	service string
}

// NewShowServiceCommand returns a new command that lists resources defined
// by a charm.
func NewShowServiceCommand(deps ShowServiceDeps) *ShowServiceCommand {
	return &ShowServiceCommand{deps: deps}
}

// Info implements cmd.Command.Info
func (c *ShowServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-service-resources",
		Args:    "service",
		Purpose: "show the resources for a service",
		Doc: `
This command shows the resources required by and those in use by an existing service in your model.
`,
	}
}

// Init implements cmd.Command.Init. It will return an error satisfying
// errors.BadRequest if you give it an incorrect number of arguments.
func (c *ShowServiceCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.BadRequestf("missing service name")
	case 1:
		// good!
	default:
		return errors.BadRequestf("too many arguments")
	}

	c.service = args[0]

	return nil
}

// Run implements cmd.Command.Run.
func (c *ShowServiceCommand) Run(*cmd.Context) error {
	apiclient, err := c.deps.NewClient(c)
	if err != nil {
		return errors.Annotatef(err, "can't connect to %s", c.ConnectionName())
	}
	defer apiclient.Close()

}
