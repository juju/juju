// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"sort"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// AllWatch represents methods used on the AllWatcher
// Primarily to facilitate mock tests.
type AllWatch interface {
	Next() ([]params.Delta, error)
	Stop() error
}

// AllWatcher holds information allowing us to get Deltas describing
// changes to the entire model or all models (depending on
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

// NewAllModelWatcher returns an AllWatcher instance which interacts
// with a watcher created by the WatchAllModels API call.
//
// There should be no need to call this from outside of the api
// package. It is only used by Client.WatchAllModels in
// api/controller.
func NewAllModelWatcher(caller base.APICaller, id *string) *AllWatcher {
	return newAllWatcher("AllModelWatcher", caller, id)
}

func newAllWatcher(objType string, caller base.APICaller, id *string) *AllWatcher {
	return &AllWatcher{
		objType: objType,
		caller:  caller,
		id:      id,
	}
}

// Next returns a new set of deltas from a watcher previously created
// by the WatchAll or WatchAllModels API calls. It will block until
// there are deltas to return.
func (watcher *AllWatcher) Next() ([]params.Delta, error) {
	var info params.AllWatcherNextResults
	err := watcher.caller.APICall(
		context.TODO(),
		watcher.objType,
		watcher.caller.BestFacadeVersion(watcher.objType),
		*watcher.id,
		"Next",
		nil, &info,
	)
	// We'll order the deltas so relation changes come last.
	// This allows the callers like the Dashboard to process changes
	// in the right order.
	sort.Sort(orderedDeltas(info.Deltas))
	return info.Deltas, err
}

type orderedDeltas []params.Delta

func (o orderedDeltas) Len() int {
	return len(o)
}

func (o orderedDeltas) kindPriority(kind string) int {
	switch kind {
	case "machine":
		return 1
	case "application":
		return 2
	case "relation":
		return 3
	}
	return 0
}

func (o orderedDeltas) Less(i, j int) bool {
	// All we care about is having relation deltas last.
	// We'll add extra checks though to make the order more
	// deterministic for tests.
	pi, pj := o.kindPriority(o[i].Entity.EntityId().Kind), o.kindPriority(o[j].Entity.EntityId().Kind)
	if pi == pj {
		return o[i].Entity.EntityId().Id < o[j].Entity.EntityId().Id
	}
	return pi < pj
}

func (o orderedDeltas) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// Stop shutdowns down a watcher previously created by the WatchAll or
// WatchAllModels API calls
func (watcher *AllWatcher) Stop() error {
	return watcher.caller.APICall(
		context.TODO(),
		watcher.objType,
		watcher.caller.BestFacadeVersion(watcher.objType),
		*watcher.id,
		"Stop",
		nil, nil,
	)
}
