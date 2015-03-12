// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

const networkerFacade = "Networker"

// State provides access to an networker worker's view of the state.
//
// NOTE: This is defined as an interface due to PPC64 bug #1424669 -
// if it were a type build errors happen (due to a linker bug).
type State interface {
	MachineNetworkConfig(names.MachineTag) ([]network.InterfaceInfo, error)
	WatchInterfaces(names.MachineTag) (watcher.NotifyWatcher, error)
}

var _ State = (*state)(nil)

// state implements State.
type state struct {
	facade base.FacadeCaller
}

// NewState creates a new client-side Machiner facade.
func NewState(caller base.APICaller) State {
	return &state{base.NewFacadeCaller(caller, networkerFacade)}
}

// MachineNetworkConfig returns information about network interfaces to
// setup only for a single machine.
func (st *state) MachineNetworkConfig(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	var results params.MachineNetworkConfigResults
	err := st.facade.FacadeCall("MachineNetworkConfig", args, &results)
	if err != nil {
		if params.IsCodeNotImplemented(err) {
			// Fallback to former name.
			err = st.facade.FacadeCall("MachineNetworkInfo", args, &results)
		}
		if err != nil {
			// TODO: Not directly tested.
			return nil, err
		}
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		err = errors.Errorf("expected one result, got %d", len(results.Results))
		return nil, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	interfaceInfo := make([]network.InterfaceInfo, len(result.Config))
	for i, ifaceInfo := range result.Config {
		interfaceInfo[i].DeviceIndex = ifaceInfo.DeviceIndex
		interfaceInfo[i].MACAddress = ifaceInfo.MACAddress
		interfaceInfo[i].CIDR = ifaceInfo.CIDR
		interfaceInfo[i].NetworkName = ifaceInfo.NetworkName
		interfaceInfo[i].ProviderId = network.Id(ifaceInfo.ProviderId)
		interfaceInfo[i].VLANTag = ifaceInfo.VLANTag
		interfaceInfo[i].InterfaceName = ifaceInfo.InterfaceName
		interfaceInfo[i].Disabled = ifaceInfo.Disabled
		// TODO(dimitern) Once we store all the information from
		// network.InterfaceInfo in state, change this as needed to
		// return it.
	}

	return interfaceInfo, nil
}

// WatchInterfaces returns a NotifyWatcher that notifies of changes to network
// interfaces on the machine.
func (st *state) WatchInterfaces(tag names.MachineTag) (watcher.NotifyWatcher, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	var results params.NotifyWatchResults
	err := st.facade.FacadeCall("WatchInterfaces", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		err = errors.Errorf("expected one result, got %d", len(results.Results))
		return nil, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
