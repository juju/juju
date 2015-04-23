// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"launchpad.net/gnuflag"
)

type StatusHistoryCommand struct {
	envcmd.EnvCommandBase
	out           cmd.Output
	outputContent string
	backlogSize   int
	isoTime       bool
	unitName      string
}

var statusHistoryDoc = `
This command will report the history of status changes for
a given unit up to 20 changes in the past.
The statuses for the unit workload or agent are available.
-type supports:
    agent: will show last 20 statuses for the unit's agent
    workload: will show last 20 statuses for the unit's workload
    combined: will 20 entries for agent and workload statuses combined
 and sorted by time of occurence.
`

func (c *StatusHistoryCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status-history",
		Args:    "[-n N] <unit>",
		Purpose: "output last 20 statuses for a unit",
		Doc:     statusHistoryDoc,
	}
}

func (c *StatusHistoryCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.outputContent, "type", "combined", "type of statuses to be displayed [agent|workload|combined].")
	f.IntVar(&c.backlogSize, "n", 20, "size of logs backlog.")
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")
}

func (c *StatusHistoryCommand) Init(args []string) error {
	switch {
	case len(args) > 1:
		return errors.Errorf("unexpected arguments after unit name.")
	case len(args) == 0:
		return errors.Errorf("unit name is missing.")
	default:
		c.unitName = args[0]
	}
	return nil
}

func (c *StatusHistoryCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.NewAPIClient()
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()
	var statuses *api.UnitStatuses
	switch c.outputContent {
	case "combined", "agent", "workload":
		statuses, err = apiclient.UnitStatusHistory(c.outputContent, c.unitName, c.backlogSize)
	default:
		return errors.Errorf("unexpected status type %q", c.outputContent)
	}
	if err != nil {
		if len(statuses.Statuses) == 0 {
			return errors.Trace(err)
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	} else if len(statuses.Statuses) == 0 {
		return errors.Errorf("unable to obtain status history")
	}
	table := [][]string{{"TIME", "TYPE", "STATUS", "MESSAGE"}}
	lengths := []int{1, 1, 1, 1}
	for _, v := range statuses.Statuses {
		fields := []string{c.formatTime(v.Since), v.Kind, string(v.Status), v.Info}
		for k, v := range fields {
			if len(v) > lengths[k] {
				lengths[k] = len(v)
			}
		}
		table = append(table, fields)
	}
	f := fmt.Sprintf("%%-%ds\t%%-%ds\t%%-%ds\t%%-%ds\n", lengths[0], lengths[1], lengths[2], lengths[3])
	for _, v := range table {
		fmt.Printf(f, v[0], v[1], v[2], v[3])
	}
	return nil
}

func (c *StatusHistoryCommand) formatTime(t *time.Time) string {
	if c.isoTime {
		// If requested, use ISO time format
		return t.Format(time.RFC3339)
	} else {
		// Otherwise use local time.
		return t.Local().Format("02 Jan 2006 15:04:05 MST")
	}
}
