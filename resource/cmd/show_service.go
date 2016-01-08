// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/resource"
)

// ShowServiceClient has the API client methods needed by ShowServiceCommand.
type ShowServiceClient interface {
	// ListResources returns info about resources for services in the model.
	ListResources(services []string) ([][]resource.Resource, error)
	// Close closes the connection.
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
	envcmd.EnvCommandBase

	deps    ShowServiceDeps
	out     cmd.Output
	service string
}

// NewShowServiceCommand returns a new command that lists resources defined
// by a charm.
func NewShowServiceCommand(deps ShowServiceDeps) *ShowServiceCommand {
	return &ShowServiceCommand{deps: deps}
}

// Info implements cmd.Command.Info.
func (c *ShowServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resources",
		Args:    "service",
		Purpose: "show the resources for a service",
		Doc: `
This command shows the resources required by and those in use by an existing service in your model.
`,
	}
}

// SetFlags implements cmd.Command.SetFlags.
func (c *ShowServiceCommand) SetFlags(f *gnuflag.FlagSet) {
	const defaultFlag = "tabular"
	c.out.AddFlags(f, defaultFlag, map[string]cmd.Formatter{
		defaultFlag: FormatSvcTabular,
		"yaml":      cmd.FormatYaml,
		"json":      cmd.FormatJson,
	})
}

// Init implements cmd.Command.Init. It will return an error satisfying
// errors.BadRequest if you give it an incorrect number of arguments.
func (c *ShowServiceCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.NewBadRequest(nil, "missing service name")
	}
	c.service = args[0]
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.NewBadRequest(err, "")
	}
	return nil
}

// Run implements cmd.Command.Run.
func (c *ShowServiceCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.deps.NewClient(c)
	if err != nil {
		return errors.Annotatef(err, "can't connect to %s", c.ConnectionName())
	}
	defer apiclient.Close()

	vals, err := apiclient.ListResources([]string{c.service})
	if err != nil {
		return errors.Trace(err)
	}
	if len(vals) != 1 {
		return errors.Errorf("bad data returned from server")
	}
	v := vals[0]
	res := make([]FormattedSvcResource, len(v))

	for i := range v {
		res[i] = FormatSvcResource(v[i])
	}

	return c.out.Write(ctx, res)
}
