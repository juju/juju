// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/modelcmd"
)

const showMachineCommandDoc = `
Show a specified machine on a model:

juju show-machine <machineID> ...

For example:

juju show-machine 0

or for multiple machines
(the following will display status for machines 1, 2 & 3):

juju show-machine 1 2 3

Default format is in yaml, other formats can be specified
with the "--format" option.  Available formats are yaml,
tabular, and json
`

// NewShowMachineCommand returns a command that shows details on the specified machine[s].
func NewShowMachineCommand() cmd.Command {
	return modelcmd.Wrap(&showMachineCommand{})
}

// showMachineCommand struct holds details on the specified machine[s].
type showMachineCommand struct {
	modelcmd.ModelCommandBase
	out       cmd.Output
	isoTime   bool
	machineId []string
	api       statusAPI
}

// Info implements Command.Info.
func (c *showMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-machine",
		Args:    "<machineID> ...",
		Purpose: "show a machines status",
		Doc:     showMachineCommandDoc,
		Aliases: []string{"show-machines"},
	}
}

// Init captures machineId's to show from CL args.
func (c *showMachineCommand) Init(args []string) error {
	c.machineId = args
	return nil
}

// SetFlags sets utc and format flags based on user specified options.
func (c *showMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": status.FormatMachineTabular,
	})
}

var newAPIClientForShowMachine = func(c *showMachineCommand) (statusAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run implements Command.Run for showMachineCommand.
func (c *showMachineCommand) Run(ctx *cmd.Context) error {
	apiclient, err := newAPIClientForShowMachine(c)
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
	formatted := formatter.MachineFormat(c.machineId)
	return c.out.Write(ctx, formatted)
}
