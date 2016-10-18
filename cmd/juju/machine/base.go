// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/modelcmd"
)

// statusAPI defines the API methods for the machines and show-machine commands.
type statusAPI interface {
	Status(pattern []string) (*params.FullStatus, error)
	Close() error
}

// baseMachineCommand provides access to information about machines in a model.
type baselistMachinesCommand struct {
	modelcmd.ModelCommandBase
	out           cmd.Output
	isoTime       bool
	api           statusAPI
	machineIds    []string
	defaultFormat string
	color         bool
}

// SetFlags sets utc and format flags based on user specified options.
func (c *baselistMachinesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	f.BoolVar(&c.color, "color", false, "Force use of ANSI color codes")
	c.out.AddFlags(f, c.defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.tabular,
	})
}

var newAPIClientForMachines = func(c *baselistMachinesCommand) (statusAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run implements Command.Run for baseMachinesCommand.
func (c *baselistMachinesCommand) Run(ctx *cmd.Context) error {
	apiclient, err := newAPIClientForMachines(c)
	if err != nil {
		return errors.Trace(err)
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
	formatted := formatter.MachineFormat(c.machineIds)
	return c.out.Write(ctx, formatted)
}

func (c *baselistMachinesCommand) tabular(writer io.Writer, value interface{}) error {
	return status.FormatMachineTabular(writer, c.color, value)
}
