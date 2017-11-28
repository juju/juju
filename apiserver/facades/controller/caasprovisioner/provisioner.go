// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

type API struct {
	*common.PasswordChanger

	auth      facade.Authorizer
	resources facade.Resources

	state CAASProvisionerState
}

// NewStateCAASProvisionerAPI provides the signature required for facade registration.
func NewStateCAASProvisionerAPI(ctx facade.Context) (*API, error) {

	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewCAASProvisionerAPI(resources, authorizer, ctx.State())
}

// NewCAASProvisionerAPI returns a new CAASProvisionerAPI facade.
func NewCAASProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASProvisionerState,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		PasswordChanger: common.NewPasswordChanger(st, common.AuthAlways()),
		auth:            authorizer,
		resources:       resources,
		state:           st,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch CAAS applications
// deployed to this model.
func (a *API) WatchApplications() (params.StringsWatchResult, error) {
	watch := a.state.WatchApplications()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}
