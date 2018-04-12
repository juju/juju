// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.crosscontroller")

type localControllerInfoFunc func() ([]string, string, error)
type watchLocalControllerInfoFunc func() state.NotifyWatcher

// CrossControllerAPI provides access to the CrossModelRelations API facade.
type CrossControllerAPI struct {
	resources                facade.Resources
	localControllerInfo      localControllerInfoFunc
	watchLocalControllerInfo watchLocalControllerInfoFunc
}

// NewStateCrossControllerAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func NewStateCrossControllerAPI(ctx facade.Context) (*CrossControllerAPI, error) {
	st := ctx.State()
	return NewCrossControllerAPI(
		ctx.Resources(),
		func() ([]string, string, error) { return common.StateControllerInfo(st) },
		st.WatchAPIHostPortsForClients,
	)
}

// NewCrossControllerAPI returns a new server-side CrossControllerAPI facade.
func NewCrossControllerAPI(
	resources facade.Resources,
	localControllerInfo localControllerInfoFunc,
	watchLocalControllerInfo watchLocalControllerInfoFunc,
) (*CrossControllerAPI, error) {
	return &CrossControllerAPI{
		resources:                resources,
		localControllerInfo:      localControllerInfo,
		watchLocalControllerInfo: watchLocalControllerInfo,
	}, nil
}

// WatchControllerInfo creates a watcher that notifies when the API info
// for the controller changes.
func (api *CrossControllerAPI) WatchControllerInfo() (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, 1),
	}
	w := api.watchLocalControllerInfo()
	if _, ok := <-w.Changes(); !ok {
		results.Results[0].Error = common.ServerError(watcher.EnsureErr(w))
		return results, nil
	}
	results.Results[0].NotifyWatcherId = api.resources.Register(w)
	return results, nil
}

// ControllerInfo returns the API info for the controller.
func (api *CrossControllerAPI) ControllerInfo() (params.ControllerAPIInfoResults, error) {
	results := params.ControllerAPIInfoResults{
		Results: make([]params.ControllerAPIInfoResult, 1),
	}
	addrs, caCert, err := api.localControllerInfo()
	if err != nil {
		results.Results[0].Error = common.ServerError(err)
		return results, nil
	}
	results.Results[0].Addresses = addrs
	results.Results[0].CACert = caCert
	return results, nil
}
