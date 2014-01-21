// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
)

// EnvironMachinesWatcher implements a common WatchEnvironMachines
// method for use by various facades.
type EnvironMachinesWatcher struct {
	st          state.EnvironMachinesWatcher
	resources   *Resources
	getCanWatch GetAuthFunc
}

// NewEnvironMachinesWatcher returns a new EnvironMachinesWatcher. The
// GetAuthFunc will be used on each invocation of WatchUnits to
// determine current permissions.
func NewEnvironMachinesWatcher(st state.EnvironMachinesWatcher, resources *Resources, getCanWatch GetAuthFunc) *EnvironMachinesWatcher {
	return &EnvironMachinesWatcher{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// environment.
func (e *EnvironMachinesWatcher) WatchEnvironMachines() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	canWatch, err := e.getCanWatch()
	if err != nil {
		return params.StringsWatchResult{}, err
	}
	if !canWatch("") {
		return result, ErrPerm
	}
	watch := e.st.WatchEnvironMachines()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = e.resources.Register(watch)
		result.Changes = changes
	} else {
		err := watcher.MustErr(watch)
		return result, fmt.Errorf("cannot obtain initial environment machines: %v", err)
	}
	return result, nil
}
