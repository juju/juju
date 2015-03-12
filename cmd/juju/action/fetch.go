// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"regexp"
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
	wait        string
}

const fetchDoc = `
Show the results returned by an action with the given ID.  A partial ID may
also be used.  To block until the result is known completed or failed, use
the --wait flag with a duration, as in --wait 5s or --wait 1h.  Use --wait 0
to wait indefinitely.  If units are left off, seconds are assumed.

The default behavior without --wait is to immediately check and return; if
the results are "pending" then only the available information will be
displayed.  This is also the behavior when any negative time is given.
`

// Set up the output.
func (c *FetchCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.StringVar(&c.wait, "wait", "-1s", "wait for results")
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
	// Check whether units were left off our time string.
	r := regexp.MustCompile("[a-zA-Z]")
	matches := r.FindStringSubmatch(c.wait[len(c.wait)-1:])
	// If any match, we have units.  Otherwise, we don't; assume seconds.
	if len(matches) == 0 {
		c.wait = c.wait + "s"
	}

	waitDur, err := time.ParseDuration(c.wait)
	if err != nil {
		return err
	}

	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	// tick every two seconds, to delay the loop timer.
	tick := time.NewTimer(2 * time.Second)
	wait := time.NewTimer(0 * time.Second)

	switch {
	case waitDur.Nanoseconds() < 0:
		// Negative duration signals immediate return.  All is well.
	case waitDur.Nanoseconds() == 0:
		// Zero duration signals indefinite wait.  Discard the tick.
		wait = time.NewTimer(0 * time.Second)
		_ = <-wait.C
	default:
		// Otherwise, start an ordinary timer.
		wait = time.NewTimer(waitDur)
	}

	result, err := timerLoop(api, c.requestedId, wait, tick)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, formatActionResult(result))
}

// timerLoop loops indefinitely to query the given API, until "wait" times
// out, using the "tick" timer to delay the API queries.  It writes the
// result to the given output.
func timerLoop(api APIClient, requestedId string, wait, tick *time.Timer) (params.ActionResult, error) {
	var (
		result params.ActionResult
		err    error
	)

	// Loop over results until we get "failed" or "completed".  Wait for
	// timer, and reset it each time.
	for {
		result, err = fetchResult(api, requestedId)
		if err != nil {
			return result, err
		}

		// Whether or not we're waiting for a result, if a completed
		// result arrives, we're done.
		switch result.Status {
		case params.ActionRunning, params.ActionPending:
		default:
			return result, nil
		}

		// Block until a tick happens, or the timeout arrives.
		select {
		case _ = <-wait.C:
			return result, nil

		case _ = <-tick.C:
			tick.Reset(2 * time.Second)
		}
	}
}

// fetchResult queries the given API for the given Action ID prefix, and
// makes sure the results are acceptable, returning an error if they are not.
func fetchResult(api APIClient, requestedId string) (params.ActionResult, error) {
	none := params.ActionResult{}

	actionTag, err := getActionTagByPrefix(api, requestedId)
	if err != nil {
		return none, err
	}

	actions, err := api.Actions(params.Entities{
		Entities: []params.Entity{{actionTag.String()}},
	})
	if err != nil {
		return none, err
	}
	actionResults := actions.Results
	numActionResults := len(actionResults)
	if numActionResults == 0 {
		return none, errors.Errorf("no results for action %s", requestedId)
	}
	if numActionResults != 1 {
		return none, errors.Errorf("too many results for action %s", requestedId)
	}

	result := actionResults[0]
	if result.Error != nil {
		return none, result.Error
	}

	return result, nil
}

// formatActionResult removes empty values from the given ActionResult and
// inserts the remaining ones in a map[string]interface{} for cmd.Output to
// write in an easy-to-read format.
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
