// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// OpenedPortsWatcher implements a method WatchOpenedPorts
// that can be used by various facades.
type OpenedPortsWatcher struct {
	st          state.PortsWatcher
	resources   *common.Resources
	getCanWatch common.GetAuthFunc
}

// NewOpenedPortsWatcher returns a new OpenedPortsWatcher.
func NewOpenedPortsWatcher(st state.PortsWatcher, resources *common.Resources, getCanWatch common.GetAuthFunc) *OpenedPortsWatcher {
	return &OpenedPortsWatcher{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

// WatchOpenedPorts returns a StringsWatcher that observes the changes in
// the openedPorts configuration.
func (o *OpenedPortsWatcher) WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{}
	result.Results = make([]params.StringsWatchResult, len(args.Entities))
	for i, entity := range args.Entities {
		watcherResult, err := o.watchOneEnvOpenedPorts(entity)
		result.Results[i] = watcherResult
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (o *OpenedPortsWatcher) watchOneEnvOpenedPorts(arg params.Entity) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}

	canWatch, err := o.getCanWatch()
	if err != nil {
		return nothing, err
	}

	// Using empty string for the id of the current environment.

	if arg.Tag != "" {
		envTag, err := names.ParseEnvironTag(arg.Tag)
		if err != nil {
			return nothing, err
		}
		if !canWatch(envTag) {
			return nothing, common.ErrPerm
		}
	}

	watch := o.st.WatchOpenedPorts()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: o.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.MustErr(watch)
}
