// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/secrets"
)

type RoleSuite struct{}

func TestRoleSuite(t *testing.T) {
	tc.Run(t, &RoleSuite{})
}

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
