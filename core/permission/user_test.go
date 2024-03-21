// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
)

type userSuite struct{}

var _ = gc.Suite(&userSuite{})

func (s *userSuite) TestControllerForAccess(c *gc.C) {
	spec := permission.ControllerForAccess(permission.ReadAccess)
	c.Assert(spec.Target.Key, gc.Equals, database.ControllerNS)
}
