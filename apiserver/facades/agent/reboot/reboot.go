// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// RebootAPI provides access to the Upgrader API facade.
type RebootAPI struct {
	*common.RebootActionGetter
	// The ability for a machine to reboot itself is not yet used.
	*common.RebootRequester
	*common.RebootFlagClearer

	auth      facade.Authorizer
	st        *state.State
	machine   *state.Machine
	resources facade.Resources
}

// NewRebootAPI creates a new server-side RebootAPI facade.
func NewRebootAPI(ctx facade.Context) (*RebootAPI, error) {
	auth := ctx.Auth()
	if !auth.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	tag, ok := auth.GetAuthTag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got %T", auth.GetAuthTag())
	}
	st := ctx.State()
	machine, err := st.Machine(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	canAccess := func() (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}

	return &RebootAPI{
		RebootActionGetter: common.NewRebootActionGetter(st, canAccess),
		RebootRequester:    common.NewRebootRequester(st, canAccess),
		RebootFlagClearer:  common.NewRebootFlagClearer(st, canAccess),
		st:                 st,
		machine:            machine,
		resources:          ctx.Resources(),
		auth:               auth,
	}, nil
}

// WatchForRebootEvent starts a watcher to track if there is a new
// reboot request on the machines ID or any of its parents (in case we are a container).
func (r *RebootAPI) WatchForRebootEvent(ctx context.Context) (params.NotifyWatchResult, error) {
	var err error = apiservererrors.ErrPerm
	var watch state.NotifyWatcher
	var result params.NotifyWatchResult

	if r.auth.AuthOwner(r.machine.Tag()) {
		watch = r.machine.WatchForRebootEvent()
		err = nil
		// Consume the initial event. Technically, API
		// calls to Watch 'transmit' the initial event
		// in the Watch response. But NotifyWatchers
		// have no state to transmit.
		if _, ok := <-watch.Changes(); ok {
			result.NotifyWatcherId = r.resources.Register(watch)
		} else {
			err = watcher.EnsureErr(watch)
		}
	}
	result.Error = apiservererrors.ServerError(err)
	return result, nil
}
