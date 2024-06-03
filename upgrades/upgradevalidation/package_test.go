// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/upgrades/upgradevalidation StatePool,State,Model,Machine
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/lxd_mock.go github.com/juju/juju/provider/lxd ServerFactory,Server

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

var (
	CheckForDeprecatedUbuntuSeriesForModel      = checkForDeprecatedUbuntuSeriesForModel
	CheckNoWinMachinesForModel                  = checkNoWinMachinesForModel
	GetCheckUpgradeSeriesLockForModel           = getCheckUpgradeSeriesLockForModel
	GetCheckTargetVersionForModel               = getCheckTargetVersionForModel
	CheckModelMigrationModeForControllerUpgrade = checkModelMigrationModeForControllerUpgrade
	CheckMongoStatusForControllerUpgrade        = checkMongoStatusForControllerUpgrade
	CheckMongoVersionForControllerModel         = checkMongoVersionForControllerModel
	GetCheckForLXDVersion                       = getCheckForLXDVersion
	CheckForCharmStoreCharms                    = checkForCharmStoreCharms
	CheckFanNetworksAndContainers               = checkFanNetworksAndContainers
)
