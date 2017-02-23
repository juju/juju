// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacadeForFeature("RemoteFirewaller", 1, NewStateRemoteFirewallerAPI, feature.CrossModelRelations)
}

// FirewallerAPI provides access to the Remote Firewaller API facade.
type FirewallerAPI struct {
	st         State
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewStateRemoteFirewallerAPI creates a new server-side RemoteFirewallerAPI facade.
func NewStateRemoteFirewallerAPI(ctx facade.Context) (*FirewallerAPI, error) {
	return NewRemoteFirewallerAPI(stateShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}

// NewRemoteFirewallerAPI creates a new server-side FirewallerAPI facade.
func NewRemoteFirewallerAPI(
	st State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*FirewallerAPI, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &FirewallerAPI{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// WatchSubnets creates a strings watcher that notifies of the addition,
// removal, and lifecycle changes of subnets in the model.
func (f *FirewallerAPI) WatchSubnets() (params.StringsWatchResult, error) {
	var result params.StringsWatchResult

	watch := f.st.WatchSubnets()
	// Consume the initial event and forward it to the result.
	initial, ok := <-watch.Changes()
	if !ok {
		return params.StringsWatchResult{}, watcher.EnsureErr(watch)
	}
	result.StringsWatcherId = f.resources.Register(watch)
	result.Changes = initial
	return result, nil
}
