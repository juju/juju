// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/utils/series"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewSetSeriesCommand returns a command which updates the series of
// an application.
func NewSetSeriesCommand() cmd.Command {
	return modelcmd.Wrap(&setSeriesCommand{})
}

// setSeriesAPI defines a subset of the application facade, as required
// by the set-series command.
type setSeriesAPI interface {
	BestAPIVersion() int
	Close() error
	UpdateApplicationSeries(string, string, bool) error
}

// setSeriesCommand is responsible for updating the series of an application or machine.
type setSeriesCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	setSeriesClient setSeriesAPI

	applicationName string
	force           bool
	series          string
}

var setSeriesDoc = `
When no options are set, an application series value will be set within juju.

The update is disallowed unless the --force option is used if the requested
series is not explicitly supported by the application's charm and all
subordinates, as well as any other charms which may be deployed to the same
machine.

Examples:
	juju set-series <application> <series>
	juju set-series <application> <series> --force

See also:
    status
    upgrade-charm
`

func (c *setSeriesCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-series",
		Args:    "<application> <series>",
		Purpose: "Set an application's series.",
		Doc:     setSeriesDoc,
	})
}

func (c *setSeriesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Set even if the series is not supported by the charm and/or related subordinate charms.")
}

// Init implements cmd.Command.
func (c *setSeriesCommand) Init(args []string) error {
	switch len(args) {
	case 2:
		if names.IsValidApplication(args[0]) {
			c.applicationName = args[0]
		} else {
			return errors.Errorf("invalid application name %q", args[0])
		}
		if _, err := series.GetOSFromSeries(args[1]); err != nil {
			return errors.Errorf("invalid series %q", args[1])
		}
		c.series = args[1]
	case 1:
		if _, err := series.GetOSFromSeries(args[0]); err != nil {
			return errors.Errorf("no series specified")
		} else {
			return errors.Errorf("no application name")
		}
	case 0:
		return errors.Errorf("application name and series required")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

// Run implements cmd.Run.
func (c *setSeriesCommand) Run(ctx *cmd.Context) error {
	var apiRoot api.Connection
	if c.setSeriesClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		defer apiRoot.Close()
	}

	if c.applicationName != "" {
		if c.setSeriesClient == nil {
			c.setSeriesClient = application.NewClient(apiRoot)
			defer c.setSeriesClient.Close()
		}
		if c.setSeriesClient.BestAPIVersion() < 5 {
			return errors.New("setting the application series is not supported by this API server")
		}
		return c.updateApplicationSeries()
	}

	// This should never happen...
	return errors.New("no application nor machine name specified")
}

func (c *setSeriesCommand) updateApplicationSeries() error {
	err := block.ProcessBlockedError(
		c.setSeriesClient.UpdateApplicationSeries(c.applicationName, c.series, c.force),
		block.BlockChange)

	if params.IsCodeIncompatibleSeries(err) {
		return errors.Errorf("%v. Use --force to set the series anyway.", err)
	}
	return err
}
