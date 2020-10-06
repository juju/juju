// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package api exists because we can't generate mocks in the main namespace.
package api

import "github.com/juju/juju/apiserver/params"

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
