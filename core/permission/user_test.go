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

var validateRevokeAccessTest = []struct {
	spec     permission.AccessSpec
	expected permission.Access
}{
	{
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Model}, Access: permission.AddModelAccess},
		expected: permission.WriteAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Model}, Access: permission.WriteAccess},
		expected: permission.ReadAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Model}, Access: permission.ReadAccess},
		expected: permission.NoAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Offer}, Access: permission.AdminAccess},
		expected: permission.ConsumeAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Offer}, Access: permission.ConsumeAccess},
		expected: permission.ReadAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Offer}, Access: permission.ReadAccess},
		expected: permission.NoAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Controller}, Access: permission.SuperuserAccess},
		expected: permission.LoginAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Controller}, Access: permission.LoginAccess},
		expected: permission.NoAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Cloud}, Access: permission.AdminAccess},
		expected: permission.AddModelAccess,
	}, {
		spec:     permission.AccessSpec{Target: permission.ID{ObjectType: permission.Cloud}, Access: permission.AddModelAccess},
		expected: permission.NoAccess,
	},
}

func (*userSuite) TestRevokeAccess(c *gc.C) {
	size := len(validateRevokeAccessTest)
	for i, test := range validateRevokeAccessTest {
		c.Logf("Running test %d of %d", i, size)
		obtained := test.spec.RevokeAccess()
		c.Check(obtained, gc.Equals, test.expected,
			gc.Commentf("revoke %q on %q, expect %q", test.spec.Access, test.spec.Target.ObjectType, test.expected))
	}
}
