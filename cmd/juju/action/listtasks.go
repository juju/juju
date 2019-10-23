// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/output"
	"gopkg.in/juju/names.v3"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewListTasksCommand() cmd.Command {
	return modelcmd.Wrap(&listTasksCommand{})
}

// listTasksCommand fetches the results of an action by ID.
type listTasksCommand struct {
	ActionCommandBase
	out              cmd.Output
	utc              bool
	applicationNames []string
	unitNames        []string
	functionNames    []string
	statusValues     []string
}

const listTasksDoc = `
List the tasks with the specified query criteria.
With no query arguments, any completed tasks will be listed.
A completed task is one that has run successfully, been cancelled, or failed.

When an application is specified, all units from that application are relevant.

Examples:
    juju tasks
    juju tasks --format yaml
    juju tasks --functions backup,restore
    juju tasks --apps mysql,mediawiki
    juju tasks --units mysql/0,mediawiki/1
    juju tasks --status pending,completed
    juju tasks --apps mysql --units mediawiki/0 --status running --functions backup

See also:
    call
    show-task
`

// Set up the output.
func (c *listTasksCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	defaultFormatter := "plain"
	c.out.AddFlags(f, defaultFormatter, map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": c.formatTabular,
	})

	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
	f.Var(cmd.NewStringsValue(nil, &c.applicationNames), "applications", "Comma separated list of applications to filter on")
	f.Var(cmd.NewStringsValue(nil, &c.applicationNames), "apps", "Comma separated list of applications to filter on")
	f.Var(cmd.NewStringsValue(nil, &c.unitNames), "units", "Comma separated list of units to filter on")
	f.Var(cmd.NewStringsValue(nil, &c.functionNames), "functions", "Comma separated list of function names to filter on")
	f.Var(cmd.NewStringsValue([]string{params.ActionCompleted}, &c.statusValues), "status", "Comma separated list of task status values to filter on")
}

func (c *listTasksCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "tasks",
		Purpose: "Lists pending, running, or completed tasks for specified application, units, or all.",
		Doc:     listTasksDoc,
		Aliases: []string{"list-tasks"},
	})
}

// Init implements Command.
func (c *listTasksCommand) Init(args []string) error {
	var nameErrors []string
	for _, application := range c.applicationNames {
		if !names.IsValidApplication(application) {
			nameErrors = append(nameErrors, fmt.Sprintf("invalid application name %q", application))
		}
	}
	for _, unit := range c.unitNames {
		if !names.IsValidUnit(unit) {
			nameErrors = append(nameErrors, fmt.Sprintf("invalid unit name %q", unit))
		}
	}
	for _, status := range c.statusValues {
		switch status {
		case params.ActionPending,
			params.ActionRunning,
			params.ActionCompleted:
		default:
			nameErrors = append(nameErrors,
				fmt.Sprintf("%q is not a valid function status, want one of %v",
					status,
					[]string{params.ActionPending, params.ActionRunning, params.ActionCompleted}))
		}
	}
	if len(nameErrors) > 0 {
		return errors.New(strings.Join(nameErrors, "\n"))
	}
	return cmd.CheckEmpty(args)
}

// Run implements Command.
func (c *listTasksCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	args := params.TaskQueryArgs{
		Applications:  c.applicationNames,
		Units:         c.unitNames,
		FunctionNames: c.functionNames,
		Status:        c.statusValues,
	}
	results, err := api.Tasks(args)
	if err != nil {
		return errors.Trace(err)
	}

	out := make(map[string]interface{})
	var actionResults byId = results.Results
	if len(actionResults) == 0 {
		fmt.Fprintln(ctx.Stderr, "no matching tasks")
		return nil
	}

	sort.Sort(actionResults)
	for _, result := range actionResults {
		if result.Error != nil {
			continue
		}
		tag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		out[tag.Id()] = FormatActionResult(result, c.utc)
	}
	if c.out.Name() != "plain" {
		return c.out.Write(ctx, out)
	}
	return c.out.Write(ctx, actionResults)
}

type taskLine struct {
	timestamp time.Time
	id        string
	task      string
	status    string
	unit      string
}

func (c *listTasksCommand) formatTabular(writer io.Writer, value interface{}) error {
	results, ok := value.(byId)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", results, value)
	}
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.SetColumnAlignRight(1)

	printTasks := func(tasks []taskLine, utc bool) {
		for _, line := range tasks {
			w.Print(formatTimestamp(line.timestamp, false, c.utc))
			w.Println(line.id, line.task, line.status, line.unit)
		}
	}
	w.Println("Time", "Id", "Task", "Status", "Unit")
	printTasks(actionTaskLinesFromResults(results), c.utc)
	return tw.Flush()
}

func taskDisplayTime(r params.ActionResult) time.Time {
	timestamp := r.Completed
	if timestamp.IsZero() {
		timestamp = r.Started
	}
	if timestamp.IsZero() {
		timestamp = r.Enqueued
	}
	return timestamp
}

func actionTaskLinesFromResults(results []params.ActionResult) []taskLine {
	sort.Sort(byTimestamp(results))

	var taskLines []taskLine
	for _, r := range results {
		if r.Action == nil {
			continue
		}
		line := taskLine{
			timestamp: taskDisplayTime(r),
			status:    r.Status,
			task:      r.Action.Name,
		}
		if at, err := names.ParseActionTag(r.Action.Tag); err == nil {
			line.id = at.Id()
		}
		if ut, err := names.ParseUnitTag(r.Action.Receiver); err == nil {
			line.unit = ut.Id()
		}
		taskLines = append(taskLines, line)
	}
	return taskLines
}

type byTimestamp []params.ActionResult

func (s byTimestamp) Len() int {
	return len(s)
}

func (s byTimestamp) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byTimestamp) Less(i, j int) bool {
	return taskDisplayTime(s[i]).UnixNano() < taskDisplayTime(s[j]).UnixNano()
}

type byId []params.ActionResult

func (s byId) Len() int {
	return len(s)
}
func (s byId) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byId) Less(i, j int) bool {
	if s[i].Action == nil {
		return true
	}
	if s[j].Action == nil {
		return false
	}
	return s[i].Action.Tag < s[j].Action.Tag
}
