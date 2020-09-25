// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
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
	cmd.waitForFn = cmd.waitFor
	return modelcmd.Wrap(cmd)
}

const applicationCommandDoc = `
Wait for a given application to reach a goal state.

arguments:
name
   application name identifier

options:
--status (= "active")
   status of the application to wait-for
--life (= "alive")
   life of the application to wait-for
`

// applicationCommand stores image metadata in Juju environment.
type applicationCommand struct {
	waitForCommandBase

	life   string
	status string

	predicate Predicate
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

	predicates := map[string]Predicate{
		"life":   LifePredicate("alive"),
		"status": StatusPredicate("active"),
	}
	if c.life != "" {
		predicates["life"] = LifePredicate(c.life)
	}
	if c.status != "" {
		predicates["status"] = StatusPredicate(c.status)
	}
	c.predicate = ComposePredicates(predicates)

	return nil
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
	f.StringVar(&c.life, "life", "", "goal state for the life of a application")
	f.StringVar(&c.status, "status", "", "goal state for the status of a application")
}

func (c *applicationCommand) waitFor(name string, state State, deltas []params.Delta) State {
	for _, delta := range deltas {
		switch entityInfo := delta.Entity.(type) {
		case *params.ApplicationInfo:
			if entityInfo.Name == name {
				if c.predicate(entityInfo) {
					state.Complete = true
					return state
				}
				state.Found = true
				state.EntityInfo = entityInfo
				break
			}
		}
	}

	if !state.Found {
		logger.Infof("application %q not found, waiting...", name)
		return state
	}

	var logOutput bool
	appInfo := state.EntityInfo.(*params.ApplicationInfo)
	currentStatus := appInfo.Status.Current

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

	appInfo.Status.Current = currentStatus

	if c.predicate(appInfo) {
		state.Complete = true
		return state
	}

	if logOutput {
		logger.Infof("application %q found with %q, waiting for goal state", name, currentStatus)
	}
	return state
}
