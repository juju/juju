// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package peergrouper -destination controllerconfig_mock_test.go github.com/juju/juju/internal/worker/peergrouper ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -package peergrouper -destination service_mock_test.go github.com/juju/juju/internal/servicefactory ControllerServiceFactory

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
