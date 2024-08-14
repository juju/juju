// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/application/service ApplicationState,Broker,CharmState,WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination caas_mock_test.go github.com/juju/juju/caas Application
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
