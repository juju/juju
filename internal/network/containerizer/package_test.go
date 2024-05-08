// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package containerizer -destination bridgepolicy_mock_test.go github.com/juju/juju/internal/network/containerizer Container,Address,Subnet,LinkLayerDevice
func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
