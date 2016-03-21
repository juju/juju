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
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
)

// NewStatusHistoryCommand returns a command that reports the history
// of status changes for the specified unit.
func NewStatusHistoryCommand() cmd.Command {
	return modelcmd.Wrap(&statusHistoryCommand{})
}

type statusHistoryCommand struct {
	modelcmd.ModelCommandBase
	out           cmd.Output
	outputContent string
	backlogSize   int
	isoTime       bool
	unitName      string
}

var statusHistoryDoc = `
This command will report the history of status changes for
a given entity.
The statuses are available for the following types.
-type supports:
    juju-unit: will show statuses for the unit's juju agent.
    workload: will show statuses for the unit's workload.
    unit: will show workload and juju agent combined for the specified unit.
    juju-machine: will show statuses for machine's juju agent.
    machine: will show statuses for machines.
    juju-container: will show statuses for the container's juju agent.
    container: will show statuses for containers.
 and sorted by time of occurrence.
 The default is unit.
`

func (c *statusHistoryCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status-history",
		Args:    "[-n N] [--type T] [--utc] <entity name>",
		Purpose: "output past statuses for the passed entity",
		Doc:     statusHistoryDoc,
	}
}

func (c *statusHistoryCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.outputContent, "type", "unit", "type of statuses to be displayed [agent|workload|combined|machine|machineInstance|container|containerinstance].")
	f.IntVar(&c.backlogSize, "n", 20, "size of logs backlog.")
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")
}

func (c *statusHistoryCommand) Init(args []string) error {
	switch {
	case len(args) > 1:
		return errors.Errorf("unexpected arguments after entity name.")
	case len(args) == 0:
		return errors.Errorf("entity name is missing.")
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
	case params.KindUnit, params.KindUnitAgent, params.KindWorkload,
		params.KindMachineInstance, params.KindMachine, params.KindContainer,
		params.KindContainerInstance:
		return nil
	}
	return errors.Errorf("unexpected status type %q", c.outputContent)
}

func (c *statusHistoryCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.NewAPIClient()
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()
	var statuses *params.StatusHistoryResults
	kind := params.HistoryKind(c.outputContent)
	statuses, err = apiclient.StatusHistory(kind, c.unitName, c.backlogSize)
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
