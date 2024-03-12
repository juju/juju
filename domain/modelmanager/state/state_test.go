// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmanager/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestStateCreate(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.Create(context.Background(), modeltesting.GenModelUUID(c))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestStateCreateCalledTwice(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := modeltesting.GenModelUUID(c)

	err := st.Create(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Create(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

// Note: This will pass as we don't validate the UUID at this level, and we
// don't compile UUID module into sqlite3 either.
func (s *stateSuite) TestStateCreateWithInvalidUUID(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	err := st.Create(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}
