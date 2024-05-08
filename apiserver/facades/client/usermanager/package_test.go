// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package usermanager_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/client/usermanager UserService

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func newUserUUID(c *gc.C) user.UUID {
	uuid, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
