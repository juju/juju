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
	"github.com/juju/juju/cmd/modelcmd"
)

type State struct {
	Found      bool
	EntityInfo params.EntityInfo
	Complete   bool
}

type WaitForFunc func(string, State, []params.Delta) State

type waitForCommandBase struct {
	modelcmd.ModelCommandBase

	newWatchAllAPIFunc func() (WatchAllAPI, error)
	waitForFn          WaitForFunc

	name    string
	timeout time.Duration
}

// SetFlags implements Command.SetFlags.
func (c *waitForCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
}

// Run implements Command.Run.
func (c *waitForCommandBase) Run(ctx *cmd.Context) error {
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

	timeout := make(chan struct{})
	go func() {
		select {
		case <-time.After(c.timeout):
			close(timeout)
			watcher.Stop()
		}
	}()

	var state State

	for {
		deltas, err := watcher.Next()
		if err != nil {
			select {
			case <-timeout:
				return errors.Errorf("timed out waiting for %q to reach goal state", c.name)
			default:
				return errors.Trace(err)
			}
		}

		if state = c.waitForFn(c.name, state, deltas); state.Complete {
			return nil
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
