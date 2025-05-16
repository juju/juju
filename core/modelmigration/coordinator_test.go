// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type migrationSuite struct {
	testhelpers.IsolationSuite

	op        *MockOperation
	txnRunner *MockTxnRunner
	model     *MockModel

	scope Scope
}

func TestMigrationSuite(t *stdtesting.T) { tc.Run(t, &migrationSuite{}) }
func (s *migrationSuite) TestAdd(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator(loggertesting.WrapCheckLog(c))
	c.Assert(m.Len(), tc.Equals, 0)

	m.Add(s.op)
	c.Assert(m.Len(), tc.Equals, 1)
}

func (s *migrationSuite) TestPerform(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator(loggertesting.WrapCheckLog(c))
	c.Assert(m.Len(), tc.Equals, 0)

	m.Add(s.op)

	// We do care about the order of the calls.
	gomock.InOrder(
		s.op.EXPECT().Name().Return("op"),
		s.op.EXPECT().Setup(s.scope).Return(nil),
		s.op.EXPECT().Execute(gomock.Any(), s.model).Return(nil),
	)

	err := m.Perform(c.Context(), s.scope, s.model)
	c.Assert(err, tc.ErrorIsNil)
}
func (s *migrationSuite) TestPerformWithRollbackAtSetup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator(loggertesting.WrapCheckLog(c))
	c.Assert(m.Len(), tc.Equals, 0)

	m.Add(s.op)

	s.op.EXPECT().Name().Return("op").MinTimes(1)

	// We do care about the order of these calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(errors.New("boom")),
		s.op.EXPECT().Rollback(gomock.Any(), s.model).Return(nil),
	)

	err := m.Perform(c.Context(), s.scope, s.model)
	c.Assert(err, tc.ErrorMatches, `setup operation op: boom`)
}

func (s *migrationSuite) TestPerformWithRollbackAtExecution(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator(loggertesting.WrapCheckLog(c))
	c.Assert(m.Len(), tc.Equals, 0)

	m.Add(s.op)

	s.op.EXPECT().Name().Return("op").MinTimes(1)

	// We do care about the order of these calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(nil),
		s.op.EXPECT().Execute(gomock.Any(), s.model).Return(errors.New("boom")),
		s.op.EXPECT().Rollback(gomock.Any(), s.model).Return(nil),
	)

	err := m.Perform(c.Context(), s.scope, s.model)
	c.Assert(err, tc.ErrorMatches, `execute operation op: boom`)
}

func (s *migrationSuite) TestPerformWithRollbackError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := NewCoordinator(loggertesting.WrapCheckLog(c))
	c.Assert(m.Len(), tc.Equals, 0)

	m.Add(s.op)

	s.op.EXPECT().Name().Return("op").MinTimes(1)

	// We do care about the order of these calls.
	gomock.InOrder(
		s.op.EXPECT().Setup(s.scope).Return(nil),
		s.op.EXPECT().Execute(gomock.Any(), s.model).Return(errors.New("boom")),
		s.op.EXPECT().Rollback(gomock.Any(), s.model).Return(errors.New("sad")),
	)

	err := m.Perform(c.Context(), s.scope, s.model)
	c.Assert(err, tc.ErrorMatches, `rollback operation at 0 with sad: execute operation op: boom`)
}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.op = NewMockOperation(ctrl)
	s.txnRunner = NewMockTxnRunner(ctrl)
	s.model = NewMockModel(ctrl)

	s.scope = NewScope(nil, nil, nil)

	return ctrl
}
