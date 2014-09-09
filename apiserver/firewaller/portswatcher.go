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

// OpenedPortsWatcherFactory implements a method WatchOpenedPorts
// that can be used by various facades.
type OpenedPortsWatcherFactory struct {
	st          state.PortsWatcher
	resources   *common.Resources
	getCanWatch common.GetAuthFunc
}

// NewOpenedPortsWatcherFactory returns a new OpenedPortsWatcherFactory.
func NewOpenedPortsWatcherFactory(st state.PortsWatcher, resources *common.Resources, getCanWatch common.GetAuthFunc) *OpenedPortsWatcherFactory {
	return &OpenedPortsWatcherFactory{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

// WatchOpenedPorts returns a StringsWatcher that observes the changes in
// the openedPorts configuration.
func (o *OpenedPortsWatcherFactory) WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error) {
	canWatch, err := o.getCanWatch()
	if err != nil {
		return params.StringsWatchResults{}, err
	}

	result := params.StringsWatchResults{}
	result.Results = make([]params.StringsWatchResult, len(args.Entities))
	for i, entity := range args.Entities {
		result.Results[i] = o.watchOneEnvOpenedPorts(entity, canWatch)
	}
	return result, nil
}

func (o *OpenedPortsWatcherFactory) watchOneEnvOpenedPorts(arg params.Entity, canWatch common.AuthFunc) params.StringsWatchResult {
	envTag, err := names.ParseEnvironTag(arg.Tag)
	if err != nil {
		return params.StringsWatchResult{Error: common.ServerError(err)}
	}
	if !canWatch(envTag) {
		return params.StringsWatchResult{Error: common.ServerError(common.ErrPerm)}
	}

	watch := o.st.WatchOpenedPorts()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: o.resources.Register(watch),
			Changes:          changes,
		}
	}
	return params.StringsWatchResult{Error: common.ServerError(watcher.MustErr(watch))}
}
