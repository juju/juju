// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/state"
)

var (
	GetZone                = &getZone
	WatchStorageAttachment = watchStorageAttachment

	NewUniterAPI             = newUniterAPI
	NewUniterAPIWithServices = newUniterAPIWithServices
)

type (
	Backend                    backend
	StorageStateInterface      storageInterface
	StorageVolumeInterface     = storageVolumeInterface
	StorageFilesystemInterface = storageFilesystemInterface
	BlockDeviceService         = blockDeviceService
)

func NewStorageAPI(
	backend backend,
	storage storageAccess,
	blockDeviceService blockDeviceService,
	resources facade.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {
	return newStorageAPI(backend, storage, blockDeviceService, resources, accessUnit)
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

func (n *NetworkInfoIAAS) MachineNetworkInfos() (map[string][]NetInfoAddress, error) {
	err := n.populateMachineAddresses()
	return n.machineAddresses, err
}
