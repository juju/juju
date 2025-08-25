// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	actionapi "github.com/juju/juju/api/client/action"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/rpc/params"
)

// NewListOperationsCommand returns a ListOperations command.
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
	machineNames     []string
	actionNames      []string
	statusValues     []string

	// These attributes are used for batching large result sets.
	limit  uint
	offset uint
}

const listOperationsDoc = `
List the operations with the specified query criteria.
When an application is specified, all units from that application are relevant.

When run without any arguments, operations corresponding to actions for all
application units are returned.
To see operations corresponding to ` + "`juju run`" + ` tasks, specify an action name,
` + "`juju-exec`" + `, and/or one or more machines.
`

const listOperationsExamples = `
    juju operations
    juju operations --format yaml
    juju operations --actions juju-exec
    juju operations --actions backup,restore
    juju operations --apps mysql,mediawiki
    juju operations --units mysql/0,mediawiki/1
    juju operations --machines 0,1
    juju operations --status pending,completed
    juju operations --apps mysql --units mediawiki/0 --status running --actions backup

`

// SetFlags implements Command.
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
	f.Var(cmd.NewStringsValue(nil, &c.machineNames), "machines", "Comma separated list of machines to filter on")
	f.Var(cmd.NewStringsValue(nil, &c.actionNames), "actions", "Comma separated list of actions names to filter on")
	f.Var(cmd.NewStringsValue(nil, &c.statusValues), "status", "Comma separated list of operation status values to filter on")
	f.UintVar(&c.limit, "limit", 0, "The maximum number of operations to return")
	f.UintVar(&c.offset, "offset", 0, "Return operations from offset onwards")
}

func (c *listOperationsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "operations",
		Purpose:  "Lists pending, running, or completed operations for specified application, units, machines, or all.",
		Doc:      listOperationsDoc,
		Aliases:  []string{"list-operations"},
		Examples: listOperationsExamples,
		SeeAlso: []string{
			"run",
			"show-operation",
			"show-task",
		},
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
	for _, machine := range c.machineNames {
		if !names.IsValidMachine(machine) {
			nameErrors = append(nameErrors, fmt.Sprintf("invalid machine id %q", machine))
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
			params.ActionAborted,
			params.ActionError:
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
						params.ActionAborted,
						params.ActionError}))
		}
	}
	if len(nameErrors) > 0 {
		return errors.New(strings.Join(nameErrors, "\n"))
	}
	return cmd.CheckEmpty(args)
}

const defaultMaxOperationsLimit = 50

// Run implements Command.
func (c *listOperationsCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	args := actionapi.OperationQueryArgs{
		Applications: c.applicationNames,
		Units:        c.unitNames,
		Machines:     c.machineNames,
		ActionNames:  c.actionNames,
		Status:       c.statusValues,
	}
	if c.offset != 0 {
		offset := int(c.offset)
		args.Offset = &offset
	}
	if c.limit != 0 {
		limit := int(c.limit)
		args.Limit = &limit
	}
	results, err := api.ListOperations(args)
	if err != nil {
		return errors.Trace(err)
	}

	out := make(map[string]interface{})
	var operationResults byId = results.Operations
	if len(operationResults) == 0 {
		ctx.Infof("no matching operations")
		return nil
	}

	sort.Sort(operationResults)
	if c.out.Name() == "plain" {
		if c.offset > 0 || results.Truncated {
			fmt.Fprintf(ctx.Stdout, "Displaying operation results %d to %d.\n", c.offset+1, int(c.offset)+len(operationResults))
			if results.Truncated {
				limit := c.limit
				if limit == 0 {
					limit = defaultMaxOperationsLimit
				}
				fmt.Fprintf(ctx.Stdout, "Run the command again with --offset=%d --limit=%d to see the next batch.\n\n", c.offset+limit, limit)
			}
		}
		return c.out.Write(ctx, operationResults)
	}
	for _, result := range operationResults {
		out[result.ID] = formatOperationResult(result, c.utc)
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
	w := output.Wrapper{TabWriter: tw}
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
	w.Println("ID", "Status", "Started", "Finished", "Task IDs", "Summary")
	printOperations(actionOperationLinesFromResults(results), c.utc)
	return tw.Flush()
}

func actionOperationLinesFromResults(results []actionapi.Operation) []operationLine {
	var operationLines []operationLine
	for _, r := range results {
		line := operationLine{
			id:        r.ID,
			started:   r.Started,
			finished:  r.Completed,
			status:    r.Status,
			operation: r.Summary,
		}
		for _, a := range r.Actions {
			if a.Action == nil {
				line.status = "error"
				continue
			}
			line.tasks = append(line.tasks, a.Action.ID)
		}
		operationLines = append(operationLines, line)
	}
	return operationLines
}

type byId []actionapi.Operation

func (s byId) Len() int {
	return len(s)
}
func (s byId) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byId) Less(i, j int) bool {
	// We expect IDs to be ints but legacy actions
	// still use UUIDs (err will be non-nil below).
	id1, err1 := strconv.Atoi(s[i].ID)
	id2, err2 := strconv.Atoi(s[j].ID)
	if err1 != nil && err2 == nil {
		return true
	}
	if err1 == nil && err2 != nil {
		return false
	}
	if err1 == nil && err2 == nil {
		return id1 < id2
	}
	return s[i].ID < s[j].ID
}

type operationInfo struct {
	Summary string              `yaml:"summary" json:"summary"`
	Status  string              `yaml:"status" json:"status"`
	Fail    string              `yaml:"fail,omitempty" json:"fail,omitempty"`
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
func formatOperationResult(operation actionapi.Operation, utc bool) operationInfo {
	result := operationInfo{
		Summary: operation.Summary,
		Fail:    operation.Fail,
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
		receiver, err := names.ParseTag(task.Action.Receiver)
		if err == nil {
			taskInfo.Host = receiver.Id()
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
		result.Tasks[task.Action.ID] = taskInfo
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
			task := result.Tasks[a.Action.ID]
			task.Name = operation.Actions[i].Action.Name
			result.Tasks[a.Action.ID] = task
		}
	}
	return result
}
