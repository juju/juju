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
		Args:    "[--include-data]",
		Purpose: "print status information",
		Doc:     doc,
	}
}

func (c *StatusGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.includeData, "include-data", false, "print all status data")
}

func (c *StatusGetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// StatusInfo is a record of the status information for a unit's workload.
type StatusInfo struct {
	Status string
	Info   string
	Data   map[string]interface{}
}

func (c *StatusGetCommand) Run(ctx *cmd.Context) error {
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
	statusDetails := make(map[string]interface{})
	statusDetails["status"] = unitStatus.Status
	if c.includeData {
		data := make(map[string]interface{})
		for k, v := range unitStatus.Data {
			data[k] = v
		}
		statusDetails["status-data"] = data
		statusDetails["message"] = unitStatus.Info
	}
	c.out.Write(ctx, statusDetails)
	return nil
}
