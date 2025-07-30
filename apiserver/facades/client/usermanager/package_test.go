// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/user"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/client/usermanager AccessService,ModelService
//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination block_mock_test.go github.com/juju/juju/apiserver/common BlockCommandService

func newUserUUID(c *tc.C) user.UUID {
	uuid, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}
