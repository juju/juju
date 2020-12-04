// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/worker/metrics/sender"
)

// TODO(bogdanteleaga): update this once querying for actions by name is implemented.
const collectMetricsDoc = `
Trigger metrics collection

This command waits for the metric collection to finish before returning.
You may abort this command and it will continue to run asynchronously.
Results may be checked by 'juju show-task'.
`

const (
	// commandTimeout represents the timeout for executing the command itself
	commandTimeout = 3 * time.Second
)

var logger = loggo.GetLogger("juju.cmd.juju.collect-metrics")

// collectMetricsCommand retrieves metrics stored in the juju controller.
type collectMetricsCommand struct {
	modelcmd.ModelCommandBase
	unit        string
	application string
	entity      string
}

// NewCollectMetricsCommand creates a new collectMetricsCommand.
func NewCollectMetricsCommand() cmd.Command {
	return modelcmd.Wrap(&collectMetricsCommand{})
}

// Info implements Command.Info.
func (c *collectMetricsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "collect-metrics",
		Args:    "[application or unit]",
		Purpose: "Collect metrics on the given unit/application.",
		Doc:     collectMetricsDoc,
	})
}

// Init reads and verifies the cli arguments for the collectMetricsCommand
func (c *collectMetricsCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("you need to specify a unit or application.")
	}
	c.entity = args[0]
	if names.IsValidUnit(c.entity) {
		c.unit = c.entity
	} else if names.IsValidApplication(args[0]) {
		c.application = c.entity
	} else {
		return errors.Errorf("%q is not a valid unit or application", args[0])
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Errorf("unknown command line arguments: " + strings.Join(args, ","))
	}
	return nil
}

type runClient interface {
	action.APIClient
	Run(run params.RunParams) (params.EnqueuedActions, error)
}

var newRunClient = func(conn api.Connection) runClient {
	return actionapi.NewClient(conn)
}

func parseRunOutput(result params.ActionResult) (string, string, error) {
	if result.Error != nil {
		return "", "", result.Error
	}
	stdout, ok := result.Output["stdout"].(string)
	if !ok {
		return "", "", errors.New("could not read stdout")
	}
	stderr, ok := result.Output["stderr"].(string)
	if !ok {
		return "", "", errors.New("could not read stderr")
	}
	return strings.Trim(stdout, " \t\n"), strings.Trim(stderr, " \t\n"), nil
}

func parseActionResult(result params.ActionResult) (string, error) {
	if result.Action != nil {
		logger.Infof("ran action id %v", result.Action.Tag)
	}
	_, stderr, err := parseRunOutput(result)
	if err != nil {
		return "", errors.Trace(err)
	}
	tag, err := names.ParseUnitTag(result.Action.Receiver)
	if err != nil {
		return "", errors.Trace(err)
	}
	if strings.Contains(stderr, "nc: unix connect failed: No such file or directory") {
		return "", errors.New("no collect application listening: does application support metric collection?")
	}
	return tag.Id(), nil
}

var newAPIConn = func(cmd modelcmd.ModelCommandBase) (api.Connection, error) {
	return cmd.NewAPIRoot()
}

// Run implements Command.Run.
func (c *collectMetricsCommand) Run(ctx *cmd.Context) error {
	root, err := newAPIConn(c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	runnerClient := newRunClient(root)
	defer runnerClient.Close()

	units := []string{}
	applications := []string{}
	if c.unit != "" {
		units = []string{c.unit}
	}
	if c.application != "" {
		applications = []string{c.application}
	}
	runParams := params.RunParams{
		Timeout:      commandTimeout,
		Units:        units,
		Applications: applications,
		Commands:     "nc -U ../metrics-collect.socket",
	}

	// trigger metrics collection
	runResults, err := runnerClient.Run(runParams)
	if err != nil {
		return errors.Trace(err)
	}

	clk := clock.WallClock
	// We want to wait for the action results indefinitely.  Discard the tick.
	wait := clk.NewTimer(0 * time.Second)
	_ = <-wait.Chan()
	// trigger sending metrics in parallel
	resultChannel := make(chan string, len(runResults.Actions))
	for _, result := range runResults.Actions {
		r := result
		if r.Error != nil {
			fmt.Fprintf(ctx.Stdout, "failed to collect metrics: %v\n", err)
			resultChannel <- "invalid id"
			continue
		}
		tag, err := names.ParseActionTag(r.Action.Tag)
		if err != nil {
			fmt.Fprintf(ctx.Stdout, "failed to collect metrics: %v\n", err)
			resultChannel <- "invalid id"
			continue
		}
		actionResult, err := getActionResult(runnerClient, tag.Id(), clk, wait)
		if err != nil {
			fmt.Fprintf(ctx.Stdout, "failed to collect metrics: %v\n", err)
			resultChannel <- "invalid id"
			continue
		}
		unitId, err := parseActionResult(actionResult)
		if err != nil {
			fmt.Fprintf(ctx.Stdout, "failed to collect metrics: %v\n", err)
			resultChannel <- "invalid id"
			continue
		}
		go func() {
			defer func() {
				resultChannel <- unitId
			}()
			sendParams := params.RunParams{
				Timeout:  commandTimeout,
				Units:    []string{unitId},
				Commands: "nc -U ../" + sender.DefaultMetricsSendSocketName,
			}
			sendResults, err := runnerClient.Run(sendParams)
			if err != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			if len(sendResults.Actions) != 1 {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v\n", unitId)
				return
			}
			if sendResults.Actions[0].Error != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, sendResults.Actions[0].Error)
				return
			}
			tag, err := names.ParseActionTag(sendResults.Actions[0].Action.Tag)
			if err != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			actionResult, err := getActionResult(runnerClient, tag.Id(), clk, wait)
			if err != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			stdout, stderr, err := parseRunOutput(actionResult)
			if err != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			if stdout != "ok" {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, errors.New(stderr))
			}
		}()
	}

	for range runResults.Actions {
		// The default is to wait forever for the command to finish.
		select {
		case <-resultChannel:
		}
	}
	return nil
}

// getActionResult abstracts over the action CLI function that we use here to fetch results
var getActionResult = func(c runClient, actionId string, clk clock.Clock, wait clock.Timer) (params.ActionResult, error) {
	tick := clk.NewTimer(2 * time.Second)
	return action.GetActionResult(c, actionId, tick, wait)
}
