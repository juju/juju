// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/permission"
)

type permissionSuite struct{}

var _ = gc.Suite(&permissionSuite{})

func (s *permissionSuite) TestParseModelAccessValid(c *gc.C) {
	var (
		access permission.ModelAccess
		err    error
	)

	access, err = permission.ParseModelAccess("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(access, gc.Equals, permission.ModelReadAccess)

	access, err = permission.ParseModelAccess("read")
	c.Check(err, jc.ErrorIsNil)
	c.Check(access, gc.Equals, permission.ModelReadAccess)

	access, err = permission.ParseModelAccess("write")
	c.Check(err, jc.ErrorIsNil)
	c.Check(access, gc.Equals, permission.ModelAdminAccess)

	access, err = permission.ParseModelAccess("admin")
	c.Check(err, jc.ErrorIsNil)
	c.Check(access, gc.Equals, permission.ModelAdminAccess)
}

func (s *permissionSuite) TestParseModelAccessInvalid(c *gc.C) {
	_, err := permission.ParseModelAccess("preposterous")
	c.Check(err, gc.ErrorMatches, "invalid model access permission.*")
}
