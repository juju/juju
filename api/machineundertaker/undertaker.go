// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/network"
)

// NewWatcherFunc exists to let us test WatchMachineRemovals.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// API provides access to the machine undertaker API facade.
type API struct {
	facade     base.FacadeCaller
	modelTag   names.ModelTag
	newWatcher NewWatcherFunc
}

// NewAPI creates a new client-side machine undertaker facade.
func NewAPI(caller base.APICaller, newWatcher NewWatcherFunc) (*API, error) {
	modelTag, ok := caller.ModelTag()
	if !ok {
		return nil, errors.New("machine undertaker client requires a model API connection")
	}
	api := API{
		facade:     base.NewFacadeCaller(caller, "MachineUndertaker"),
		modelTag:   modelTag,
		newWatcher: newWatcher,
	}
	return &api, nil
}

// AllMachineRemovals returns all the machines that have been marked
// ready to clean up.
func (api *API) AllMachineRemovals() ([]names.MachineTag, error) {
	var results params.EntitiesResults
	args := wrapEntities(api.modelTag)
	err := api.facade.FacadeCall("AllMachineRemovals", &args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	machines := make([]names.MachineTag, len(result.Entities))
	for i, entity := range result.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		machines[i] = tag
	}
	return machines, nil
}

// GetProviderInterfaceInfo gets the provider details for all of the
// interfaces for one machine.
func (api *API) GetProviderInterfaceInfo(machine names.MachineTag) ([]network.ProviderInterfaceInfo, error) {
	var result params.ProviderInterfaceInfoResults
	args := wrapEntities(machine)
	err := api.facade.FacadeCall("GetMachineProviderInterfaceInfo", &args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected one result, got %d", len(result.Results))
	}
	item := result.Results[0]
	if item.MachineTag != machine.String() {
		return nil, errors.Errorf("expected interface info for %s but got %s", machine, item.MachineTag)
	}
	infos := make([]network.ProviderInterfaceInfo, len(item.Interfaces))
	for i, info := range item.Interfaces {
		infos[i].InterfaceName = info.InterfaceName
		infos[i].MACAddress = info.MACAddress
		infos[i].ProviderId = corenetwork.Id(info.ProviderId)
	}
	return infos, nil
}

// CompleteRemoval finishes the removal of the machine in the database
// after any provider resources are cleaned up.
func (api *API) CompleteRemoval(machine names.MachineTag) error {
	args := wrapEntities(machine)
	return api.facade.FacadeCall("CompleteMachineRemovals", &args, nil)
}

// WatchMachineRemovals registers to be notified when a machine
// removal is requested.
func (api *API) WatchMachineRemovals() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := wrapEntities(api.modelTag)
	err := api.facade.FacadeCall("WatchMachineRemovals", &args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return nil, errors.Trace(result.Error)
	}
	w := api.newWatcher(api.facade.RawAPICaller(), result)
	return w, nil
}

func wrapEntities(tag names.Tag) params.Entities {
	return params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
}
