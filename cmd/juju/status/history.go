// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/osenv"
)

// TODO(peritto666) - add tests

// NewStatusHistoryCommand returns a command that reports the history
// of status changes for the specified unit.
func NewStatusHistoryCommand() cmd.Command {
	return modelcmd.Wrap(&statusHistoryCommand{})
}

// HistoryAPI is the API surface for the show-status-log command.
type HistoryAPI interface {
	StatusHistory(kind status.HistoryKind, tag names.Tag, filter status.StatusHistoryFilter) (status.History, error)
	Close() error
}

type statusHistoryCommand struct {
	modelcmd.ModelCommandBase
	api                  HistoryAPI
	out                  cmd.Output
	outputContent        string
	backlogSize          int
	backlogSizeDays      int
	backlogDate          string
	isoTime              bool
	entityName           string
	date                 time.Time
	includeStatusUpdates bool
}

var statusHistoryDoc = fmt.Sprintf(`
This command will report the history of status changes for
a given entity.
The statuses are available for the following types.
-type supports:
%v
 and sorted by time of occurrence.
 The default is unit.
`, supportedHistoryKindDescs())

func (c *statusHistoryCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-status-log",
		Args:    "<entity name>",
		Purpose: "Output past statuses for the specified entity.",
		Doc:     statusHistoryDoc,
	})
}

func supportedHistoryKindTypes() string {
	supported := set.NewStrings()
	for k := range status.AllHistoryKind() {
		supported.Add(string(k))
	}
	return strings.Join(supported.SortedValues(), "|")
}

func supportedHistoryKindDescs() string {
	types := status.AllHistoryKind()
	supported := set.NewStrings()
	for k := range types {
		supported.Add(string(k))
	}
	all := ""
	for _, k := range supported.SortedValues() {
		all += fmt.Sprintf("    %v:  %v\n", k, types[status.HistoryKind(k)])
	}
	return all
}

func (c *statusHistoryCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.outputContent, "type", "unit", fmt.Sprintf("Type of statuses to be displayed [%v]", supportedHistoryKindTypes()))
	f.IntVar(&c.backlogSize, "n", 0, "Returns the last N logs (cannot be combined with --days or --date)")
	f.IntVar(&c.backlogSizeDays, "days", 0, "Returns the logs for the past <days> days (cannot be combined with -n or --date)")
	f.StringVar(&c.backlogDate, "from-date", "", "Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days)")
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	// TODO (anastasiamac 2018-04-11) Remove at the next major release, say Juju 2.5+ or Juju 3.x.
	// the functionality is no longer there since a fix for lp#1530840
	f.BoolVar(&c.includeStatusUpdates, "include-status-updates", false, "Deprecated, has no effect for 2.3+ controllers: Include update status hook messages in the returned logs")
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

const runningHookMSG = "running update-status hook"

func (c *statusHistoryCommand) getAPI() (HistoryAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *statusHistoryCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
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
	if !c.includeStatusUpdates {
		filterArgs.Exclude = set.NewStrings(runningHookMSG)
	}

	if !c.date.IsZero() {
		filterArgs.FromDate = &c.date
	}
	var tag names.Tag
	switch kind {
	case status.KindUnit, status.KindWorkload, status.KindUnitAgent:
		if !names.IsValidUnit(c.entityName) {
			return errors.Errorf("%q is not a valid name for a %s", c.entityName, kind)
		}
		tag = names.NewUnitTag(c.entityName)
	default:
		if !names.IsValidMachine(c.entityName) {
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

	c.writeTabular(ctx.Stdout, statuses)
	return nil
}

func (c *statusHistoryCommand) writeTabular(writer io.Writer, statuses status.History) {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	w.Println("Time", "Type", "Status", "Message")
	for _, v := range statuses {
		w.Print(common.FormatTime(v.Since, c.isoTime), v.Kind)
		w.PrintStatus(v.Status)
		w.Println(v.Info)
	}
	tw.Flush()
}
