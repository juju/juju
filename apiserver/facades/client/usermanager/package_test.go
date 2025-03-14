// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/client/usermanager AccessService,ModelService
//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination block_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
