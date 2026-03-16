// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corerelation "github.com/juju/juju/core/relation"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
	"github.com/juju/juju/domain/unitstate/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	st *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSetState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0").String()

	as := unitstate.UnitState{
		Name:          name,
		CharmState:    new(map[string]string{"one-key": "one-value"}),
		UniterState:   new("some-uniter-state-yaml"),
		RelationState: new(map[int]string{1: "one-value"}),
		StorageState:  new("some-storage-state-yaml"),
		SecretState:   new("some-secret-state-yaml"),
	}

	exp := s.st.EXPECT()
	exp.SetUnitState(gomock.Any(), as)

	err := NewService(s.st, loggertesting.WrapCheckLog(c)).SetState(c.Context(), as)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0").String()

	as := unitstate.UnitState{
		Name:        name,
		UniterState: new("some-uniter-state-yaml"),
	}

	exp := s.st.EXPECT()
	exp.SetUnitState(gomock.Any(), as).Return(errors.UnitNotFound)

	err := NewService(s.st, loggertesting.WrapCheckLog(c)).SetState(c.Context(), as)
	c.Check(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name.String())

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).GetState(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name.String()).Return(unitstate.RetrievedUnitState{}, unitstateerrors.UnitNotFound)

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).GetState(c.Context(), name)
	c.Assert(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	c.Cleanup(func() { s.st = nil })

	return ctrl
}

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
	s.st.EXPECT().CommitHookChanges(c.Context(), internal.TransformCommitHookChangesArg(arg)).Return(nil)

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
