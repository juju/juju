// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	relationerrors "github.com/juju/juju/domain/relation/errors"
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
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)
	s.st.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(expectedRelationUUID, nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "test/0"),
		RelationSettings: []unitstate.RelationSettings{{
			RelationKey: key,
			Settings:    map[string]string{"key": "value"},
		}},
	}
	unitUUID := tc.Must(c, coreunit.NewUUID)
	s.st.EXPECT().GetUnitUUIDByName(c.Context(), arg.UnitName).Return(unitUUID, nil)

	expected := internal.CommitHookChangesArg{
		UnitUUID: unitUUID,
		RelationSettings: []internal.RelationSettings{{
			RelationUUID: expectedRelationUUID,
			UnitSet:      map[string]string{"key": "value"},
		},
		},
	}
	s.st.EXPECT().CommitHookChanges(c.Context(), expected).Return(nil)

	// Act
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	err := svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: args which indicate leadership is required
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)
	s.st.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(expectedRelationUUID, nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unittesting.GenNewName(c, "test/0"),
		RelationSettings: []unitstate.RelationSettings{{
			RelationKey:         key,
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

func (s *commitHookSuite) TestGetRelationUUIDByKeyPeer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1")

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.st.EXPECT().GetPeerRelationUUIDByEndpointIdentifiers(
		gomock.Any(), key.EndpointIdentifiers()[0],
	).Return(expectedRelationUUID, nil)

	// Act:
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	uuid, err := svc.getRelationUUIDByKey(c.Context(), key)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, expectedRelationUUID)
}

func (s *commitHookSuite) TestGetRelationUUIDByKeyRegular(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)
	s.st.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(expectedRelationUUID, nil)

	// Act:
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	uuid, err := svc.getRelationUUIDByKey(c.Context(), key)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, expectedRelationUUID)
}

func (s *commitHookSuite) TestGetRelationUUIDByKeyRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.st.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return("", relationerrors.RelationNotFound)

	// Act:
	svc := NewLeadershipService(s.st, s.leadershipEnsurer, loggertesting.WrapCheckLog(c))
	_, err := svc.getRelationUUIDByKey(
		c.Context(),
		corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2"),
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *commitHookSuite) TestParseForSetAndUnsetSettings(c *tc.C) {
	// Arrange
	input := unitstate.Settings{
		"key1": "value1",
		"key2": "value2",
		"key3": "",
	}

	// Act
	obtainedSettings, obtainedKeys := parseForSetAndUnsetSettings(input)

	// Assert
	c.Check(obtainedSettings, tc.DeepEquals, unitstate.Settings{"key1": "value1", "key2": "value2"})
	c.Check(obtainedKeys, tc.DeepEquals, []string{"key3"})
}

func (s *commitHookSuite) TestParseForSetAndUnsetSettingsNilKeys(c *tc.C) {
	// Arrange
	input := unitstate.Settings{
		"key1": "value1",
	}

	// Act
	obtainedSettings, obtainedKeys := parseForSetAndUnsetSettings(input)

	// Assert
	c.Check(obtainedSettings, tc.DeepEquals, unitstate.Settings{"key1": "value1"})
	c.Check(obtainedKeys, tc.DeepEquals, []string(nil))
}

func (s *commitHookSuite) TestParseForSetAndUnsetSettingsNilSettings(c *tc.C) {
	// Arrange
	input := unitstate.Settings{
		"key1": "",
	}

	// Act
	obtainedSettings, obtainedKeys := parseForSetAndUnsetSettings(input)

	// Assert
	c.Check(obtainedSettings, tc.DeepEquals, unitstate.Settings(nil))
	c.Check(obtainedKeys, tc.DeepEquals, []string{"key1"})
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
