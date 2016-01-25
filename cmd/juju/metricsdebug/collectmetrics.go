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
	"github.com/juju/juju/cmd/envcmd"
)

const collectMetricsDoc = `
debug-metrics
trigger metrics collection
`

// collectMetricsCommand retrieves metrics stored in the juju controller.
type collectMetricsCommand struct {
	envcmd.EnvCommandBase
	units    []string
	services []string
}

// NewCollectMetricsCommand creates a new collectMetricsCommand.
func NewCollectMetricsCommand() cmd.Command {
	return envcmd.Wrap(&collectMetricsCommand{})
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
	c.EnvCommandBase.SetFlags(f)
}

type runClient interface {
	Run(run params.RunParams) ([]params.RunResult, error)
	Close() error
}

var newRunClient = func(env envcmd.EnvCommandBase) (runClient, error) {
	return env.NewAPIClient()
}

// Run implements Command.Run.
func (c *collectMetricsCommand) Run(ctx *cmd.Context) error {
	runnerClient, err := newRunClient(c.EnvCommandBase)
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
		if r.Error == "" {
			go func() {
				spoolDir := strings.Trim(string(r.Stdout), " \t\n")
				if spoolDir == "" {
					fmt.Println("failed to collect metrics for unit:", r.UnitId)
					resultChannel <- r.UnitId
					return
				}
				sendParams := params.RunParams{
					Timeout:  3 * time.Second,
					Units:    []string{r.UnitId},
					Commands: fmt.Sprintf("echo %v | sudo nc -U ../metrics-send.socket", spoolDir),
				}
				sendResults, err := runnerClient.Run(sendParams)
				if err != nil || len(sendResults) != 1 || sendResults[0].Error != "" {
					fmt.Println("failed to send metrics for unit:", r.UnitId)
				}
				resultChannel <- r.UnitId
			}()
		} else {
			fmt.Println("failed to collect metrics for unit:", r.UnitId)
		}

	}

	for range runResults {
		select {
		case u := <-resultChannel:
		case <-time.After(3 * time.Second):
			fmt.Println("wait result timeout")
			break
		}
	}
	return nil
}
