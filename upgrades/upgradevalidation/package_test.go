// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/upgrades/upgradevalidation StatePool,State,Model
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/lxd_mock.go github.com/juju/juju/provider/lxd ServerFactory,Server

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
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
)
