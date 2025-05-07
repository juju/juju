// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/client/usermanager AccessService,ModelService
//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination block_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func newUserUUID(c *tc.C) user.UUID {
	uuid, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
