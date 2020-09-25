// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api"
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
	return modelcmd.Wrap(cmd)
}

const applicationCommandDoc = `
Wait for a given application to reach a goal state.

arguments:
name
   application name identifier

options:
-m, --model (= "")
   juju model to operate in
--state (= "active")
   state of the application to wait-for
`

// applicationCommand stores image metadata in Juju environment.
type applicationCommand struct {
	waitForCommandBase

	newWatchAllAPIFunc func() (WatchAllAPI, error)

	Name    string
	State   string
	Timeout time.Duration
}

// Init implements Command.Init.
func (c *applicationCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("application name must be supplied when waiting for an application")
	}
	if len(args) != 1 {
		return errors.New("only one application name can be supplied as an argument to this command")
	}
	c.Name = args[0]
	return nil
}

// Info implements Command.Info.
func (c *applicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "application",
		Purpose: "wait for an application to reach a goal state",
		Doc:     applicationCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *applicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.State, "state", "active", "goal state of the application")
	f.DurationVar(&c.Timeout, "timeout", time.Minute, "how long to wait, before timing out")
}

// Run implements Command.Run.
func (c *applicationCommand) Run(ctx *cmd.Context) error {
	client, err := c.newWatchAllAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}

	watcher, err := client.WatchAll()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = watcher.Stop()
	}()

	var timedout bool
	go func() {
		select {
		case <-time.After(c.Timeout):
			timedout = true
			watcher.Stop()
		}
	}()

	var applicationFound bool
	var applicationStatus string

	for {
		deltas, err := watcher.Next()
		if err != nil {
			if timedout {
				return errors.Errorf("timedout waiting for application %q to reach goal state %q", c.Name, c.State)
			}
			return errors.Trace(err)
		}

		for _, delta := range deltas {
			switch entityInfo := delta.Entity.(type) {
			case *params.ApplicationInfo:
				if entityInfo.Name == c.Name {
					// If the application already has a status that is the valid
					// state, skip everything and just return.
					if applicationStatus = entityInfo.Status.Current.String(); applicationStatus == c.State {
						return nil
					}
					// We've not found the current status, let's attempt to
					// derive it.
					applicationFound = true
					break
				}
			}
		}

		if !applicationFound {
			logger.Infof("application %q not found, waiting...", c.Name)
			continue
		}

		var logOutput bool
		currentStatus := applicationStatus

		// If the application is unset, the derive it from the units.
		if currentStatus == "unset" {
			statuses := make([]status.StatusInfo, 0)
			for _, delta := range deltas {
				switch entityInfo := delta.Entity.(type) {
				case *params.UnitInfo:
					if entityInfo.Application == c.Name {
						logOutput = true

						agentStatus := entityInfo.WorkloadStatus
						statuses = append(statuses, status.StatusInfo{
							Status: agentStatus.Current,
						})
					}
				}
			}

			derived := status.DeriveStatus(statuses)
			currentStatus = derived.Status.String()
		}

		if currentStatus == c.State {
			return nil
		}

		if logOutput {
			logger.Infof("application %q found with %q, waiting for goal state: %q", c.Name, currentStatus, c.State)
		}
	}
}

// AllWatcher represents methods used on the AllWatcher
// Primarily to facilitate mock tests.
type AllWatcher interface {

	// Next returns a new set of deltas from a watcher previously created
	// by the WatchAll or WatchAllModels API calls. It will block until
	// there are deltas to return.
	Next() ([]params.Delta, error)

	// Stop shutdowns down a watcher previously created by the WatchAll or
	// WatchAllModels API calls
	Stop() error
}

// WatchAllAPI defines the API methods that allow the watching of a given item.
type WatchAllAPI interface {
	// WatchAll returns an AllWatcher, from which you can request the Next
	// collection of Deltas.
	WatchAll() (AllWatcher, error)
}

type watchAllAPIShim struct {
	*api.Client
}

func (s watchAllAPIShim) WatchAll() (AllWatcher, error) {
	return s.Client.WatchAll()
}
