// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	model "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type commitHookSuite struct {
	svc *LeadershipService

	st                *MockState
	leadershipEnsurer *MockEnsurer
	secretBackend     *MockSecretBackendReferenceMutator
	clock             *testclock.Clock
	uuidGen           func() (uuid.UUID, error)
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
	err := s.svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesNoLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: args which indicate leadership is not required
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()
	expectedRelationUUID := tc.Must(c, corerelation.NewUUID)
	s.st.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(expectedRelationUUID, nil)

	unitName := unittesting.GenNewName(c, "test/0")
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		RelationSettings: []unitstate.RelationSettings{{
			RelationKey: key,
			Settings:    map[string]string{"key": "value"},
		}},
	}
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{
		UnitUUID: unitUUID.String(),
	}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)

	expected := internal.CommitHookChangesArg{
		UnitUUID: unitUUID.String(),
		RelationSettings: []internal.RelationSettings{{
			RelationUUID: expectedRelationUUID,
			UnitSet:      map[string]string{"key": "value"},
		},
		},
	}
	s.st.EXPECT().CommitHookChanges(c.Context(), expected).Return(nil)

	// Act
	err := s.svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: args which indicate leadership is required
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()
	expectedRelationUUID := tc.Must(c, corerelation.NewUUID)
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
	unitInfo := internal.CommitHookUnitInfo{
		UnitUUID: unitUUID.String(),
	}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), arg.UnitName.String()).Return(unitInfo, nil)
	s.leadershipEnsurer.EXPECT().WithLeader(gomock.Any(), "test", "test/0", gomock.Any()).Return(nil)

	// Act
	err := s.svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesLeadershipGrantAppSecret(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.leadershipEnsurer.EXPECT().WithLeader(gomock.Any(), "test", "test/0", gomock.Any()).Return(nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretGrants: []unitstate.GrantSecretArg{{
			URI:         coresecrets.NewURI(),
			SubjectUUID: "subject-uuid",
			ScopeUUID:   "scope-uuid",
			OwnerKind:   secret.ApplicationCharmSecretOwner,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesNoLeadershipGrantUnitSecret(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).Return(nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretGrants: []unitstate.GrantSecretArg{{
			URI:         coresecrets.NewURI(),
			SubjectUUID: "subject-uuid",
			ScopeUUID:   "scope-uuid",
			OwnerKind:   secret.UnitCharmSecretOwner,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesLeadershipRevokeAppSecret(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.leadershipEnsurer.EXPECT().WithLeader(gomock.Any(), "test", "test/0", gomock.Any()).Return(nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretRevokes: []unitstate.RevokeSecretArg{{
			URI:         coresecrets.NewURI(),
			SubjectUUID: "subject-uuid",
			OwnerKind:   secret.ApplicationCharmSecretOwner,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesNoLeadershipRevokeUnitSecret(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).Return(nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretRevokes: []unitstate.RevokeSecretArg{{
			URI:         coresecrets.NewURI(),
			SubjectUUID: "subject-uuid",
			OwnerKind:   secret.UnitCharmSecretOwner,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesUpdateNetworkInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Unit
	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{
		UnitUUID: unitUUID.String(),
	}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)

	// Arrange: relations
	relation1UUID := tc.Must(c, corerelation.NewUUID)
	relation2UUID := tc.Must(c, corerelation.NewUUID)
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()
	s.st.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(relation1UUID, nil)

	// Arrange: CommitHookChanges state call
	expected := internal.CommitHookChangesArg{
		UnitUUID: unitUUID.String(),
		RelationSettings: []internal.RelationSettings{
			{
				RelationUUID: relation1UUID,
				UnitSet: map[string]string{
					"key":                       "value",
					unitstate.IngressAddressKey: "10.0.0.6",
					unitstate.EgressSubnetsKey:  "192.0.2.0/24, 192.51.100.0/24",
				},
			}, {
				RelationUUID: relation2UUID,
				UnitSet: map[string]string{
					unitstate.IngressAddressKey: "10.0.1.23",
					unitstate.EgressSubnetsKey:  "203.0.113.0/24",
				},
			},
		},
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.RelationSettings`, tc.SameContents, expected.RelationSettings)
	s.st.EXPECT().CommitHookChanges(c.Context(), tc.Bind(mc, expected)).Return(nil)

	// Arrange args for call
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		RelationSettings: []unitstate.RelationSettings{{
			RelationKey: key,
			Settings:    map[string]string{"key": "value"},
		}},
		UpdatedRelationNetworkInfo: map[corerelation.UUID]unitstate.Settings{
			relation1UUID: {
				unitstate.IngressAddressKey: "10.0.0.6",
				unitstate.EgressSubnetsKey:  "192.0.2.0/24, 192.51.100.0/24",
			},
			relation2UUID: {
				unitstate.IngressAddressKey: "10.0.1.23",
				unitstate.EgressSubnetsKey:  "203.0.113.0/24",
			},
		},
	}

	// Act
	err := s.svc.CommitHookChanges(c.Context(), arg)

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
	uuid, err := s.svc.getRelationUUIDByKey(c.Context(), key)

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
	uuid, err := s.svc.getRelationUUIDByKey(c.Context(), key)

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
	_, err := s.svc.getRelationUUIDByKey(
		c.Context(),
		corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2"),
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesUUIDGenerationFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: mock UUID generator to fail
	uuidErr := errors.New("uuid boom")
	s.svc.uuidGenerator = func() (uuid.UUID, error) {
		return uuid.UUID{}, uuidErr
	}

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	// Secret update with data triggers UUID generation
	uri := coresecrets.NewURI()

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				Data:     map[string]string{"key": "value"},
				Checksum: "checksum",
			},
		}},
	}

	// Act
	err := s.svc.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, `generating revision UUID for update\[0\]: uuid boom`)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesAlwaysAddsBackendRef(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()

	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	)

	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).Return(nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				Data:     map[string]string{"key": "value"},
				Checksum: "same-checksum",
			},
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesDifferentChecksumAddsBackendRef(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()

	rollbackCalled := false
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), model.UUID("model-uuid"), gomock.Any(), uri.ID,
	).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).Return(nil)

	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				Data:     map[string]string{"key": "value"},
				Checksum: "new-checksum",
			},
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rollbackCalled, tc.IsFalse)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesPartialRollback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	firstRolledBack := false
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(func() error {
		firstRolledBack = true
		return nil
	}, nil)
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, errors.New("backend boom"))

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{
			{
				URI: uri1,
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"a": "1"},
					Checksum: "checksum-1",
				},
			},
			{
				URI: uri2,
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"b": "2"},
					Checksum: "checksum-2",
				},
			},
		},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Check(err, tc.ErrorMatches, `.*adding backend reference for update\[1\].*`)
	c.Check(firstRolledBack, tc.IsTrue)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesRotatePolicyMoreFrequent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()

	// Mock the current policy as RotateDaily (less frequent than RotateHourly).
	s.st.EXPECT().GetSecretRotatePolicy(gomock.Any(), uri.ID).
		Return(coresecrets.RotateDaily, nil)

	var captured internal.CommitHookChangesArg
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, arg internal.CommitHookChangesArg) {
			captured = arg
		}).
		Return(nil)

	hourly := coresecrets.RotateHourly
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				RotatePolicy: &hourly,
			},
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(captured.SecretUpdates), tc.Equals, 1)
	update := captured.SecretUpdates[0]
	c.Assert(update.NextRotateTime, tc.Not(tc.IsNil))
	c.Assert(update.RotatePolicy, tc.Not(tc.IsNil))
	c.Check(*update.RotatePolicy, tc.Equals, secret.RotateHourly)
	// NextRotateTime should be roughly now + 1 hour.
	expected := s.clock.Now().Add(time.Hour)
	c.Check(update.NextRotateTime.UTC().Truncate(time.Second), tc.Equals, expected.UTC().Truncate(time.Second))
}

func (s *commitHookSuite) TestPrepareSecretUpdatesRotatePolicyToNever(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()

	// GetSecretRotatePolicy must NOT be called because RotateNever.WillRotate() returns false.
	var captured internal.CommitHookChangesArg
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, arg internal.CommitHookChangesArg) {
			captured = arg
		}).
		Return(nil)

	never := coresecrets.RotateNever
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				RotatePolicy: &never,
			},
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(captured.SecretUpdates), tc.Equals, 1)
	update := captured.SecretUpdates[0]
	c.Assert(update.NextRotateTime, tc.IsNil)
	c.Assert(update.RotatePolicy, tc.Not(tc.IsNil))
	c.Check(*update.RotatePolicy, tc.Equals, secret.RotateNever)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesRotatePolicyLessFrequent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()

	// Mock the current policy as RotateHourly (more frequent than RotateDaily).
	s.st.EXPECT().GetSecretRotatePolicy(gomock.Any(), uri.ID).
		Return(coresecrets.RotateHourly, nil)

	var captured internal.CommitHookChangesArg
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, arg internal.CommitHookChangesArg) {
			captured = arg
		}).
		Return(nil)

	daily := coresecrets.RotateDaily
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				RotatePolicy: &daily,
			},
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(captured.SecretUpdates), tc.Equals, 1)
	update := captured.SecretUpdates[0]
	// NextRotateTime should be nil because the new policy is less frequent.
	c.Assert(update.NextRotateTime, tc.IsNil)
	c.Assert(update.RotatePolicy, tc.Not(tc.IsNil))
	c.Check(*update.RotatePolicy, tc.Equals, secret.RotateDaily)
}

func (s *commitHookSuite) TestPrepareSecretUpdatesRotatePolicySecretNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()

	// Mock GetSecretRotatePolicy to return SecretNotFound, which occurs when
	// the secret is being created in this transaction or was concurrently deleted.
	s.st.EXPECT().GetSecretRotatePolicy(gomock.Any(), uri.ID).
		Return(coresecrets.RotateNever, secreterrors.SecretNotFound)

	var captured internal.CommitHookChangesArg
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, arg internal.CommitHookChangesArg) {
			captured = arg
		}).
		Return(nil)

	hourly := coresecrets.RotateHourly
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretUpdates: []unitstate.UpdateSecretArg{{
			URI: uri,
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				RotatePolicy: &hourly,
			},
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(captured.SecretUpdates), tc.Equals, 1)
	update := captured.SecretUpdates[0]
	// NextRotateTime should be computed because SecretNotFound is treated as RotateNever.
	// This implements "last update wins" semantics and handles the case where a secret
	// is being created in the same transaction or was concurrently deleted.
	c.Assert(update.NextRotateTime, tc.Not(tc.IsNil))
	c.Assert(update.RotatePolicy, tc.Not(tc.IsNil))
	c.Check(*update.RotatePolicy, tc.Equals, secret.RotateHourly)
	// NextRotateTime should be roughly now + 1 hour.
	expected := s.clock.Now().Add(time.Hour)
	c.Check(update.NextRotateTime.UTC().Truncate(time.Second), tc.Equals, expected.UTC().Truncate(time.Second))
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

func (s *commitHookSuite) TestCommitHookChangesCreateAppSecretRequiresLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{
		UnitUUID:        unitUUID.String(),
		ApplicationUUID: appUUID.String(),
	}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(func() error { return nil }, nil)
	s.leadershipEnsurer.EXPECT().WithLeader(gomock.Any(), "test", "test/0", gomock.Any()).Return(nil)

	uri := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretCreates: []unitstate.CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				Version: 1,
				CharmOwner: secret.CharmSecretOwner{
					Kind: secret.ApplicationCharmSecretOwner,
					ID:   "test",
				},
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"key": "val"},
					Checksum: "checksum",
				},
			},
			URI: uri,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesCreateUnitSecretNoLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{
		UnitUUID: unitUUID.String(),
	}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(func() error { return nil }, nil)
	s.st.EXPECT().CommitHookChanges(gomock.Any(), gomock.Any()).Return(nil)

	uri := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretCreates: []unitstate.CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				Version: 1,
				CharmOwner: secret.CharmSecretOwner{
					Kind: secret.UnitCharmSecretOwner,
					ID:   "test/0",
				},
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"key": "val"},
					Checksum: "checksum",
				},
			},
			URI: uri,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestPrepareSecretCreatesUUIDGenerationFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuidErr := errors.New("uuid boom")
	s.svc.uuidGenerator = func() (uuid.UUID, error) {
		return uuid.UUID{}, uuidErr
	}

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	uri := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretCreates: []unitstate.CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				Version: 1,
				CharmOwner: secret.CharmSecretOwner{
					Kind: secret.UnitCharmSecretOwner,
					ID:   "test/0",
				},
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"key": "val"},
					Checksum: "checksum",
				},
			},
			URI: uri,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorMatches, `generating revision UUID for create\[0\]: uuid boom`)
}

func (s *commitHookSuite) TestPrepareSecretCreatesBackendRefFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, errors.New("backend boom"))

	uri := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretCreates: []unitstate.CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				Version: 1,
				CharmOwner: secret.CharmSecretOwner{
					Kind: secret.UnitCharmSecretOwner,
					ID:   "test/0",
				},
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"key": "val"},
					Checksum: "checksum",
				},
			},
			URI: uri,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Check(err, tc.ErrorMatches, `.*adding backend reference for create\[0\].*`)
}

func (s *commitHookSuite) TestPrepareSecretCreatesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")

	arg := unitstate.CommitHookChangesArg{
		UnitName:      unitName,
		SecretCreates: nil,
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookChangesSecretCreateDeadUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{
		UnitUUID: unitUUID.String(),
		UnitLife: life.Dead,
	}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)

	uri := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretCreates: []unitstate.CreateSecretArg{{
			CreateCharmSecretParams: secret.CreateCharmSecretParams{
				Version: 1,
				CharmOwner: secret.CharmSecretOwner{
					Kind: secret.UnitCharmSecretOwner,
					ID:   "test/0",
				},
				UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
					Data:     map[string]string{"key": "val"},
					Checksum: "checksum",
				},
			},
			URI: uri,
		}},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorMatches, `.*unit "test/0" is dead.*`)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *commitHookSuite) TestPrepareSecretCreatesPartialRollback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "test/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitInfo := internal.CommitHookUnitInfo{UnitUUID: unitUUID.String()}
	s.st.EXPECT().GetCommitHookUnitInfo(gomock.Any(), unitName.String()).Return(unitInfo, nil)
	s.st.EXPECT().GetModelUUID(gomock.Any()).Return("model-uuid", nil)

	firstRolledBack := false
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(func() error {
		firstRolledBack = true
		return nil
	}, nil)
	s.secretBackend.EXPECT().AddSecretBackendReference(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, errors.New("backend boom"))

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	arg := unitstate.CommitHookChangesArg{
		UnitName: unitName,
		SecretCreates: []unitstate.CreateSecretArg{
			{
				CreateCharmSecretParams: secret.CreateCharmSecretParams{
					Version: 1,
					CharmOwner: secret.CharmSecretOwner{
						Kind: secret.UnitCharmSecretOwner,
						ID:   "test/0",
					},
					UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
						Data:     map[string]string{"a": "1"},
						Checksum: "checksum-1",
					},
				},
				URI: uri1,
			},
			{
				CreateCharmSecretParams: secret.CreateCharmSecretParams{
					Version: 1,
					CharmOwner: secret.CharmSecretOwner{
						Kind: secret.UnitCharmSecretOwner,
						ID:   "test/0",
					},
					UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
						Data:     map[string]string{"b": "2"},
						Checksum: "checksum-2",
					},
				},
				URI: uri2,
			},
		},
	}

	err := s.svc.CommitHookChanges(c.Context(), arg)
	c.Check(err, tc.ErrorMatches, `.*adding backend reference for create\[1\].*`)
	c.Check(firstRolledBack, tc.IsTrue)
}

func (s *commitHookSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.leadershipEnsurer = NewMockEnsurer(ctrl)
	s.secretBackend = NewMockSecretBackendReferenceMutator(ctrl)
	s.clock = testclock.NewClock(time.Now())
	s.uuidGen = uuid.NewUUID

	s.svc = NewLeadershipService(
		s.st,
		s.secretBackend,
		s.leadershipEnsurer,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.svc = nil
		s.st = nil
		s.leadershipEnsurer = nil
		s.secretBackend = nil
	})

	return ctrl
}
