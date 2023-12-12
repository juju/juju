// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	userstate "github.com/juju/juju/domain/user/state"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertAdminUser(c *gc.C) {
	st := userstate.NewState(s.TxnRunnerFactory())
	err := InsertAdminUser()(context.Background(), s.TxnRunner())
	c.Assert(err, gc.IsNil)

	adminUser, err := st.GetUserByName(context.Background(), "admin")
	c.Assert(err, gc.IsNil)
	c.Assert(adminUser.Name, gc.Equals, "admin")
}
