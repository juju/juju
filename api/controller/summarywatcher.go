// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// SummaryWatcher holds information allowing us to get ModelAbstract
// results for the models the user can see.
type SummaryWatcher struct {
	objType string
	caller  base.APICaller
	id      *string
}

// NewSummaryWatcher returns a SummaryWatcher instance which is in response
// to either a WatchModelSummaries call for all models a user can see, or
// WatchAllModelSummaries by a controller superuser.
func NewSummaryWatcher(caller base.APICaller, id *string) *SummaryWatcher {
	return newSummaryWatcher("ModelSummaryWatcher", caller, id)
}

func newSummaryWatcher(objType string, caller base.APICaller, id *string) *SummaryWatcher {
	return &SummaryWatcher{
		objType: objType,
		caller:  caller,
		id:      id,
	}
}

// Next returns a slice of ModelAbstracts. A new abstract is returned for a
// model if any part of the abstract changes. No indication is given however to
// which bit has changed. It will block until there is information to return.
func (watcher *SummaryWatcher) Next() ([]params.ModelAbstract, error) {
	var info params.SummaryWatcherNextResults
	err := watcher.caller.APICall(
		watcher.objType,
		watcher.caller.BestFacadeVersion(watcher.objType),
		*watcher.id,
		"Next",
		nil, &info,
	)
	return info.Models, err
}

// Stop shutdowns down a summary watcher.
func (watcher *SummaryWatcher) Stop() error {
	return watcher.caller.APICall(
		watcher.objType,
		watcher.caller.BestFacadeVersion(watcher.objType),
		*watcher.id,
		"Stop",
		nil, nil,
	)
}
