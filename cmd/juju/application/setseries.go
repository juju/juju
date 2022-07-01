// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v3/api"
	"github.com/juju/juju/v3/api/client/application"
	jujucmd "github.com/juju/juju/v3/cmd"
	"github.com/juju/juju/v3/cmd/juju/block"
	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/core/series"
)

// NewSetSeriesCommand returns a command which updates the series of
// an application.
func NewSetSeriesCommand() cmd.Command {
	return modelcmd.Wrap(&setSeriesCommand{})
}

// setSeriesAPI defines a subset of the application facade, as required
// by the set-series command.
type setSeriesAPI interface {
	Close() error
	UpdateApplicationSeries(string, string, bool) error
}

// setSeriesCommand is responsible for updating the series of an application or machine.
type setSeriesCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	setSeriesClient setSeriesAPI

	applicationName string
	series          string
}

var setSeriesDoc = `
The specified application's series value will be set within juju. Any subordinates of
the application will also have their series set to the provided value.

This will not change the series of any existing units, rather new units will use
the new series when deployed.

It is recommended to only do this after upgrade-series has been run for machine containing
all existing units of the application.

To ensure correct binaries, run 'juju refresh' before running 'juju add-unit'.

Examples:

Set the series for the ubuntu application to focal

	juju set-series ubuntu focal

See also:
    status
    refresh
    upgrade-series
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
		defer func() { _ = apiRoot.Close() }()
	}

	if c.applicationName != "" {
		if c.setSeriesClient == nil {
			c.setSeriesClient = application.NewClient(apiRoot)
			defer func() { _ = c.setSeriesClient.Close() }()
		}
		err := c.updateApplicationSeries()
		if err == nil {
			// TODO hmlanigan 2022-01-18
			// Remove warning once improvements to develop are made, where by
			// upgrade-series downloads the new charm. Or this command is removed.
			// subordinate
			ctx.Warningf("To ensure the correct charm binaries are installed when add-unit is next called, please first run `juju refresh` for this application and any related subordinates.")
		}
		return err
	}

	// This should never happen...
	return errors.New("no application name specified")
}

func (c *setSeriesCommand) updateApplicationSeries() error {
	err := block.ProcessBlockedError(
		c.setSeriesClient.UpdateApplicationSeries(c.applicationName, c.series, false),
		block.BlockChange)

	return err
}
