// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type commitHookSuite struct {
	st                *MockState
	leadershipEnsurer *MockEnsurer
}

func TestCommitHookSuite(t *testing.T) {
	tc.Run(t, &commitHookSuite{})
}

func (s *commitHookSuite) TestCommitHookChangesNoChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: args which no changes are needed
	arg := unitstate.CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "test/0"),
	}

	// Act
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	err := svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesNoLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: args which indicate leadership is not required
	arg := unitstate.CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "test/0"),
		RelationSettings: []unitstate.RelationSettings{{
			RelationUUID: tc.Must(c, corerelation.NewUUID),
			Settings:     map[string]string{"key": "value"},
		}},
	}
	unitUUID := tc.Must(c, coreunit.NewUUID)
	s.st.EXPECT().GetUnitUUIDByName(c.Context(), arg.UnitName).Return(unitUUID, nil)
	s.st.EXPECT().CommitHookChanges(c.Context(), internal.TransformCommitHookChangesArg(arg, unitUUID)).Return(nil)

	// Act
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	err := svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: args which indicate leadership is required
	arg := unitstate.CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "test/0"),
		RelationSettings: []unitstate.RelationSettings{{
			RelationUUID:        tc.Must(c, corerelation.NewUUID),
			ApplicationSettings: map[string]string{"key": "value"},
		}},
	}
	unitUUID := tc.Must(c, coreunit.NewUUID)
	s.st.EXPECT().GetUnitUUIDByName(c.Context(), arg.UnitName).Return(unitUUID, nil)
	s.leadershipEnsurer.EXPECT().WithLeader(c.Context(), "test", "test/0", gomock.Any()).Return(nil)

	// Act
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	err := svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.leadershipEnsurer = NewMockEnsurer(ctrl)

	c.Cleanup(func() {
		s.st = nil
		s.leadershipEnsurer = nil
	})

	return ctrl
}
