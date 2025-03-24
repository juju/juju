// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/model/service ControllerState,EnvironVersionProvider,ModelDeleter,ModelState,State,ModelResourcesProvider,WatcherFactory

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
