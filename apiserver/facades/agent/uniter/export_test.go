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

	NewUniterAPI    = newUniterAPI
	NewUniterAPIV16 = newUniterAPIV16
	NewUniterAPIV14 = newUniterAPIV14
	NewUniterAPIV12 = newUniterAPIV12
	NewUniterAPIV8  = newUniterAPIV8
	NewUniterAPIV6  = newUniterAPIV6
	NewUniterAPIV5  = newUniterAPIV5
	NewUniterAPIV4  = newUniterAPIV4

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

func (n *NetworkInfoIAAS) MachineNetworkInfos() (map[string][]NetInfoAddress, error) {
	err := n.populateMachineAddresses()
	return n.machineAddresses, err
}
