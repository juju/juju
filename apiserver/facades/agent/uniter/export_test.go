// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

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
	WatchStorageAttachment = watchStorageAttachment

	NewUniterAPI             = newUniterAPI
	NewUniterAPIv19          = newUniterAPIv19
	NewUniterAPIWithServices = newUniterAPIWithServices
)

type (
	StorageStateInterface      storageInterface
	StorageVolumeInterface     = storageVolumeInterface
	StorageFilesystemInterface = storageFilesystemInterface
	BlockDeviceService         = blockDeviceService
)

func NewTestAPI(
	c *tc.C,
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
	storage storageAccess,
	blockDeviceService blockDeviceService,
	applicationService ApplicationService,
	resources facade.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {
	return newStorageAPI(storage, blockDeviceService, applicationService, resources, accessUnit)
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
