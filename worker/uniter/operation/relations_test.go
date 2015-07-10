// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/operation"
)

type UpdateRelationsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpdateRelationsSuite{})

func (s *UpdateRelationsSuite) TestPrepare(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewUpdateRelations(nil)
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Prepare(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)
}

func (s *UpdateRelationsSuite) TestExecuteError(c *gc.C) {
	callbacks := &UpdateRelationsCallbacks{
		MockUpdateRelations: &MockUpdateRelations{err: errors.New("quack")},
	}
	factory := operation.NewFactory(operation.FactoryParams{Callbacks: callbacks})
	op, err := factory.NewUpdateRelations([]int{3, 2, 1})
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Prepare(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)

	state, err = op.Execute(operation.State{})
	c.Check(err, gc.ErrorMatches, "quack")
	c.Check(state, gc.IsNil)
	c.Check(callbacks.MockUpdateRelations.gotIds, jc.DeepEquals, &[]int{3, 2, 1})
}

func (s *UpdateRelationsSuite) TestExecuteSuccess(c *gc.C) {
	callbacks := &UpdateRelationsCallbacks{
		MockUpdateRelations: &MockUpdateRelations{},
	}
	factory := operation.NewFactory(operation.FactoryParams{Callbacks: callbacks})
	op, err := factory.NewUpdateRelations([]int{3, 2, 1})
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Prepare(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)

	state, err = op.Execute(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)
	c.Check(callbacks.MockUpdateRelations.gotIds, jc.DeepEquals, &[]int{3, 2, 1})
}

func (s *UpdateRelationsSuite) TestCommit(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewUpdateRelations(nil)
	c.Assert(err, jc.ErrorIsNil)
	state, err := op.Commit(operation.State{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(state, gc.IsNil)
}

func (s *UpdateRelationsSuite) TestDoesNotNeedGlobalMachineLock(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewUpdateRelations(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsFalse)
}
