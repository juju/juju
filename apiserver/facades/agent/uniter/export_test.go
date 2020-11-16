// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
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

func NewStorageAPI(
	backend backend,
	storage storageAccess,
	resources facade.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {
	return newStorageAPI(backend, storage, resources, accessUnit)
}

func SetNewContainerBrokerFunc(api *UniterAPI, newBroker caas.NewContainerBrokerFunc) {
	api.containerBrokerFunc = newBroker
}

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchGetStorageStateError(patcher patcher, err error) {
	patcher.PatchValue(&getStorageState, func(st *state.State) (storageAccess, error) { return nil, err })
}

func (n *NetworkInfoBase) MachineNetworkInfos(spaceIDs ...string) (map[string]machineNetworkInfoResult, error) {
	return n.machineNetworkInfos(spaceIDs...)
}
