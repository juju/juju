// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/auth"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestAddUser(c *gc.C) {
	ctx := context.Background()
	uuid, addAdminUser := AddUser("admin")
	err := addAdminUser(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.Validate(), jc.ErrorIsNil)

	// Check that the user was created.
	var name string
	row := s.DB().QueryRow(`
SELECT name FROM user WHERE name = ?`, "admin")
	c.Assert(row.Scan(&name), jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "admin")
}

func (s *bootstrapSuite) TestAddUserWithPassword(c *gc.C) {
	ctx := context.Background()
	uuid, addAdminUser := AddUserWithPassword("admin", auth.NewPassword("password"))
	err := addAdminUser(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.Validate(), jc.ErrorIsNil)

	// Check that the user was created.
	var name string
	row := s.DB().QueryRow(`
SELECT name FROM user WHERE name = ?`, "admin")
	c.Assert(row.Scan(&name), jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "admin")
}
