// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	actionapi "github.com/juju/juju/api/client/action"
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
Results may be checked by 'juju show-action-status'.
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
		return errors.New("unknown command line arguments: " + strings.Join(args, ","))
	}
	return nil
}

type runClient interface {
	action.APIClient
	Run(run actionapi.RunParams) (actionapi.EnqueuedActions, error)
}

var newRunClient = func(conn api.Connection) runClient {
	return actionapi.NewClient(conn)
}

func parseRunOutput(result actionapi.ActionResult) (string, string, error) {
	if result.Error != nil {
		return "", "", result.Error
	}
	stdout, ok := result.Output["Stdout"].(string)
	if !ok {
		return "", "", errors.New("could not read stdout")
	}
	stderr, ok := result.Output["Stderr"].(string)
	if !ok {
		return "", "", errors.New("could not read stderr")
	}
	return strings.Trim(stdout, " \t\n"), strings.Trim(stderr, " \t\n"), nil
}

func parseActionResult(result actionapi.ActionResult) (string, error) {
	if result.Action != nil {
		logger.Infof("ran action id %v", result.Action.ID)
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
	runParams := actionapi.RunParams{
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

	// We want to wait for the action results indefinitely.  Discard the tick.
	wait := time.NewTimer(0 * time.Second)
	_ = <-wait.C
	// trigger sending metrics in parallel
	wg := &sync.WaitGroup{}
	wg.Add(len(runResults.Actions))
	for _, result := range runResults.Actions {
		r := result
		if err := r.Error; err != nil {
			_, _ = fmt.Fprintf(ctx.Stderr, "failed to collect metrics: %v\n", err)
			wg.Done()
			continue
		}
		actionResult, err := getActionResult(runnerClient, r.Action.ID, wait)
		if err != nil {
			_, _ = fmt.Fprintf(ctx.Stderr, "failed to collect metrics: %v\n", err)
			wg.Done()
			continue
		}
		unitId, err := parseActionResult(actionResult)
		if err != nil {
			_, _ = fmt.Fprintf(ctx.Stderr, "failed to collect metrics: %v\n", err)
			wg.Done()
			continue
		}
		go func() {
			defer wg.Done()
			sendParams := actionapi.RunParams{
				Timeout:  commandTimeout,
				Units:    []string{unitId},
				Commands: "nc -U ../" + sender.DefaultMetricsSendSocketName,
			}
			sendResults, err := runnerClient.Run(sendParams)
			if err != nil {
				_, _ = fmt.Fprintf(ctx.Stderr, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			if len(sendResults.Actions) != 1 {
				_, _ = fmt.Fprintf(ctx.Stderr, "failed to send metrics for unit %v\n", unitId)
				return
			}
			if sendResults.Actions[0].Error != nil {
				_, _ = fmt.Fprintf(ctx.Stderr, "failed to send metrics for unit %v: %v\n", unitId, sendResults.Actions[0].Error)
				return
			}
			actionResult, err := getActionResult(runnerClient, sendResults.Actions[0].Action.ID, wait)
			if err != nil {
				_, _ = fmt.Fprintf(ctx.Stderr, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			stdout, stderr, err := parseRunOutput(actionResult)
			if err != nil {
				_, _ = fmt.Fprintf(ctx.Stderr, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			if stdout != "ok" {
				_, _ = fmt.Fprintf(ctx.Stderr, "failed to send metrics for unit %v: %v\n", unitId, errors.New(stderr))
			}
		}()
	}

	wg.Wait()
	return nil
}

// getActionResult abstracts over the action CLI function that we use here to fetch results
var getActionResult = func(c runClient, actionId string, wait *time.Timer) (actionapi.ActionResult, error) {
	return action.GetActionResult(c, actionId, wait, false)
}
