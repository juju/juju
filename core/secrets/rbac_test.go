// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/secrets"
)

type RoleSuite struct{}

var _ = tc.Suite(&RoleSuite{})

func (s *SecretValueSuite) TestAllowed(c *tc.C) {
	c.Assert(secrets.RoleNone.Allowed(secrets.RoleView), jc.IsFalse)
	c.Assert(secrets.RoleNone.Allowed(secrets.RoleRotate), jc.IsFalse)
	c.Assert(secrets.RoleNone.Allowed(secrets.RoleManage), jc.IsFalse)
	c.Assert(secrets.RoleView.Allowed(secrets.RoleView), jc.IsTrue)
	c.Assert(secrets.RoleView.Allowed(secrets.RoleRotate), jc.IsFalse)
	c.Assert(secrets.RoleView.Allowed(secrets.RoleManage), jc.IsFalse)
	c.Assert(secrets.RoleRotate.Allowed(secrets.RoleView), jc.IsTrue)
	c.Assert(secrets.RoleRotate.Allowed(secrets.RoleRotate), jc.IsTrue)
	c.Assert(secrets.RoleRotate.Allowed(secrets.RoleManage), jc.IsFalse)
	c.Assert(secrets.RoleManage.Allowed(secrets.RoleView), jc.IsTrue)
	c.Assert(secrets.RoleManage.Allowed(secrets.RoleRotate), jc.IsTrue)
	c.Assert(secrets.RoleManage.Allowed(secrets.RoleManage), jc.IsTrue)
}
