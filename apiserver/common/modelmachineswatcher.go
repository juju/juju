// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"time"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ModelMachinesWatcher implements a common WatchModelMachines
// method for use by various facades.
type ModelMachinesWatcher struct {
	st         state.ModelMachinesWatcher
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewModelMachinesWatcher returns a new ModelMachinesWatcher. The
// GetAuthFunc will be used on each invocation of WatchUnits to
// determine current permissions.
func NewModelMachinesWatcher(st state.ModelMachinesWatcher, resources facade.Resources, authorizer facade.Authorizer) *ModelMachinesWatcher {
	return &ModelMachinesWatcher{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// model.
func (e *ModelMachinesWatcher) WatchModelMachines() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !e.authorizer.AuthController() {
		return result, ErrPerm
	}
	watch := e.st.WatchModelMachines()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = e.resources.Register(watch)
		result.Changes = changes
	} else {
		err := watcher.EnsureErr(watch)
		return result, fmt.Errorf("cannot obtain initial model machines: %v", err)
	}
	return result, nil
}

// WatchModelMachineStartTimes watches the non-container machines in the model
// for changes to the Life or AgentStartTime fields and reports them as a batch
// after the specified quiesceInterval time has passed without seeing any new
// change events.
func (e *ModelMachinesWatcher) WatchModelMachineStartTimes() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !e.authorizer.AuthController() {
		return result, ErrPerm
	}
	watch := e.st.WatchModelMachineStartTimes(500 * time.Millisecond)
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = e.resources.Register(watch)
		result.Changes = changes
	} else {
		err := watcher.EnsureErr(watch)
		return result, fmt.Errorf("cannot obtain initial model machines: %v", err)
	}
	return result, nil
}
