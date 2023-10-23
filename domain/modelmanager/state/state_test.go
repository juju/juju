// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/testing"
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

func (s *stateSuite) TestStateListIsEmpty(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	models, err := st.List(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(models, gc.HasLen, 0)
}

func (s *stateSuite) TestStateList(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := modeltesting.GenModelUUID(c)

	err := st.Create(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	models, err := st.List(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(models, gc.DeepEquals, []model.UUID{uuid})
}

func (s *stateSuite) TestStateListMultipleItems(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	var uuids []model.UUID
	for i := 0; i < 10; i++ {
		uuid := modeltesting.GenModelUUID(c)

		err := st.Create(context.Background(), uuid)
		c.Assert(err, jc.ErrorIsNil)

		uuids = append(uuids, uuid)
	}

	models, err := st.List(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(models, gc.DeepEquals, uuids)
}

func (s *stateSuite) TestStateDeleteWithNoMatchingUUID(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.Delete(context.Background(), modeltesting.GenModelUUID(c))
	c.Assert(err, jc.ErrorIs, domain.ErrNoRecord)
}

func (s *stateSuite) TestStateDelete(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := modeltesting.GenModelUUID(c)

	err := st.Create(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestStateDeleteCalledTwice(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := modeltesting.GenModelUUID(c)

	err := st.Create(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, domain.ErrNoRecord)
}
