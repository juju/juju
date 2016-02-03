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

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

const collectMetricsDoc = `
collect-metrics
trigger metrics collection
`

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
	Run(run params.RunParams) ([]params.RunResult, error)
	Close() error
}

var newRunClient = func(env modelcmd.ModelCommandBase) (runClient, error) {
	return env.NewAPIClient()
}

func resultError(result params.RunResult) string {
	if result.Error != "" {
		return result.Error
	}
	stdout := strings.Trim(string(result.Stdout), " \t\n")
	if stdout == "ok" {
		return ""
	}
	if len(result.Stderr) != 0 && strings.Contains(string(result.Stderr), "No such file or directory") {
		return "not a metered charm"
	}
	return ""
}

// Run implements Command.Run.
func (c *collectMetricsCommand) Run(ctx *cmd.Context) error {
	runnerClient, err := newRunClient(c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	defer runnerClient.Close()

	runParams := params.RunParams{
		Timeout:  3 * time.Second,
		Units:    c.units,
		Services: c.services,
		Commands: "nc -U ../metrics-collect.socket",
	}

	// trigger metrics collection
	runResults, err := runnerClient.Run(runParams)
	if err != nil {
		return errors.Trace(err)
	}

	// trigger sending metrics in parallel
	resultChannel := make(chan string, len(runResults))
	for _, result := range runResults {
		r := result
		errString := resultError(r)
		if errString == "" {
			go func() {
				defer func() {
					resultChannel <- r.UnitId
				}()
				sendParams := params.RunParams{
					Timeout:  3 * time.Second,
					Units:    []string{r.UnitId},
					Commands: "nc -U ../metrics-send.socket",
				}
				sendResults, err := runnerClient.Run(sendParams)
				if err != nil {
					fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", r.UnitId, err)
					return
				}
				if len(sendResults) != 1 {
					fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v\n", r.UnitId)
					return
				}
				errString := sendResults[0].Error
				stdout := strings.Trim(string(sendResults[0].Stdout), " \t\n")
				if stdout != "ok" {
					errString = strings.Trim(string(sendResults[0].Stderr), " \t\n")
				}
				if errString != "" {
					fmt.Fprintf(ctx.Stdout, "failed to send metrics for unit %v: %v\n", r.UnitId, errString)
				}
			}()
		} else {
			fmt.Fprintf(ctx.Stdout, "failed to collect metrics for unit %v: %v\n", r.UnitId, errString)
			resultChannel <- r.UnitId
		}
	}

	for _ = range runResults {
		select {
		case <-resultChannel:
		case <-time.After(3 * time.Second):
			fmt.Fprintf(ctx.Stdout, "wait result timeout")
			break
		}
	}
	return nil
}
