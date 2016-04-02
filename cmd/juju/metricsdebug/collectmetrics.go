// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/action"
	"github.com/juju/juju/cmd/modelcmd"
)

// TODO(bogdanteleaga): update this once querying for actions by name is implemented.
const collectMetricsDoc = `
collect-metrics
trigger metrics collection

Collect metrics infinitely waits for the command to finish.
However if you cancel the command while waiting, you can still
look for the results under 'juju action status'.
`

const (
	// commandTimeout represents the timeout for executing the command itself
	commandTimeout = 3 * time.Second
)

// collectMetricsCommand retrieves metrics stored in the juju controller.
type collectMetricsCommand struct {
	modelcmd.ModelCommandBase
	units    []string
	services []string
}

// NewCollectMetricsCommand creates a new collectMetricsCommand.
func NewCollectMetricsCommand() cmd.Command {
	return modelcmd.Wrap(&collectMetricsCommand{})
}

// Info implements Command.Info.
func (c *collectMetricsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "collect-metrics",
		Args:    "[service or unit]",
		Purpose: "collect metrics on the given unit/service",
		Doc:     collectMetricsDoc,
	}
}

// Init reads and verifies the cli arguments for the collectMetricsCommand
func (c *collectMetricsCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("you need to specify a unit or service.")
	}
	if names.IsValidUnit(args[0]) {
		c.units = []string{args[0]}
	} else if names.IsValidService(args[0]) {
		c.services = []string{args[0]}
	} else {
		return errors.Errorf("%q is not a valid unit or service", args[0])
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Errorf("unknown command line arguments: " + strings.Join(args, ","))
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *collectMetricsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

type runClient interface {
	action.APIClient
	Run(run params.RunParams) ([]params.ActionResult, error)
}

var newRunClient = func(env modelcmd.ModelCommandBase) (runClient, error) {
	root, err := env.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return actionapi.NewClient(root), errors.Trace(err)
}

func parseRunOutput(result params.ActionResult) (string, string, error) {
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

func parseActionResult(result params.ActionResult) (string, error) {
	stdout, stderr, err := parseRunOutput(result)
	if err != nil {
		return "", errors.Trace(err)
	}
	tag, err := names.ParseUnitTag(result.Action.Receiver)
	if err != nil {
		return "", errors.Trace(err)
	}
	if stdout == "ok" {
		return tag.Id(), nil
	}
	if strings.Contains(stderr, "No such file or directory") {
		return "", errors.New("not a metered charm")
	}
	return tag.Id(), nil
}

// Run implements Command.Run.
func (c *collectMetricsCommand) Run(ctx *cmd.Context) error {
	runnerClient, err := newRunClient(c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	defer runnerClient.Close()

	runParams := params.RunParams{
		Timeout:  commandTimeout,
		Units:    c.units,
		Services: c.services,
		Commands: "nc -U ../metrics-collect.socket",
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
	resultChannel := make(chan string, len(runResults))
	for _, result := range runResults {
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
		actionResult, err := getActionResult(runnerClient, tag.Id(), wait)
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
				Commands: "nc -U ../metrics-send.socket",
			}
			sendResults, err := runnerClient.Run(sendParams)
			if err != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			if len(sendResults) != 1 {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v\n", unitId)
				return
			}
			if sendResults[0].Error != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, sendResults[0].Error)
				return
			}
			tag, err := names.ParseActionTag(sendResults[0].Action.Tag)
			if err != nil {
				fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", unitId, err)
				return
			}
			fmt.Println("hi" + tag.Id())
			actionResult, err := getActionResult(runnerClient, tag.Id(), wait)
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

	for _ = range runResults {
		// The default is to wait forever for the command to finish.
		select {
		case <-resultChannel:
		}
	}
	return nil
}

// getActionResult abstracts over the action CLI function that we use here to fetch results
var getActionResult = func(c runClient, actionId string, wait *time.Timer) (params.ActionResult, error) {
	return action.GetActionResult(c, actionId, wait)
}
