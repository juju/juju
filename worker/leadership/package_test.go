// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package leadership_test -destination package_mock_test.go github.com/juju/juju/core/leadership Claimer

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
