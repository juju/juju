// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination controllerconfig_mock_test.go github.com/juju/juju/internal/worker/apiserver ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination service_mock_test.go github.com/juju/juju/internal/servicefactory ServiceFactoryGetter
//go:generate go run go.uber.org/mock/mockgen -package apiserver_test -destination status_mock_test.go github.com/juju/juju/core/status StatusHistoryFactory,StatusHistorySetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
