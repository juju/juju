// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
)

// AllWatcher holds information allowing us to get Deltas describing
// changes to the entire environment or all environments (depending on
// the watcher type).
type AllWatcher struct {
	objType string
	caller  base.APICaller
	id      *string
}

// NewAllWatcher returns an AllWatcher instance which interacts with a
// watcher created by the WatchAll API call.
//
// There should be no need to call this from outside of the api
// package. It is only used by Client.WatchAll in this package.
func NewAllWatcher(caller base.APICaller, id *string) *AllWatcher {
	return newAllWatcher("AllWatcher", caller, id)
}

// NewAllEnvWatcher returns an AllWatcher instance which interacts
// with a watcher created by the WatchAllEnvs API call.
//
// There should be no need to call this from outside of the api
// package. It is only used by Client.WatchAllEnvs in
// api/systemmanager.
func NewAllEnvWatcher(caller base.APICaller, id *string) *AllWatcher {
	return newAllWatcher("AllEnvWatcher", caller, id)
}

func newAllWatcher(objType string, caller base.APICaller, id *string) *AllWatcher {
	return &AllWatcher{
		objType: objType,
		caller:  caller,
		id:      id,
	}
}

// Next returns a new set of deltas from a watcher previously created
// by the WatchAll or WatchAllEnvs API calls. It will block until
// there are deltas to return.
func (watcher *AllWatcher) Next() ([]multiwatcher.Delta, error) {
	var info params.AllWatcherNextResults
	err := watcher.caller.APICall(
		watcher.objType,
		watcher.caller.BestFacadeVersion(watcher.objType),
		*watcher.id,
		"Next",
		nil, &info,
	)
	return info.Deltas, err
}

// Stop shutdowns down a watcher previously created by the WatchAll or
// WatchAllEnvs API calls
func (watcher *AllWatcher) Stop() error {
	return watcher.caller.APICall(
		watcher.objType,
		watcher.caller.BestFacadeVersion(watcher.objType),
		*watcher.id,
		"Stop",
		nil, nil,
	)
}
