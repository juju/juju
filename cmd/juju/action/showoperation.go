// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	actionapi "github.com/juju/juju/api/client/action"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

func NewShowOperationCommand() cmd.Command {
	return modelcmd.Wrap(&showOperationCommand{
		clock: clock.WallClock,
	})
}

// showOperationCommand fetches the results of an operation by ID.
type showOperationCommand struct {
	ActionCommandBase
	out         cmd.Output
	requestedID string
	wait        time.Duration
	watch       bool
	utc         bool
	clock       clock.Clock
}

const showOperationDoc = `
Show the results returned by an operation with the given ID.
To block until the result is known completed or failed, use
the ` + "`--wait`" + ` option with a duration, as in ` + "`--wait 5s`" + ` or ` + "`--wait 1h`" + `.
Use ` + "`--watch`" + ` to wait indefinitely.

The default behavior without ` + "`--wait`" + ` or ` + "`--watch`" + ` is to immediately check and return;
if the results are ` + "`pending`" + `, then only the available information will be
displayed.  This is also the behavior when any negative time is given.
`

const showOperationExamples = `
    juju show-operation 1
    juju show-operation 1 --wait=2m
    juju show-operation 1 --watch
`

const defaultOperationWait = -1 * time.Second

// SetFlags implements Command.
func (c *showOperationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	defaultFormatter := "yaml"
	c.out.AddFlags(f, defaultFormatter, map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})

	f.DurationVar(&c.wait, "wait", defaultOperationWait, "Wait for results")
	f.BoolVar(&c.watch, "watch", false, "Wait indefinitely for results")
	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
}

// Info implements Command.
func (c *showOperationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-operation",
		Args:     "<operation-id>",
		Purpose:  "Show results of an operation.",
		Doc:      showOperationDoc,
		Examples: showOperationExamples,
		SeeAlso: []string{
			"run",
			"operations",
			"show-task",
		},
	})
}

// Init implements Command.
func (c *showOperationCommand) Init(args []string) error {
	if c.watch {
		if c.wait != defaultOperationWait {
			return errors.New("specify either --watch or --wait but not both")
		}
		// If we are watching the wait is 0 (indefinite).
		c.wait = 0 * time.Second
	}
	switch len(args) {
	case 0:
		return errors.New("no operation ID specified")
	case 1:
		if !names.IsValidOperation(args[0]) {
			return errors.NotValidf("operation ID %q", args[0])
		}
		c.requestedID = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run implements Command.
func (c *showOperationCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	wait := c.clock.NewTimer(c.wait)
	if c.wait.Nanoseconds() == 0 {
		// Zero duration signals indefinite wait.  Discard the tick.
		<-wait.Chan()
	}

	var result actionapi.Operation
	shouldWatch := c.wait.Nanoseconds() >= 0
	if shouldWatch {
		tick := c.clock.NewTimer(resultPollMaxTime)
		result, err = getOperationResult(api, c.requestedID, tick, wait)
	} else {
		result, err = fetchOperationResult(api, c.requestedID)
	}
	if err != nil {
		return errors.Trace(err)
	}

	formatted := formatOperationResult(result, c.utc)
	return c.out.Write(ctx, formatted)
}

// fetchOperationResult queries the given API for the given operation ID.
func fetchOperationResult(api APIClient, requestedId string) (actionapi.Operation, error) {
	result, err := api.Operation(requestedId)
	if err != nil {
		return result, err
	}
	return result, nil
}

// getOperationResult tries to repeatedly fetch an operation until it is
// in a completed state and then it returns it.
// It waits for a maximum of "wait" before returning with the latest operation status.
func getOperationResult(api APIClient, requestedId string, tick, wait clock.Timer) (actionapi.Operation, error) {
	return operationTimerLoop(api, requestedId, tick, wait)
}

// operationTimerLoop loops indefinitely to query the given API, until "wait" times
// out, using the "tick" timer to delay the API queries.  It writes the
// result to the given output.
func operationTimerLoop(api APIClient, requestedId string, tick, wait clock.Timer) (actionapi.Operation, error) {
	var (
		result actionapi.Operation
		err    error
	)

	// Loop over results until we get "failed", "completed", or "cancelled.  Wait for
	// timer, and reset it each time.
	for {
		result, err = fetchOperationResult(api, requestedId)
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
		case <-wait.Chan():
			switch result.Status {
			case params.ActionRunning, params.ActionPending:
				return result, errors.NewTimeout(err, "timeout reached")
			default:
				return result, nil
			}
		case <-tick.Chan():
			tick.Reset(resultPollMaxTime)
		}
	}
}
