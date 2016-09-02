// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/status"
)

// NewStatusHistoryCommand returns a command that reports the history
// of status changes for the specified unit.
func NewStatusHistoryCommand() cmd.Command {
	return modelcmd.Wrap(&statusHistoryCommand{})
}

type statusHistoryCommand struct {
	modelcmd.ModelCommandBase
	out             cmd.Output
	outputContent   string
	backlogSize     int
	backlogSizeDays int
	backlogDate     string
	isoTime         bool
	entityName      string
	date            time.Time
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
		Name:    "show-status-log",
		Args:    "<entity name>",
		Purpose: "Output past statuses for the specified entity.",
		Doc:     statusHistoryDoc,
	}
}

func (c *statusHistoryCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.outputContent, "type", "unit", "Type of statuses to be displayed [agent|workload|combined|machine|machineInstance|container|containerinstance]")
	f.IntVar(&c.backlogSize, "n", 0, "Returns the last N logs (cannot be combined with --days or --date)")
	f.IntVar(&c.backlogSizeDays, "days", 0, "Returns the logs for the past <days> days (cannot be combined with -n or --date)")
	f.StringVar(&c.backlogDate, "date", "", "Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days)")
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
}

func (c *statusHistoryCommand) Init(args []string) error {
	switch {
	case len(args) > 1:
		return errors.Errorf("unexpected arguments after entity name.")
	case len(args) == 0:
		return errors.Errorf("entity name is missing.")
	default:
		c.entityName = args[0]
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
	emptyDate := c.backlogDate == ""
	emptySize := c.backlogSize == 0
	emptyDays := c.backlogSizeDays == 0
	if emptyDate && emptySize && emptyDays {
		c.backlogSize = 20
	}
	if (!emptyDays && !emptySize) || (!emptyDays && !emptyDate) || (!emptySize && !emptyDate) {
		return errors.Errorf("backlog size, backlog date and backlog days back cannot be specified together")
	}
	if c.backlogDate != "" {
		var err error
		c.date, err = time.Parse("2006-01-02", c.backlogDate)
		if err != nil {
			return errors.Annotate(err, "parsing backlog date")
		}
	}

	kind := status.HistoryKind(c.outputContent)
	if kind.Valid() {
		return nil
	}
	return errors.Errorf("unexpected status type %q", c.outputContent)
}

func (c *statusHistoryCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer apiclient.Close()
	kind := status.HistoryKind(c.outputContent)
	var delta *time.Duration

	if c.backlogSizeDays != 0 {
		t := time.Duration(c.backlogSizeDays*24) * time.Hour
		delta = &t
	}
	filterArgs := status.StatusHistoryFilter{
		Size:  c.backlogSize,
		Delta: delta,
	}
	if !c.date.IsZero() {
		filterArgs.Date = &c.date
	}
	var tag names.Tag
	switch kind {
	case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
		if !names.IsValidUnit(c.entityName) {
			return errors.Errorf("%q is not a valid name for a %s", c.entityName, kind)
		}
		tag = names.NewUnitTag(c.entityName)
	default:
		if names.IsValidMachine(c.entityName) {
			return errors.Errorf("%q is not a valid name for a %s", c.entityName, kind)
		}
		tag = names.NewMachineTag(c.entityName)
	}
	statuses, err := apiclient.StatusHistory(kind, tag, filterArgs)
	historyLen := len(statuses)
	if err != nil {
		if historyLen == 0 {
			return errors.Trace(err)
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	}

	if historyLen == 0 {
		return errors.Errorf("no status history available")
	}

	table := [][]string{{"TIME", "TYPE", "STATUS", "MESSAGE"}}
	lengths := []int{1, 1, 1, 1}

	statuses = statuses.SquashLogs(1)
	statuses = statuses.SquashLogs(2)
	statuses = statuses.SquashLogs(3)
	for _, v := range statuses {
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
