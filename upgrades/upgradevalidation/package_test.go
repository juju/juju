// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelmanager/upgradevalidation StatePool,State,Model

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var (
	CheckNoWinMachinesForModel                  = checkNoWinMachinesForModel
	GetCheckUpgradeSeriesLockForModel           = getCheckUpgradeSeriesLockForModel
	GetCheckTargetVersionForModel               = getCheckTargetVersionForModel
	CheckModelMigrationModeForControllerUpgrade = checkModelMigrationModeForControllerUpgrade
	CheckMongoStatusForControllerUpgrade        = checkMongoStatusForControllerUpgrade
	CheckMongoVersionForControllerModel         = checkMongoVersionForControllerModel
)
