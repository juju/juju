// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

type localControllerInfoFunc func(context.Context) ([]string, string, error)
type publicDNSAddressFunc func(context.Context) (string, error)
type watchLocalControllerInfoFunc func() state.NotifyWatcher

// CrossControllerAPI provides access to the CrossModelRelations API facade.
type CrossControllerAPI struct {
	resources                facade.Resources
	localControllerInfo      localControllerInfoFunc
	publicDNSAddress         publicDNSAddressFunc
	watchLocalControllerInfo watchLocalControllerInfoFunc
}

// NewCrossControllerAPI returns a new server-side CrossControllerAPI facade.
func NewCrossControllerAPI(
	resources facade.Resources,
	localControllerInfo localControllerInfoFunc,
	publicDNSAddress publicDNSAddressFunc,
	watchLocalControllerInfo watchLocalControllerInfoFunc,
) (*CrossControllerAPI, error) {
	return &CrossControllerAPI{
		resources:                resources,
		localControllerInfo:      localControllerInfo,
		publicDNSAddress:         publicDNSAddress,
		watchLocalControllerInfo: watchLocalControllerInfo,
	}, nil
}

// WatchControllerInfo creates a watcher that notifies when the API info
// for the controller changes.
func (api *CrossControllerAPI) WatchControllerInfo(ctx context.Context) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, 1),
	}
	w := api.watchLocalControllerInfo()
	if _, ok := <-w.Changes(); !ok {
		results.Results[0].Error = apiservererrors.ServerError(watcher.EnsureErr(w))
		return results, nil
	}
	results.Results[0].NotifyWatcherId = api.resources.Register(w)
	return results, nil
}

// ControllerInfo returns the API info for the controller.
func (api *CrossControllerAPI) ControllerInfo(ctx context.Context) (params.ControllerAPIInfoResults, error) {
	results := params.ControllerAPIInfoResults{
		Results: make([]params.ControllerAPIInfoResult, 1),
	}
	addrs, caCert, err := api.localControllerInfo(ctx)
	if err != nil {
		results.Results[0].Error = apiservererrors.ServerError(err)
		return results, nil
	}
	publicDNSAddress, err := api.publicDNSAddress(ctx)
	if err != nil {
		results.Results[0].Error = apiservererrors.ServerError(err)
		return results, nil
	}
	if publicDNSAddress == "" {
		results.Results[0].Addresses = addrs
	} else {
		results.Results[0].Addresses = append([]string{publicDNSAddress}, addrs...)
	}
	results.Results[0].CACert = caCert
	return results, nil
}
