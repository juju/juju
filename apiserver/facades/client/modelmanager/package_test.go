// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelmanager StatePool,State,Model,MongoSession
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common ToolsFinder,BlockCheckerInterface
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron

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
