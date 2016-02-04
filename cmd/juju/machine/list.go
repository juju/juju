// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/modelcmd"
)

const listMachinesCommandDoc = `
List all the machines in a juju model.
Default display is in tabular format with the following sections:
ID, STATE, DNS, INS-ID, SERIES, AZ

Note: AZ above is the cloud region's availability zone.
`

// statusAPI defines the API methods for the list-mahines and show-machine commands.
type statusAPI interface {
	Status(pattern []string) (*params.FullStatus, error)
	Close() error
}

// NewListMachineCommand returns a command that lists the machines in a model.
func NewListMachinesCommand() cmd.Command {
	return modelcmd.Wrap(&listMachinesCommand{})
}

// listMachineCommand holds infomation about machines in a model.
type listMachinesCommand struct {
	modelcmd.ModelCommandBase
	out     cmd.Output
	isoTime bool
	api     statusAPI
}

// Info implements Command.Info.
func (c *listMachinesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-machines",
		Purpose: "list machines in a model",
		Doc:     listMachinesCommandDoc,
		Aliases: []string{"machines", "machine", "list-machine"},
	}
}

// Init ensures the list-machines Command does not take arguments.
func (c *listMachinesCommand) Init(args []string) (err error) {
	if args != nil {
		return errors.Errorf("The list-machines command does not take any arguments")
	}
	return nil
}

// SetFlags sets utc and format flags based on user specified options.
func (c *listMachinesCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": status.FormatMachineTabular,
	})
}

var connectionError = `Unable to connect to model %q.
Please check your credentials or use 'juju bootstrap' to create a new model.

Error details:
%v
`
var newAPIClientForListMachines = func(c *listMachinesCommand) (statusAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run implements Command.Run for listMachineCommand.
func (c *listMachinesCommand) Run(ctx *cmd.Context) error {
	apiclient, err := newAPIClientForListMachines(c)
	if err != nil {
		return errors.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	fullStatus, err := apiclient.Status(nil)
	if err != nil {
		if fullStatus == nil {
			// Status call completely failed, there is nothing to report
			return err
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	} else if fullStatus == nil {
		return errors.Errorf("unable to obtain the current status")
	}

	formatter := status.NewStatusFormatter(fullStatus, c.isoTime)
	formatted := formatter.MachineFormat(nil)
	return c.out.Write(ctx, formatted)
}
