// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package subnets -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/subnets NetworkService

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
