// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/state"
)

var (
	GetZone                = &getZone
	WatchStorageAttachment = watchStorageAttachment

	_ meterstatus.MeterStatus = (*UniterAPI)(nil)
)

type (
	Backend                    backend
	StorageStateInterface      storageInterface
	StorageVolumeInterface     = storageVolumeInterface
	StorageFilesystemInterface = storageFilesystemInterface
)

func SetNewContainerBrokerFunc(api *UniterAPI, newBroker caas.NewContainerBrokerFunc) {
	api.containerBrokerFunc = newBroker
}

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchGetStorageStateError(patcher patcher, err error) {
	patcher.PatchValue(&GetStorageState, func(st *state.State) (storageAccess, error) { return nil, err })
}

func (n *NetworkInfoIAAS) MachineNetworkInfos() (map[string][]NetInfoAddress, error) {
	err := n.populateMachineAddresses()
	return n.machineAddresses, err
}
