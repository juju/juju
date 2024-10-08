// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/status"
)

// StatusGetCommand implements the status-get command.
type StatusGetCommand struct {
	cmd.CommandBase
	ctx             Context
	includeData     bool
	applicationWide bool
	out             cmd.Output
}

func NewStatusGetCommand(ctx Context) (cmd.Command, error) {
	return &StatusGetCommand{ctx: ctx}, nil
}

func (c *StatusGetCommand) Info() *cmd.Info {
	doc := `
By default, only the status value is printed.
If the --include-data flag is passed, the associated data are printed also.

Further details:
status-get allows charms to query the current workload status.

Without arguments, it just prints the status code e.g. ‘maintenance’.
With --include-data specified, it prints YAML which contains the status
value plus any data associated with the status.

Include the --application option to get the overall status for the application, rather than an individual unit.
`
	examples := `
    # Access the unit’s status:
    status-get
    status-get --include-data

    # Access the application’s status:
    status-get --application
`
	return jujucmd.Info(&cmd.Info{
		Name:     "status-get",
		Args:     "[--include-data] [--application]",
		Purpose:  "Print status information.",
		Doc:      doc,
		Examples: examples,
	})
}

func (c *StatusGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.BoolVar(&c.includeData, "include-data", false, "print all status data")
	f.BoolVar(&c.applicationWide, "application", false, "print status for all units of this application if this unit is the leader")
}

func (c *StatusGetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// StatusInfo is a record of the status information for a application or a unit's workload.
type StatusInfo struct {
	Tag    string
	Status string
	Info   string
	Data   map[string]interface{}
}

// ApplicationStatusInfo holds StatusInfo for an Application and all its Units.
type ApplicationStatusInfo struct {
	Application StatusInfo
	Units       []StatusInfo
}

func toDetails(info StatusInfo, includeData bool) map[string]interface{} {
	details := make(map[string]interface{})
	details["status"] = info.Status
	if includeData {
		data := make(map[string]interface{})
		for k, v := range info.Data {
			data[k] = v
		}
		details["status-data"] = data
		details["message"] = info.Info
	}
	return details
}

func (c *StatusGetCommand) ApplicationStatus(ctx *cmd.Context) error {
	applicationStatus, err := c.ctx.ApplicationStatus(ctx)
	if err != nil {
		if errors.Is(err, errors.NotImplemented) {
			return c.out.Write(ctx, status.Unknown)
		}
		return errors.Annotatef(err, "finding application status")
	}
	if !c.includeData && c.out.Name() == "smart" {
		return c.out.Write(ctx, applicationStatus.Application.Status)
	}
	statusDetails := make(map[string]interface{})
	details := toDetails(applicationStatus.Application, c.includeData)

	units := make(map[string]interface{}, len(applicationStatus.Units))
	for _, unit := range applicationStatus.Units {
		// NOTE: unit.Tag is a unit name, not a unit tag.
		units[unit.Tag] = toDetails(unit, c.includeData)
	}
	details["units"] = units
	statusDetails["application-status"] = details
	_ = c.out.Write(ctx, statusDetails)

	return nil

}

func (c *StatusGetCommand) unitOrApplicationStatus(ctx *cmd.Context) error {
	var err error

	if c.applicationWide {
		return c.ApplicationStatus(ctx)
	}

	unitStatus, err := c.ctx.UnitStatus(ctx)
	if err != nil {
		if errors.Is(err, errors.NotImplemented) {
			return c.out.Write(ctx, status.Unknown)
		}
		return errors.Annotatef(err, "finding workload status")
	}
	if !c.includeData && c.out.Name() == "smart" {
		return c.out.Write(ctx, unitStatus.Status)
	}
	_ = c.out.Write(ctx, toDetails(*unitStatus, c.includeData))
	return nil
}

func (c *StatusGetCommand) Run(ctx *cmd.Context) error {
	return c.unitOrApplicationStatus(ctx)
}
