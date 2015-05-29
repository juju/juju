// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

// StatusGetCommand implements the status-get command.
type StatusGetCommand struct {
	cmd.CommandBase
	ctx         Context
	includeData bool
	serviceWide bool
	out         cmd.Output
}

func NewStatusGetCommand(ctx Context) cmd.Command {
	return &StatusGetCommand{ctx: ctx}
}

func (c *StatusGetCommand) Info() *cmd.Info {
	doc := `
By default, only the status value is printed.
If the --include-data flag is passed, the associated data are printed also.
`
	return &cmd.Info{
		Name:    "status-get",
		Args:    "[--include-data] [--service]",
		Purpose: "print status information",
		Doc:     doc,
	}
}

func (c *StatusGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.includeData, "include-data", false, "print all status data")
	f.BoolVar(&c.serviceWide, "service", false, "print status for all units of this service if this unit is the leader")
}

func (c *StatusGetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// StatusInfo is a record of the status information for a service or a unit's workload.
type StatusInfo struct {
	Tag    string
	Status string
	Info   string
	Data   map[string]interface{}
}

// ServiceStatusInfo holds StatusInfo for a Service and all its Units.
type ServiceStatusInfo struct {
	Service StatusInfo
	Units   []StatusInfo
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

func (c *StatusGetCommand) ServiceStatus(ctx *cmd.Context) error {
	serviceStatus, err := c.ctx.ServiceStatus()
	if err != nil {
		if errors.IsNotImplemented(err) {
			return c.out.Write(ctx, params.StatusUnknown)
		}
		return errors.Annotatef(err, "finding service status")
	}
	if !c.includeData && c.out.Name() == "smart" {
		return c.out.Write(ctx, serviceStatus.Service.Status)
	}
	statusDetails := make(map[string]interface{})
	details := toDetails(serviceStatus.Service, c.includeData)

	units := make(map[string]interface{}, len(serviceStatus.Units))
	for _, unit := range serviceStatus.Units {
		units[unit.Tag] = toDetails(unit, c.includeData)
	}
	details["units"] = units
	statusDetails["service-status"] = details
	c.out.Write(ctx, statusDetails)

	return nil

}

func (c *StatusGetCommand) unitOrServiceStatus(ctx *cmd.Context) error {
	var err error

	if c.serviceWide {
		return c.ServiceStatus(ctx)
	}

	unitStatus, err := c.ctx.UnitStatus()
	if err != nil {
		if errors.IsNotImplemented(err) {
			return c.out.Write(ctx, params.StatusUnknown)
		}
		return errors.Annotatef(err, "finding workload status")
	}
	if !c.includeData && c.out.Name() == "smart" {
		return c.out.Write(ctx, unitStatus.Status)
	}
	c.out.Write(ctx, toDetails(*unitStatus, c.includeData))
	return nil
}

func (c *StatusGetCommand) Run(ctx *cmd.Context) error {
	return c.unitOrServiceStatus(ctx)
}
