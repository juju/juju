// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/state"
)

var (
	GetZone                = &getZone
	WatchStorageAttachment = watchStorageAttachment

	NewUniterAPI             = newUniterAPI
	NewUniterAPIv19          = newUniterAPIv19
	NewUniterAPIWithServices = newUniterAPIWithServices
)

type (
	Backend                    backend
	StorageStateInterface      storageInterface
	StorageVolumeInterface     = storageVolumeInterface
	StorageFilesystemInterface = storageFilesystemInterface
	BlockDeviceService         = blockDeviceService
)

func NewTestAPI(
	c *gc.C,
	authorizer facade.Authorizer,
	leadership leadership.Checker,
	secretService SecretService,
	applicationService ApplicationService,
	clock clock.Clock,
) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &UniterAPI{
		auth:               authorizer,
		secretService:      secretService,
		applicationService: applicationService,
		leadershipChecker:  leadership,
		clock:              clock,
		logger:             loggertesting.WrapCheckLog(c),
	}, nil
}

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
	patcher.PatchValue(&getStorageState, func(*state.State, coremodel.ModelType) (storageAccess, error) { return nil, err })
}

func (n *NetworkInfoIAAS) MachineNetworkInfos() (map[string][]NetInfoAddress, error) {
	err := n.populateMachineAddresses()
	return n.machineAddresses, err
}
