// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"os"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/juju/osenv"
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
a given unit.
The statuses for the unit workload and/or agent are available.
-type supports:
    agent: will show statuses for the unit's agent
    workload: will show statuses for the unit's workload
    combined: will show agent and workload statuses combined
 and sorted by time of occurrence.
`

func (c *StatusHistoryCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status-history",
		Args:    "[-n N] <unit>",
		Purpose: "output past statuses for a unit",
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
	// If use of ISO time not specified on command line,
	// check env var.
	if !c.isoTime {
		var err error
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
	kind := params.HistoryKind(c.outputContent)
	switch kind {
	case params.KindCombined, params.KindAgent, params.KindWorkload:
		return nil

	}
	return errors.Errorf("unexpected status type %q", c.outputContent)
}

func (c *StatusHistoryCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.NewAPIClient()
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()
	var statuses *params.UnitStatusHistory
	kind := params.HistoryKind(c.outputContent)
	statuses, err = apiclient.UnitStatusHistory(kind, c.unitName, c.backlogSize)
	if err != nil {
		if len(statuses.Statuses) == 0 {
			return errors.Trace(err)
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	} else if len(statuses.Statuses) == 0 {
		return errors.Errorf("no status history available")
	}
	table := [][]string{{"TIME", "TYPE", "STATUS", "MESSAGE"}}
	lengths := []int{1, 1, 1, 1}
	for _, v := range statuses.Statuses {
		fields := []string{common.FormatTime(v.Since, c.isoTime), string(v.Kind), string(v.Status), v.Info}
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
