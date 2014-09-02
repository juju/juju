// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// AllWatcher holds information allowing us to get Deltas describing changes
// to the entire environment.
type AllWatcher struct {
	caller base.APICaller
	id     *string
}

func newAllWatcher(caller base.APICaller, id *string) *AllWatcher {
	return &AllWatcher{caller, id}
}

func (watcher *AllWatcher) Next() ([]params.Delta, error) {
	info := new(params.AllWatcherNextResults)
	err := watcher.caller.APICall(
		"AllWatcher", watcher.caller.BestFacadeVersion("AllWatcher"),
		*watcher.id, "Next", nil, info)
	return info.Deltas, err
}

func (watcher *AllWatcher) Stop() error {
	return watcher.caller.APICall(
		"AllWatcher", watcher.caller.BestFacadeVersion("AllWatcher"),
		*watcher.id, "Stop", nil, nil)
}
