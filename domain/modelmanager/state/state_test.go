// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelmanager/service"
	"github.com/juju/juju/domain/modelmanager/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestStateCreate(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.Create(context.TODO(), mustUUID(c))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestStateCreateCalledTwice(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := mustUUID(c)

	err := st.Create(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Create(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, `UNIQUE constraint failed: model_list.uuid`)
}

// Note: This will pass as we don't validate the UUID at this level, and we
// don't compile UUID module into sqlite3 either.
func (s *stateSuite) TestStateCreateWithInvalidUUID(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	err := st.Create(context.TODO(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestStateDeleteWithNoMatchingUUID(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())
	err := st.Delete(context.TODO(), mustUUID(c))
	c.Assert(err, gc.ErrorMatches, domain.ErrNoRecord.Error()+".*")
	c.Assert(errors.Is(errors.Cause(err), domain.ErrNoRecord), jc.IsTrue)
}

func (s *stateSuite) TestStateDelete(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := mustUUID(c)

	err := st.Create(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Delete(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestStateDeleteCalledTwice(c *gc.C) {
	st := state.NewState(s.TxnRunnerFactory())

	uuid := mustUUID(c)

	err := st.Create(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Delete(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Delete(context.TODO(), uuid)
	c.Assert(err, gc.ErrorMatches, domain.ErrNoRecord.Error()+".*")
}

func mustUUID(c *gc.C) service.UUID {
	return service.UUID(utils.MustNewUUID().String())
}
