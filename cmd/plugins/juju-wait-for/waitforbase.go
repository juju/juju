// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

type waitForCommandBase struct {
	modelcmd.ModelCommandBase

	newWatchAllAPIFunc func() (WatchAllAPI, error)
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
