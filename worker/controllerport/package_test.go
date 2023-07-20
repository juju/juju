// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerport_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package controllerport_test -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -package controllerport_test -destination controller_config_service_mock_test.go github.com/juju/juju/worker/controllerport ControllerConfigService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
