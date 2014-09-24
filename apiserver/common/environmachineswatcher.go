// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// EnvironMachinesWatcher implements a common WatchEnvironMachines
// method for use by various facades.
type EnvironMachinesWatcher struct {
	st         state.EnvironMachinesWatcher
	resources  *Resources
	authorizer Authorizer
}

// NewEnvironMachinesWatcher returns a new EnvironMachinesWatcher. The
// GetAuthFunc will be used on each invocation of WatchUnits to
// determine current permissions.
func NewEnvironMachinesWatcher(st state.EnvironMachinesWatcher, resources *Resources, authorizer Authorizer) *EnvironMachinesWatcher {
	return &EnvironMachinesWatcher{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}
}

// WatchEnvironMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// environment.
func (e *EnvironMachinesWatcher) WatchEnvironMachines() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !e.authorizer.AuthEnvironManager() {
		return result, ErrPerm
	}
	watch := e.st.WatchEnvironMachines()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = e.resources.Register(watch)
		result.Changes = changes
	} else {
		err := watcher.EnsureErr(watch)
		return result, fmt.Errorf("cannot obtain initial environment machines: %v", err)
	}
	return result, nil
}
