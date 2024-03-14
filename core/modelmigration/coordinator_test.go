// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type migrationSuite struct {
	testing.IsolationSuite

	op        *MockOperation
	txnRunner *MockTxnRunner
	model     *MockModel

	scope Scope
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) TestAdd(c *gc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator()
	c.Assert(m.Len(), gc.Equals, 0)

	m.Add(s.op)
	c.Assert(m.Len(), gc.Equals, 1)
}

func (s *migrationSuite) TestPerform(c *gc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator()
	c.Assert(m.Len(), gc.Equals, 0)

	m.Add(s.op)

	// We do care about the order of the calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(nil),
		s.op.EXPECT().Execute(gomock.Any(), s.model).Return(nil),
	)

	err := m.Perform(context.Background(), s.scope, s.model)
	c.Assert(err, jc.ErrorIsNil)
}
func (s *migrationSuite) TestPerformWithRollbackAtSetup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator()
	c.Assert(m.Len(), gc.Equals, 0)

	m.Add(s.op)

	// We do care about the order of the calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(errors.New("boom")),
		s.op.EXPECT().Rollback(gomock.Any(), s.model).Return(nil),
	)

	err := m.Perform(context.Background(), s.scope, s.model)
	c.Assert(err, gc.ErrorMatches, `setup operation at 0: boom`)
}

func (s *migrationSuite) TestPerformWithRollbackAtExecution(c *gc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator()
	c.Assert(m.Len(), gc.Equals, 0)

	m.Add(s.op)

	// We do care about the order of the calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(nil),
		s.op.EXPECT().Execute(gomock.Any(), s.model).Return(errors.New("boom")),
		s.op.EXPECT().Rollback(gomock.Any(), s.model).Return(nil),
	)

	err := m.Perform(context.Background(), s.scope, s.model)
	c.Assert(err, gc.ErrorMatches, `execute operation at 0: boom`)
}

func (s *migrationSuite) TestPerformWithRollbackError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator()
	c.Assert(m.Len(), gc.Equals, 0)

	m.Add(s.op)

	// We do care about the order of the calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(nil),
		s.op.EXPECT().Execute(gomock.Any(), s.model).Return(errors.New("boom")),
		s.op.EXPECT().Rollback(gomock.Any(), s.model).Return(errors.New("sad")),
	)

	err := m.Perform(context.Background(), s.scope, s.model)
	c.Assert(err, gc.ErrorMatches, `rollback operation at 0 with sad: execute operation at 0: boom`)
}

func (s *migrationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.op = NewMockOperation(ctrl)
	s.txnRunner = NewMockTxnRunner(ctrl)
	s.model = NewMockModel(ctrl)

	s.scope = NewScope(nil, nil)

	return ctrl
}
