// Copyright 2014-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	actionapi "github.com/juju/juju/api/client/action"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
)

func NewCancelCommand() cmd.Command {
	return modelcmd.Wrap(&cancelCommand{})
}

type cancelCommand struct {
	ActionCommandBase
	out          cmd.Output
	requestedIDs []string
}

// Set up the output.
func (c *cancelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

const cancelDoc = `
Cancel pending or running tasks matching given IDs or partial ID prefixes.`

func (c *cancelCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "cancel-task",
		Args:    "(<task-id>|<task-id-prefix>) [...]",
		Purpose: "Cancel pending or running tasks.",
		Doc:     cancelDoc,
	}
	return jujucmd.Info(info)
}

func (c *cancelCommand) Init(args []string) error {
	for _, arg := range args {
		if !names.IsValidAction(arg) {
			return errors.NotValidf("task ID %q", arg)
		}
	}
	c.requestedIDs = args
	return nil
}

func (c *cancelCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	if len(c.requestedIDs) == 0 {
		return errors.Errorf("no task IDs specified")
	}

	actions, err := api.Cancel(c.requestedIDs)
	if err != nil {
		return err
	}

	if len(actions) < 1 {
		return errors.Errorf("no tasks found, no tasks have been canceled")
	}

	type unCanceledAction struct {
		ID     string
		Result *actionapi.ActionResult
	}
	var failedCancels []unCanceledAction
	var canceledActions []actionapi.ActionResult

	for i, v := range actions {
		result := v
		if result.Action != nil {
			canceledActions = append(canceledActions, result)
		} else {
			failedCancels = append(failedCancels, unCanceledAction{c.requestedIDs[i], &result})
		}
	}

	if len(canceledActions) > 0 {
		err = c.out.Write(ctx, resultsToMap(canceledActions))
	}

	if len(failedCancels) > 0 {
		message := "The following tasks could not be canceled:\n"
		for _, a := range failedCancels {
			message += fmt.Sprintf("task: %s, error: %s\n", a.ID, a.Result.Message)
		}

		logger.Warningf(message)
	}

	return err
}

// resultsToMap is a helper function that takes in a []params.ActionResult
// and returns a map[string]interface{} ready to be served to the
// formatter for printing.
func resultsToMap(results []actionapi.ActionResult) map[string]interface{} {
	items := []map[string]interface{}{}
	for _, item := range results {
		items = append(items, resultToMap(item))
	}
	return map[string]interface{}{"actions": items}
}

func resultToMap(result actionapi.ActionResult) map[string]interface{} {
	item := map[string]interface{}{}
	if result.Error != nil {
		item["error"] = result.Error.Error()
	}
	if result.Action != nil {
		item["action"] = result.Action.Name
		item["id"] = result.Action.ID

		rtag, err := names.ParseUnitTag(result.Action.Receiver)
		if err != nil {
			item["unit"] = result.Action.Receiver
		} else {
			item["unit"] = rtag.Id()
		}

	}
	item["status"] = result.Status

	// result.Completed uses the zero-value to indicate not completed
	if result.Completed.Equal(time.Time{}) {
		item["completed at"] = "n/a"
	} else {
		item["completed at"] = result.Completed.UTC().Format("2006-01-02 15:04:05")
	}

	return item
}
