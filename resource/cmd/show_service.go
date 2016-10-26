// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
)

// ShowServiceClient has the API client methods needed by ShowServiceCommand.
type ShowServiceClient interface {
	// ListResources returns info about resources for applications in the model.
	ListResources(services []string) ([]resource.ServiceResources, error)
	// Close closes the connection.
	Close() error
}

// ShowServiceDeps is a type that contains external functions that ShowService
// depends on to function.
type ShowServiceDeps struct {
	// NewClient returns the value that wraps the API for showing application
	// resources from the server.
	NewClient func(*ShowServiceCommand) (ShowServiceClient, error)
}

// ShowServiceCommand implements the upload command.
type ShowServiceCommand struct {
	modelcmd.ModelCommandBase

	details bool
	deps    ShowServiceDeps
	out     cmd.Output
	target  string
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
		Aliases: []string{"list-resources"},
		Args:    "application-or-unit",
		Purpose: "show the resources for an application or unit",
		Doc: `
This command shows the resources required by and those in use by an existing
application or unit in your model.  When run for an application, it will also show any
updates available for resources from the charmstore.
`,
	}
}

// SetFlags implements cmd.Command.SetFlags.
func (c *ShowServiceCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	const defaultFormat = "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		defaultFormat: FormatSvcTabular,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})

	f.BoolVar(&c.details, "details", false, "show detailed information about resources used by each unit.")
}

// Init implements cmd.Command.Init. It will return an error satisfying
// errors.BadRequest if you give it an incorrect number of arguments.
func (c *ShowServiceCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.NewBadRequest(nil, "missing application name")
	}
	c.target = args[0]
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

	var unit string
	var service string
	if names.IsValidApplication(c.target) {
		service = c.target
	} else {
		service, err = names.UnitApplication(c.target)
		if err != nil {
			return errors.Errorf("%q is neither an application nor a unit", c.target)
		}
		unit = c.target
	}

	vals, err := apiclient.ListResources([]string{service})
	if err != nil {
		return errors.Trace(err)
	}
	if len(vals) != 1 {
		return errors.Errorf("bad data returned from server")
	}
	v := vals[0]

	if unit == "" {
		return c.formatServiceResources(ctx, v)
	}
	return c.formatUnitResources(ctx, unit, service, v)
}

const noResources = "No resources to display."

func (c *ShowServiceCommand) formatServiceResources(ctx *cmd.Context, sr resource.ServiceResources) error {
	if c.details {
		formatted, err := FormatServiceDetails(sr)
		if err != nil {
			return errors.Trace(err)
		}
		if len(formatted.Resources) == 0 && len(formatted.Updates) == 0 {
			ctx.Infof(noResources)
			return nil
		}

		return c.out.Write(ctx, formatted)
	}

	formatted, err := formatServiceResources(sr)
	if err != nil {
		return errors.Trace(err)
	}
	if len(formatted.Resources) == 0 && len(formatted.Updates) == 0 {
		ctx.Infof(noResources)
		return nil
	}
	return c.out.Write(ctx, formatted)
}

func (c *ShowServiceCommand) formatUnitResources(ctx *cmd.Context, unit, service string, sr resource.ServiceResources) error {
	if len(sr.UnitResources) == 0 {
		ctx.Infof(noResources)
		return nil
	}

	if c.details {
		formatted, err := detailedResources(unit, sr)
		if err != nil {
			return errors.Trace(err)
		}
		return c.out.Write(ctx, FormattedUnitDetails(formatted))
	}

	resources, err := unitResources(unit, service, sr)
	if err != nil {
		return errors.Trace(err)
	}

	res := make([]FormattedUnitResource, len(resources))

	for i, r := range resources {
		res[i] = FormattedUnitResource(FormatSvcResource(r))
	}

	return c.out.Write(ctx, res)

}

func unitResources(unit, service string, v resource.ServiceResources) ([]resource.Resource, error) {
	for _, res := range v.UnitResources {
		if res.Tag.Id() == unit {
			return res.Resources, nil
		}
	}
	// TODO(natefinch): we need to differentiate between a unit with no
	// resources and a unit that doesn't exist. This requires a serverside
	// change.
	return nil, nil
}
