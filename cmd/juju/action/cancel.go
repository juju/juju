// Copyright 2014-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
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

	var actionTags []names.ActionTag
	for _, requestedID := range c.requestedIDs {
		actionTags = append(actionTags, names.NewActionTag(requestedID))
	}

	entities := []params.Entity{}
	for _, tag := range actionTags {
		entities = append(entities, params.Entity{Tag: tag.String()})
	}

	actions, err := api.Cancel(params.Entities{Entities: entities})
	if err != nil {
		return err
	}

	if len(actions.Results) < 1 {
		return errors.Errorf("no tasks found, no tasks have been canceled")
	}

	type unCanceledAction struct {
		ActionTag names.ActionTag
		Result    *params.ActionResult
	}
	var unCanceledActions []unCanceledAction
	var canceledActions []params.ActionResult

	for i, result := range actions.Results {
		if result.Action != nil {
			canceledActions = append(canceledActions, result)
		} else {
			unCanceledActions = append(unCanceledActions, unCanceledAction{actionTags[i], &result})
		}
	}

	if len(canceledActions) > 0 {
		err = c.out.Write(ctx, resultsToMap(canceledActions))
	}

	if len(unCanceledActions) > 0 {
		message := "The following tasks could not be canceled:\n"
		for _, a := range unCanceledActions {
			message += fmt.Sprintf("task: %s, error: %s\n", a.ActionTag, a.Result.Message)
		}

		logger.Warningf(message)
	}

	return err
}

// resultsToMap is a helper function that takes in a []params.ActionResult
// and returns a map[string]interface{} ready to be served to the
// formatter for printing.
func resultsToMap(results []params.ActionResult) map[string]interface{} {
	items := []map[string]interface{}{}
	for _, item := range results {
		items = append(items, resultToMap(item))
	}
	return map[string]interface{}{"actions": items}
}

func resultToMap(result params.ActionResult) map[string]interface{} {
	item := map[string]interface{}{}
	if result.Error != nil {
		item["error"] = result.Error.Error()
	}
	if result.Action != nil {
		item["action"] = result.Action.Name
		atag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			item["id"] = result.Action.Tag
		} else {
			item["id"] = atag.Id()
		}

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
