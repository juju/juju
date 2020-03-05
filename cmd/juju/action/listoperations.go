// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/actions"
)

func NewListOperationsCommand() cmd.Command {
	return modelcmd.Wrap(&listOperationsCommand{})
}

// listOperationsCommand fetches the results of an action by ID.
type listOperationsCommand struct {
	ActionCommandBase
	out              cmd.Output
	utc              bool
	applicationNames []string
	unitNames        []string
	actionNames      []string
	statusValues     []string
}

const listOperationsDoc = `
List the operations with the specified query criteria.
When an application is specified, all units from that application are relevant.

Examples:
    juju operations
    juju operations --format yaml
    juju operations --actions backup,restore
    juju operations --apps mysql,mediawiki
    juju operations --units mysql/0,mediawiki/1
    juju operations --status pending,completed
    juju operations --apps mysql --units mediawiki/0 --status running --actions backup

See also:
    run
    show-operation
    show-task
`

// Set up the output.
func (c *listOperationsCommand) SetFlags(f *gnuflag.FlagSet) {
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
	f.Var(cmd.NewStringsValue(nil, &c.actionNames), "actions", "Comma separated list of actions names to filter on")
	f.Var(cmd.NewStringsValue(nil, &c.statusValues), "status", "Comma separated list of operation status values to filter on")
}

func (c *listOperationsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "operations",
		Purpose: "Lists pending, running, or completed operations for specified application, units, or all.",
		Doc:     listOperationsDoc,
		Aliases: []string{"list-operations"},
	})
}

// Init implements Command.
func (c *listOperationsCommand) Init(args []string) error {
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
			params.ActionCompleted,
			params.ActionFailed,
			params.ActionCancelled,
			params.ActionAborting,
			params.ActionAborted:
		default:
			nameErrors = append(nameErrors,
				fmt.Sprintf("%q is not a valid task status, want one of %v",
					status,
					[]string{params.ActionPending,
						params.ActionRunning,
						params.ActionCompleted,
						params.ActionFailed,
						params.ActionCancelled,
						params.ActionAborting,
						params.ActionAborted}))
		}
	}
	if len(nameErrors) > 0 {
		return errors.New(strings.Join(nameErrors, "\n"))
	}
	return cmd.CheckEmpty(args)
}

// Run implements Command.
func (c *listOperationsCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	args := params.OperationQueryArgs{
		Applications: c.applicationNames,
		Units:        c.unitNames,
		ActionNames:  c.actionNames,
		Status:       c.statusValues,
	}
	results, err := api.ListOperations(args)
	if err != nil {
		return errors.Trace(err)
	}

	out := make(map[string]interface{})
	var operationResults byId = results.Results
	if len(operationResults) == 0 {
		ctx.Infof("no matching operations")
		return nil
	}

	sort.Sort(operationResults)
	if c.out.Name() == "plain" {
		return c.out.Write(ctx, operationResults)
	}
	for _, result := range operationResults {
		tag, err := names.ParseOperationTag(result.OperationTag)
		if err != nil {
			return errors.Trace(err)
		}
		out[tag.Id()] = formatOperationResult(result, c.utc)
	}
	return c.out.Write(ctx, out)
}

type operationLine struct {
	started   time.Time
	finished  time.Time
	id        string
	operation string
	tasks     []string
	status    string
}

const maxTaskIDs = 5

func (c *listOperationsCommand) formatTabular(writer io.Writer, value interface{}) error {
	results, ok := value.(byId)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", results, value)
	}
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.SetColumnAlignRight(0)

	printOperations := func(operations []operationLine, utc bool) {
		for _, line := range operations {
			numTasks := len(line.tasks)
			if numTasks > maxTaskIDs {
				numTasks = maxTaskIDs
			}
			tasks := strings.Join(line.tasks[:numTasks], ",")
			if len(line.tasks) > maxTaskIDs {
				tasks += "..."
			}
			w.Print(line.id, line.status)
			w.Print(formatTimestamp(line.started, false, c.utc, true))
			w.Print(formatTimestamp(line.finished, false, c.utc, true))
			w.Print(tasks)
			w.Println(line.operation)
		}
	}
	w.Println("Id", "Status", "Started", "Finished", "Task IDs", "Summary")
	printOperations(actionOperationLinesFromResults(results), c.utc)
	return tw.Flush()
}

func operationDisplayTime(r params.OperationResult) time.Time {
	timestamp := r.Completed
	if timestamp.IsZero() {
		timestamp = r.Started
	}
	if timestamp.IsZero() {
		timestamp = r.Enqueued
	}
	return timestamp
}

func actionOperationLinesFromResults(results []params.OperationResult) []operationLine {
	var operationLines []operationLine
	for _, r := range results {
		line := operationLine{
			started:   r.Started,
			finished:  r.Completed,
			status:    r.Status,
			operation: r.Summary,
		}
		if at, err := names.ParseOperationTag(r.OperationTag); err == nil {
			line.id = at.Id()
		}
		for _, a := range r.Actions {
			if a.Action == nil {
				line.status = "error"
				continue
			}
			tag, err := names.ParseActionTag(a.Action.Tag)
			if err != nil {
				// Not expected to happen.
				continue
			}
			line.tasks = append(line.tasks, tag.Id())
		}
		operationLines = append(operationLines, line)
	}
	return operationLines
}

