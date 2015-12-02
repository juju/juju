// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/resource"
)

// ShowAPI has the API methods needed by ShowCommand.
type ShowAPI interface {
	// ListSpecs lists the resource specs for each of the given services.
	ListSpecs(services ...string) ([]resource.SpecsResult, error)

	// Close closes the client.
	Close() error
}

// ShowCommand implements the show-resources command.
type ShowCommand struct {
	envcmd.EnvCommandBase
	out       cmd.Output
	serviceID string

	newAPIClient func(c *ShowCommand) (ShowAPI, error)
}

// NewShowCommand returns a new command that lists resources defined
// by a charm.
func NewShowCommand(newAPIClient func(c *ShowCommand) (ShowAPI, error)) *ShowCommand {
	cmd := &ShowCommand{
		newAPIClient: newAPIClient,
	}
	return cmd
}

var showDoc = `
This command will report the resources defined by a charm.

The resources are looked up in the service's charm metadata.
`

// Info implements cmd.Command.
func (c *ShowCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-resources",
		Args:    "service-id",
		Purpose: "display the charm-defined resources for a service",
		Doc:     showDoc,
	}
}

// SetFlags implements cmd.Command.
func (c *ShowCommand) SetFlags(f *gnuflag.FlagSet) {
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"tabular": FormatTabular,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
}

// Init implements cmd.Command.
func (c *ShowCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing service ID")
	}
	c.serviceID = args[0]

	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// TODO(ericsnow) Move this to a common place, like cmd/envcmd?
const connectionError = `Unable to connect to environment %q.
Please check your credentials or use 'juju bootstrap' to create a new environment.

Error details:
%v
`

// Run implements cmd.Command.
func (c *ShowCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.newAPIClient(c)
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	results, err := apiclient.ListSpecs(c.serviceID)
	if err != nil {
		return errors.Trace(err)
	}

	var specs []resource.Spec
	if len(results) == 1 {
		// TODO(ericsnow) Handle results[0].Error?
		specs = results[0].Specs
	}

	// Note that we do not worry about c.CompatVersion for show-resources...
	formatter := newSpecListFormatter(specs)
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}
