// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

const instancePollerFacade = "InstancePoller"

// API provides access to the InstancePoller API facade.
type API struct {
	*common.ModelWatcher

	facade base.FacadeCaller
}

// NewAPI creates a new client-side InstancePoller facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, instancePollerFacade)
	return &API{
		ModelWatcher: common.NewModelWatcher(facadeCaller),
		facade:       facadeCaller,
	}
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (api *API) Machine(tag names.MachineTag) (*Machine, error) {
	life, err := common.OneLife(api.facade, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Machine{api.facade, tag, life}, nil
}

var newStringsWatcher = apiwatcher.NewStringsWatcher

// WatchModelMachines returns a StringsWatcher reporting changes to the
// machine life or agent start timestamps.
func (api *API) WatchModelMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := api.facade.FacadeCall("WatchModelMachineStartTimes", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return newStringsWatcher(api.facade.RawAPICaller(), result), nil
}