type byId []params.OperationResult

func (s byId) Len() int {
	return len(s)
}
func (s byId) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byId) Less(i, j int) bool {
	return s[i].OperationTag < s[j].OperationTag
}

type operationInfo struct {
	Summary string              `yaml:"summary" json:"summary"`
	Status  string              `yaml:"status" json:"status"`
	Error   string              `yaml:"error,omitempty" json:"error,omitempty"`
	Action  *actionSummary      `yaml:"action,omitempty" json:"action,omitempty"`
	Timing  timingInfo          `yaml:"timing,omitempty" json:"timing,omitempty"`
	Tasks   map[string]taskInfo `yaml:"tasks,omitempty" json:"tasks,omitempty"`
}

type timingInfo struct {
	Enqueued  string `yaml:"enqueued,omitempty" json:"enqueued,omitempty"`
	Started   string `yaml:"started,omitempty" json:"started,omitempty"`
	Completed string `yaml:"completed,omitempty" json:"completed,omitempty"`
}

type actionSummary struct {
	Name       string                 `yaml:"name" json:"name"`
	Parameters map[string]interface{} `yaml:"parameters" json:"parameters"`
}

type taskInfo struct {
	Name    string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Host    string                 `yaml:"host" json:"host"`
	Status  string                 `yaml:"status" json:"status"`
	Timing  timingInfo             `yaml:"timing,omitempty" json:"timing,omitempty"`
	Log     []string               `yaml:"log,omitempty" json:"log,omitempty"`
	Message string                 `yaml:"message,omitempty" json:"message,omitempty"`
	Results map[string]interface{} `yaml:"results,omitempty" json:"results,omitempty"`
}

// formatOperationResult inserts the remaining ones in a map[string]interface{} for cmd.Output to
// write in an easy-to-read format.
func formatOperationResult(operation params.OperationResult, utc bool) operationInfo {
	result := operationInfo{
		Summary: operation.Summary,
		Status:  operation.Status,
		Timing: timingInfo{
			Enqueued:  formatTimestamp(operation.Enqueued, false, utc, false),
			Started:   formatTimestamp(operation.Started, false, utc, false),
			Completed: formatTimestamp(operation.Completed, false, utc, false),
		},
		Tasks: make(map[string]taskInfo, len(operation.Actions)),
	}
	if err := operation.Error; err != nil {
		result.Error = err.Error()
	}
	var singleAction actionSummary
	haveSingleAction := true
	for i, task := range operation.Actions {
		if task.Action == nil {
			result.Status = "error"
			continue
		}
		tag, err := names.ParseActionTag(task.Action.Tag)
		if err != nil {
			// Not expected to happen.
			continue
		}
		taskInfo := taskInfo{
			Host:   task.Action.Receiver,
			Status: task.Status,
			Timing: timingInfo{
				Enqueued:  formatTimestamp(task.Enqueued, false, utc, false),
				Started:   formatTimestamp(task.Started, false, utc, false),
				Completed: formatTimestamp(task.Completed, false, utc, false),
			},
			Message: task.Message,
			Results: task.Output,
		}
		ut, err := names.ParseUnitTag(task.Action.Receiver)
		if err == nil {
			taskInfo.Host = ut.Id()
		}
		if len(task.Log) > 0 {
			logs := make([]string, len(task.Log))
			for i, msg := range task.Log {
				logs[i] = formatLogMessage(actions.ActionMessage{
					Timestamp: msg.Timestamp,
					Message:   msg.Message,
				}, false, utc, false)
			}
			taskInfo.Log = logs
		}
		result.Tasks[tag.Id()] = taskInfo
		if i == 0 {
			singleAction = actionSummary{
				Name:       task.Action.Name,
				Parameters: make(map[string]interface{}),
			}
			for k, v := range task.Action.Parameters {
				singleAction.Parameters[k] = v
			}
			continue
		}
		// Check to see if there's a different action as part of the operation.
		// Short circuit the deep equals check if we don't need to do it.
		if haveSingleAction {
			haveSingleAction = task.Action.Name == singleAction.Name && reflect.DeepEqual(task.Action.Parameters, singleAction.Parameters)
		}
	}
	if haveSingleAction && singleAction.Name != "" {
		result.Action = &singleAction
	} else {
		for i, a := range operation.Actions {
			if a.Action == nil {
				continue
			}
			tag, err := names.ParseActionTag(a.Action.Tag)
			if err != nil {
				// Not expected to happen.
				continue
			}
			task := result.Tasks[tag.Id()]
			task.Name = operation.Actions[i].Action.Name
			result.Tasks[tag.Id()] = task
		}
	}
	return result
}
