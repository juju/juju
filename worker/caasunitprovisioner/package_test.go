// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package caasunitprovisioner -destination package_mock_test.go github.com/juju/juju/worker/caasunitprovisioner ProvisioningStatusSetter

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}
