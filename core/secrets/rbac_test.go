// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/secrets"
)

type RoleSuite struct{}

var _ = tc.Suite(&RoleSuite{})

func (s *SecretValueSuite) TestAllowed(c *tc.C) {
	c.Assert(secrets.RoleNone.Allowed(secrets.RoleView), tc.IsFalse)
	c.Assert(secrets.RoleNone.Allowed(secrets.RoleRotate), tc.IsFalse)
	c.Assert(secrets.RoleNone.Allowed(secrets.RoleManage), tc.IsFalse)
	c.Assert(secrets.RoleView.Allowed(secrets.RoleView), tc.IsTrue)
	c.Assert(secrets.RoleView.Allowed(secrets.RoleRotate), tc.IsFalse)
	c.Assert(secrets.RoleView.Allowed(secrets.RoleManage), tc.IsFalse)
	c.Assert(secrets.RoleRotate.Allowed(secrets.RoleView), tc.IsTrue)
	c.Assert(secrets.RoleRotate.Allowed(secrets.RoleRotate), tc.IsTrue)
	c.Assert(secrets.RoleRotate.Allowed(secrets.RoleManage), tc.IsFalse)
	c.Assert(secrets.RoleManage.Allowed(secrets.RoleView), tc.IsTrue)
	c.Assert(secrets.RoleManage.Allowed(secrets.RoleRotate), tc.IsTrue)
	c.Assert(secrets.RoleManage.Allowed(secrets.RoleManage), tc.IsTrue)
}
