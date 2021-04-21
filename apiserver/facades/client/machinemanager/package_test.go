// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Leadership
//go:generate go run github.com/golang/mock/mockgen -package machinemanager -destination upgrade_series_mock_test.go github.com/juju/juju/apiserver/facades/client/machinemanager Authorizer,UpgradeSeries,UpgradeSeriesState,UpgradeSeriesValidator
//go:generate go run github.com/golang/mock/mockgen -package machinemanager -destination types_mock_test.go github.com/juju/juju/apiserver/facades/client/machinemanager Machine,Application,Unit,Charm,CharmhubClient

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
