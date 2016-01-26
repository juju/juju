// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/status"
)

const showMachineCommandDoc = ` 
Show a specified machine on a model:

juju show-machine <machineID[s]>

For example:

juju show-machine 0

or for multiple machines
(the following will display status for machines 1, 2 & 3):

juju show-machine 1 2 3

Default format is in yaml, other formats can be specified
with the "--format" option.  Available formats are yaml,
tabular, and json
`

func NewShowMachineCommand() cmd.Command {
	return envcmd.Wrap(&showMachineCommand{})
}

type showMachineCommand struct {
	envcmd.EnvCommandBase
	out       cmd.Output
	isoTime   bool
	machineId []string
	api       statusAPI
}

func (c *showMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-machine",
		Args:    "<machineID[s]> ...",
		Purpose: "show a machines status",
		Doc:     showMachineCommandDoc,
		Aliases: []string{"show-machines"},
	}
}

func (c *showMachineCommand) Init(args []string) error {
	c.machineId = args
	return nil
}

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

	formatter := status.NewStatusFormatter(fullStatus, 0, c.isoTime)
	formatted := formatter.Machineformat(c.machineId)
	return c.out.Write(ctx, formatted)
}
