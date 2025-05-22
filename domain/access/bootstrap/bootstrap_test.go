// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/auth"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite

	controllerUUID string
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)
}

func (s *bootstrapSuite) TestAddUserWithPassword(c *tc.C) {
	ctx := c.Context()
	uuid, addAdminUser := AddUserWithPassword(usertesting.GenNewName(c, "admin"), auth.NewPassword("password"), permission.AccessSpec{
		Access: permission.SuperuserAccess,
		Target: permission.ID{
			ObjectType: permission.Controller,
			Key:        s.controllerUUID,
		},
	})
	err := addAdminUser(ctx, s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid.Validate(), tc.ErrorIsNil)

	// Check that the user was created.
	var name string
	row := s.DB().QueryRow(`
SELECT name FROM user WHERE name = ?`, "admin")
	c.Assert(row.Scan(&name), tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "admin")
}
