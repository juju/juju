// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

// FetchCommand fetches the results of an action by ID.
type FetchCommand struct {
	ActionCommandBase
	out         cmd.Output
	requestedId string
	fullSchema  bool
}

const fetchDoc = `
Show the results returned by an action with the given ID.  A partial ID may
also be used.
`

// Set up the output.
func (c *FetchCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *FetchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "fetch",
		Args:    "<action ID>",
		Purpose: "show results of an action by ID",
		Doc:     fetchDoc,
	}
}

// Init validates the action ID and any other options.
func (c *FetchCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no action ID specified")
	case 1:
		c.requestedId = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run issues the API call to get Actions by ID.
func (c *FetchCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actionTag, err := getActionTagByPrefix(api, c.requestedId)
	if err != nil {
		return err
	}

	actions, err := api.Actions(params.Entities{
		Entities: []params.Entity{{actionTag.String()}},
	})
	if err != nil {
		return err
	}
	actionResults := actions.Results
	numActionResults := len(actionResults)
	if numActionResults == 0 {
		return c.out.Write(ctx, fmt.Sprintf("no results for action %s", c.requestedId))
	}
	if numActionResults != 1 {
		return errors.Errorf("too many results for action %s", c.requestedId)
	}

	result := actionResults[0]
	if result.Error != nil {
		return result.Error
	}
	return c.out.Write(ctx, formatActionResult(result))
}

func formatActionResult(result params.ActionResult) map[string]interface{} {
	response := map[string]interface{}{"status": result.Status}
	if result.Message != "" {
		response["message"] = result.Message
	}
	if len(result.Output) != 0 {
		response["results"] = result.Output
	}

	if result.Enqueued.IsZero() && result.Started.IsZero() && result.Completed.IsZero() {
		return response
	}

	responseTiming := make(map[string]string)
	for k, v := range map[string]time.Time{
		"enqueued":  result.Enqueued,
		"started":   result.Started,
		"completed": result.Completed,
	} {
		if !v.IsZero() {
			responseTiming[k] = v.String()
		}
	}
	response["timing"] = responseTiming

	return response
}
