// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewUpdateSeriesCommand returns a command which updates the series of
// an application or machine.
func NewUpdateSeriesCommand() cmd.Command {
	return modelcmd.Wrap(&updateSeriesCommand{})
}

// updateApplicationAPI defines a subset of the application facade, as required
// by the update-series command.
type updateApplicationSeriesAPI interface {
	BestAPIVersion() int
	Close() error
	UpdateApplicationSeries(string, string, bool) error
}

type updateMachineSeriesAPI interface {
	BestAPIVersion() int
	Close() error
	UpdateMachineSeries(string, string, bool) error
}

// updateSeriesCommand is responsible for updating the series of an application or machine.
type updateSeriesCommand struct {
	modelcmd.ModelCommandBase

	updateApplicationSeriesClient updateApplicationSeriesAPI
	updateMachineSeriesClient     updateMachineSeriesAPI

	applicationName string
	force           bool
	machineNumber   string
	series          string
}

var updateSeriesDoc = `
When no flags are set, an application or machines series value with be updated
within juju.

The update is disallowed unless the --force flag is used if the requested
series is not explicitly supported by the application's charm and all
subordinates, as well as any other charms which may be deployed to the same
machine.

In the case of updating a machine's series, the --force option should be used
with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior. Alternately, if the requested series supported in
later revisions of the charm, upgrade-charm can run beforehand.

Examples:
	juju update-series <application> <series>
	juju update-series <application> <series> --force
	juju update-series <machine> <series>
	juju update-series <machine> <series> --force

See also:
    machines
    status
    upgrade-charm
`

func (c *updateSeriesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update-series",
		Args:    "[<application>|<machine>] <series>",
		Purpose: "Update an application or machine's series.",
		Doc:     updateSeriesDoc,
	}
}

func (c *updateSeriesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Update even if the series is not supported by the charm and/or related subordinate charms.")
}

// Init implements cmd.Command.
func (c *updateSeriesCommand) Init(args []string) error {
	switch len(args) {
	case 2:
		if names.IsValidApplication(args[0]) {
			c.applicationName = args[0]
		} else if names.IsValidMachine(args[0]) {
			c.machineNumber = args[0]
		} else {
			return errors.Errorf("invalid application or machine name %q", args[0])
		}
		c.series = args[1]
	case 1:
		if names.IsValidMachine(args[0]) {
			return errors.Errorf("no series specified")
		}
		return errors.Errorf("no application name or no series specified")
	case 0:
		return errors.Errorf("no arguments specified")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

// Run implements cmd.Run.
func (c *updateSeriesCommand) Run(ctx *cmd.Context) error {
	var apiRoot api.Connection
	if c.updateMachineSeriesClient == nil && c.updateApplicationSeriesClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		defer apiRoot.Close()
	}

	if c.machineNumber != "" {
		if c.updateMachineSeriesClient == nil {
			c.updateMachineSeriesClient = machinemanager.NewClient(apiRoot)
			defer c.updateMachineSeriesClient.Close()
		}
		if c.updateMachineSeriesClient.BestAPIVersion() < 4 {
			return errors.New("updating the machine series is not supported by this API server")
		}
		return c.updateMachineSeries()
	}

	if c.applicationName != "" {
		if c.updateApplicationSeriesClient == nil {
			c.updateApplicationSeriesClient = application.NewClient(apiRoot)
			defer c.updateApplicationSeriesClient.Close()
		}
		if c.updateApplicationSeriesClient.BestAPIVersion() < 5 {
			return errors.New("updating the application series is not supported by this API server")
		}
		return c.updateApplicationSeries()
	}

	// This should never happen...
	return errors.New("no application nor machine name specified")
}

func (c *updateSeriesCommand) updateApplicationSeries() error {
	err := block.ProcessBlockedError(
		c.updateApplicationSeriesClient.UpdateApplicationSeries(c.applicationName, c.series, c.force),
		block.BlockChange)

	if params.IsCodeIncompatibleSeries(err) {
		return errors.Errorf("%v. Use --force to update the series anyway.", err)
	}
	return err
}

func (c *updateSeriesCommand) updateMachineSeries() error {
	err := block.ProcessBlockedError(
		c.updateMachineSeriesClient.UpdateMachineSeries(c.machineNumber, c.series, c.force),
		block.BlockChange)

	if params.IsCodeIncompatibleSeries(err) {
		return errors.Errorf("%v. Use --force to update the series anyway.", err)
	}
	return err
}
