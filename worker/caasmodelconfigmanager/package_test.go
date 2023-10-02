// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package caasmodelconfigmanager_test -destination facade_mock_test.go github.com/juju/juju/worker/caasmodelconfigmanager ControllerConfigService,CAASBroker,Registry,ImageRepo

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
