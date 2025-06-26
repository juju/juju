// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type localControllerInfoFunc func(context.Context) ([]string, string, error)
type publicDNSAddressFunc func(context.Context) (string, error)
type watchLocalControllerInfoFunc func(ctx context.Context) (watcher.NotifyWatcher, error)

// CrossControllerAPI provides access to the CrossModelRelations API facade.
type CrossControllerAPI struct {
	localControllerInfo      localControllerInfoFunc
	publicDNSAddress         publicDNSAddressFunc
	watchLocalControllerInfo watchLocalControllerInfoFunc
	watcherRegistry          facade.WatcherRegistry
}

// NewCrossControllerAPI returns a new server-side CrossControllerAPI facade.
func NewCrossControllerAPI(
	watcherRegistry facade.WatcherRegistry,
	localControllerInfo localControllerInfoFunc,
	publicDNSAddress publicDNSAddressFunc,
	watchLocalControllerInfo watchLocalControllerInfoFunc,
) (*CrossControllerAPI, error) {
	return &CrossControllerAPI{
		watcherRegistry:          watcherRegistry,
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
	w, err := api.watchLocalControllerInfo(ctx)
	if err != nil {
		return results, errors.Capture(err)
	}
	results.Results[0].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
	results.Results[0].Error = apiservererrors.ServerError(err)
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
