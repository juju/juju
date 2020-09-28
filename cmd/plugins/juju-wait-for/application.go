// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/core/status"
)

func newApplicationCommand() cmd.Command {
	cmd := &applicationCommand{}
	cmd.newWatchAllAPIFunc = func() (WatchAllAPI, error) {
		client, err := cmd.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return watchAllAPIShim{
			Client: client,
		}, nil
	}
	return modelcmd.Wrap(cmd)
}

const applicationCommandDoc = `
Wait for a given application to reach a goal state.
arguments:
name
   application name identifier

options:
--query (= "life=alive; status=active")
   query represents the goal state of a given application
`

// applicationCommand defines a command for waiting for applications.
type applicationCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration

	found   bool
	appInfo *params.ApplicationInfo
}

// Info implements Command.Info.
func (c *applicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "application",
		Args:    "[<name>]",
		Purpose: "wait for an application to reach a goal state",
		Doc:     applicationCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *applicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", "life=alive; status=active", "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
}

// Init implements Command.Init.
func (c *applicationCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("application name must be supplied when waiting for an application")
	}
	if len(args) != 1 {
		return errors.New("only one application name can be supplied as an argument to this command")
	}
	if ok := names.IsValidApplication(args[0]); !ok {
		return errors.Errorf("%q is not valid application name", args[0])
	}
	c.name = args[0]

	return nil
}

func (c *applicationCommand) Run(ctx *cmd.Context) error {
	query, err := query.Parse(c.query)
	if err != nil {
		return errors.Trace(err)
	}

	client, err := c.newWatchAllAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}

	strategy := &Strategy{
		Client:  client,
		Timeout: c.timeout,
	}
	err = strategy.Run(c.name, query, c.waitFor)
	return errors.Trace(err)
}

func (c *applicationCommand) waitFor(name string, deltas []params.Delta, fn query.Predicate) bool {
	for _, delta := range deltas {
		switch entityInfo := delta.Entity.(type) {
		case *params.ApplicationInfo:
			if entityInfo.Name == name {
				if fn(entityInfo) {
					return true
				}
				c.found = true
				c.appInfo = entityInfo
				break
			}
		}
	}

	if !c.found {
		logger.Infof("application %q not found, waiting...", name)
		return false
	}

	var logOutput bool
	currentStatus := c.appInfo.Status.Current

	// If the application is unset, the derive it from the units.
	if currentStatus.String() == "unset" {
		statuses := make([]status.StatusInfo, 0)
		for _, delta := range deltas {
			switch entityInfo := delta.Entity.(type) {
			case *params.UnitInfo:
				if entityInfo.Application == name {
					logOutput = true

					agentStatus := entityInfo.WorkloadStatus
					statuses = append(statuses, status.StatusInfo{
						Status: agentStatus.Current,
					})
				}
			}
		}

		derived := status.DeriveStatus(statuses)
		currentStatus = derived.Status
	}

	c.appInfo.Status.Current = currentStatus

	if fn(c.appInfo) {
		return true
	}

	if logOutput {
		logger.Infof("application %q found with %q, waiting for goal state", name, currentStatus)
	}

	return false
}
