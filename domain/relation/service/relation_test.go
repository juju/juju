// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	domainapplication "github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
)

type relationServiceSuite struct {
	baseServiceSuite
}

func TestRelationServiceSuite(t *testing.T) {
	tc.Run(t, &relationServiceSuite{})
}

// TestAddRelation verifies the behavior of the AddRelation method when adding
// a relation between two endpoints.
func (s *relationServiceSuite) TestAddRelation(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1"
	endpoint2 := "application-2:endpoint-2"

	fakeReturn1 := relation.Endpoint{
		ApplicationName: "application-1",
	}
	fakeReturn2 := relation.Endpoint{
		ApplicationName: "application-2",
	}

	s.state.EXPECT().AddRelation(gomock.Any(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "endpoint-2",
	}).Return(fakeReturn1, fakeReturn2, nil)

	// Act
	gotEp1, gotEp2, err := s.service.AddRelation(c.Context(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotEp1, tc.Equals, fakeReturn1)
	c.Check(gotEp2, tc.Equals, fakeReturn2)
}

// TestAddRelationFirstMalformed verifies that AddRelation returns an
// appropriate error when the first endpoint is malformed.
func (s *relationServiceSuite) TestAddRelationFirstMalformed(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "app:ep:is:malformed"
	endpoint2 := "application-2:endpoint-2"

	// Act
	_, _, err := s.service.AddRelation(c.Context(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, tc.ErrorMatches, "parsing endpoint identifier \"app:ep:is:malformed\": expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
}

// TestAddRelationFirstMalformed verifies that AddRelation returns an
// appropriate error when the second endpoint is malformed.
func (s *relationServiceSuite) TestAddRelationSecondMalformed(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1:endpoint-1"
	endpoint2 := "app:ep:is:malformed"

	// Act
	_, _, err := s.service.AddRelation(c.Context(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, tc.ErrorMatches, "parsing endpoint identifier \"app:ep:is:malformed\": expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
}

// TestAddRelationStateError validates the AddRelation method handles and
// returns the correct error when state addition fails.
func (s *relationServiceSuite) TestAddRelationStateError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	var empty relation.Endpoint

	s.state.EXPECT().AddRelation(gomock.Any(), gomock.Any(), gomock.Any()).Return(empty, empty, expectedError)

	// Act
	_, _, err := s.service.AddRelation(c.Context(), "app1", "app2")

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

// TestGetAllRelationDetails verifies that GetAllRelationDetails
// retrieves and returns the expected relation details without errors.
// Doesn't have logic, so the test doesn't need to be smart.
func (s *relationServiceSuite) TestGetAllRelationDetails(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedRelationDetails := []relation.RelationDetailsResult{
		{
			Life: "alive",
			UUID: "placedholder",
			ID:   42,
		},
	}
	s.state.EXPECT().GetAllRelationDetails(gomock.Any()).Return(expectedRelationDetails, nil)

	// Act
	details, err := s.service.GetAllRelationDetails(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(details, tc.DeepEquals, expectedRelationDetails)
}

// TestGetAllRelationDetailsError verifies the behavior when GetAllRelationDetails
// encounters an error from the state layer.
func (s *relationServiceSuite) TestGetAllRelationDetailsError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	s.state.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.service.GetAllRelationDetails(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *relationServiceSuite) TestGetRelationUUIDByID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)
	relationID := 1

	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(expectedRelationUUID, nil)

	// Act.
	relationUUID, err := s.service.GetRelationUUIDByID(c.Context(), relationID)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relationUUID, tc.Equals, expectedRelationUUID)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyPeer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1")

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetPeerRelationUUIDByEndpointIdentifiers(
		gomock.Any(), key.EndpointIdentifiers()[0],
	).Return(expectedRelationUUID, nil)

	// Act:
	uuid, err := s.service.GetRelationUUIDByKey(c.Context(), key)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, expectedRelationUUID)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRegular(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(expectedRelationUUID, nil)

	// Act:
	uuid, err := s.service.GetRelationUUIDByKey(c.Context(), key)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, expectedRelationUUID)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return("", relationerrors.RelationNotFound)

	// Act:
	_, err := s.service.GetRelationUUIDByKey(
		c.Context(),
		corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2"),
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRelationKeyNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act:
	_, err := s.service.GetRelationUUIDByKey(c.Context(), corerelation.Key{})

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationKeyNotValid)
}

func (s *relationServiceSuite) TestGetRelationsStatusForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitUUID := coreunittesting.GenUnitUUID(c)

	endpoint1 := relation.Endpoint{
		ApplicationName: "app-1",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-1",
			Role: internalcharm.RoleProvider,
		},
	}
	endpoint2 := relation.Endpoint{
		ApplicationName: "app-2",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-2",
			Role: internalcharm.RoleRequirer,
		},
	}
	endpoint3 := relation.Endpoint{
		ApplicationName: "app-2",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-3",
			Role: internalcharm.RolePeer,
		},
	}

	// The state layer is responsible for returning endpoints in canonical
	// key order (requirer, provider), so the mock reflects that contract.
	results := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint2, endpoint1},
		InScope:   true,
		Suspended: true,
	}, {
		Endpoints: []relation.Endpoint{endpoint3},
		InScope:   false,
		Suspended: false,
	}}

	expectedStatuses := []relation.RelationUnitStatus{{
		Key:       corerelationtesting.GenNewKey(c, "app-2:fake-endpoint-name-2 app-1:fake-endpoint-name-1"),
		InScope:   results[0].InScope,
		Suspended: results[0].Suspended,
	}, {
		Key:       corerelationtesting.GenNewKey(c, "app-2:fake-endpoint-name-3"),
		InScope:   results[1].InScope,
		Suspended: results[1].Suspended,
	}}

	s.state.EXPECT().GetRelationsStatusForUnit(gomock.Any(), unitUUID).Return(results, nil)

	// Act.
	statuses, err := s.service.GetRelationsStatusForUnit(c.Context(), unitUUID)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statuses, tc.DeepEquals, expectedStatuses)
}

func (s *relationServiceSuite) TestGetRelationsStatusForUnitUnitUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.GetRelationsStatusForUnit(c.Context(), "bad-unit-uuid")

	// Assert.
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationsStatusForUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitUUID := coreunittesting.GenUnitUUID(c)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationsStatusForUnit(gomock.Any(), unitUUID).Return(nil, boom)

	// Act.
	_, err := s.service.GetRelationsStatusForUnit(c.Context(), unitUUID)

	// Assert.
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(unitUUID, nil)

	// Act
	uuid, err := s.service.GetRelationUnitUUID(c.Context(), relationUUID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, unitUUID)
}

func (s *relationServiceSuite) TestGetRelationUnitRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelation.UUID("not-valid-uuid")
	unitName := coreunittesting.GenNewName(c, "app1/0")

	// Act
	_, err := s.service.GetRelationUnitUUID(c.Context(), relationUUID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitUUIDUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunit.Name("not-valid-name")

	// Act
	_, err := s.service.GetRelationUnitUUID(c.Context(), relationUUID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestGetRelationUnitUUIDUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return("", boom)

	// Act
	_, err := s.service.GetRelationUnitUUID(c.Context(), relationUUID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitByID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationID := 42
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(relationUUID, nil)
	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(unitUUID, nil)

	// Act
	uuid, err := s.service.getRelationUnitByID(c.Context(), relationID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, unitUUID)
}

func (s *relationServiceSuite) TestGetRelationUnitByIDRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationID := 42
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return("", relationerrors.RelationNotFound)

	// Act
	_, err := s.service.getRelationUnitByID(c.Context(), relationID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUnitByIDUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	unitName := coreunit.Name("not-valid-name")

	// Act
	_, err := s.service.getRelationUnitByID(c.Context(), 42, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestGetRelationUnitByIDUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationID := 42
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(relationUUID, nil)
	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return("", boom)

	// Act
	_, err := s.service.getRelationUnitByID(c.Context(), relationID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	appUUIDs := []coreapplication.UUID{
		tc.Must(c, coreapplication.NewUUID),
		tc.Must(c, coreapplication.NewUUID),
	}
	unitUUIDS := []coreunit.UUID{
		coreunittesting.GenUnitUUID(c),
		coreunittesting.GenUnitUUID(c),
		coreunittesting.GenUnitUUID(c),
	}
	expectedResult := relation.RelationUnitsChange{
		Changed: map[coreunit.Name]int64{
			"foo/1": 42,
			"foo/2": 43,
		},
		AppChanged: map[string]int64{
			"foo": 42,
			"bar": 43,
		},
		Departed: []coreunit.Name{"bar/0"},
	}
	s.state.EXPECT().GetRelationUnitChanges(gomock.Any(), relationUUID.String(), unitUUIDS, appUUIDs).Return(expectedResult, nil)

	// Act
	result, err := s.service.GetRelationUnitChanges(c.Context(), relationUUID, unitUUIDS, appUUIDs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expectedResult)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesUnitUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitUUIDS := []coreunit.UUID{
		coreunittesting.GenUnitUUID(c),
		coreunit.UUID("not-valid-uuid"),
		coreunittesting.GenUnitUUID(c),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), relationUUID, unitUUIDS, nil)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesAppUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	appUUIDs := []coreapplication.UUID{
		tc.Must(c, coreapplication.NewUUID),
		coreapplication.UUID("not-valid-uuid"),
		tc.Must(c, coreapplication.NewUUID),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), relationUUID, nil, appUUIDs)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnitChanges(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(relation.RelationUnitsChange{}, boom)

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), relationUUID, nil, nil)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationDetails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	endpoint1 := relation.Endpoint{
		ApplicationName: "app-1",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-1",
			Role: internalcharm.RolePeer,
		},
	}

	relationDetailsResult := relation.RelationDetailsResult{
		Life:      corelife.Alive,
		UUID:      relationUUID,
		ID:        7,
		Endpoints: []relation.Endpoint{endpoint1},
		Suspended: true,
	}

	s.state.EXPECT().GetRelationDetails(gomock.Any(), relationUUID).Return(relationDetailsResult, nil)

	expectedRelationDetails := relation.RelationDetails{
		Life:      relationDetailsResult.Life,
		UUID:      relationDetailsResult.UUID,
		ID:        relationDetailsResult.ID,
		Key:       corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1"),
		Endpoints: relationDetailsResult.Endpoints,
		Suspended: true,
	}

	// Act:
	relationDetails, err := s.service.GetRelationDetails(c.Context(), relationUUID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relationDetails, tc.DeepEquals, expectedRelationDetails)
}

// TestGetRelationEndpointUUIDRelationUUIDNotValid tests the failure scenario
// where the provided RelationUUID is not valid.
func (s *relationServiceSuite) TestGetRelationDetailsRelationUUIDNotValid(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	_, err := s.service.GetRelationDetails(c.Context(), "bad-relation-uuid")

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid, tc.Commentf("(Assert) unexpected error: %v", err))
}

func (s *relationServiceSuite) TestGetRelationLifeSuspendedStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	result := internal.RelationLifeSuspendedStatus{
		Life:            corelife.Alive,
		Suspended:       true,
		SuspendedReason: "it's a test",
		Endpoints: []relation.Endpoint{
			{
				ApplicationName: "app-1",
				Relation: internalcharm.Relation{
					Name: "fake-endpoint-name-1",
					Role: internalcharm.RoleRequirer,
				},
			}, {
				ApplicationName: "app-2",
				Relation: internalcharm.Relation{
					Name: "fake-endpoint-name-2",
					Role: internalcharm.RoleProvider,
				},
			},
		},
	}

	s.state.EXPECT().GetRelationLifeSuspendedStatus(gomock.Any(), relationUUID.String()).Return(result, nil)

	expectedKey := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")

	// Act:
	obtained, err := s.service.GetRelationLifeSuspendedStatus(c.Context(), relationUUID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, relation.RelationLifeSuspendedStatus{
		Key:             expectedKey.String(),
		Life:            result.Life,
		Suspended:       result.Suspended,
		SuspendedReason: result.SuspendedReason,
	})
}

// TestGetRelationEndpointUUIDRelationUUIDNotValid tests the failure scenario
// where the provided RelationUUID is not valid.
func (s *relationServiceSuite) TestGetRelationLifeSuspendedStatusNotValid(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	_, err := s.service.GetRelationLifeSuspendedStatus(c.Context(), "bad-relation-uuid")

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

// TestEnterScope tests EnterScope with no subordinate unit creation
// expected.
func (s *relationServiceSuite) TestEnterScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	expectedSettings := map[string]string{"ingress": "x.x.x.x"}
	data := internal.SubordinateUnitStatusHistoryData{}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(internal.SubordinateUnitCreationInfo{}, false, nil)
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, expectedSettings,
		internal.SubordinateUnitStorageArgs{}).Return(data, nil)

	settings := map[string]string{"ingress": "x.x.x.x", "empty": ""}

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
	)
	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnterScopeNthTime tests the idempotency of EnterScope. If it's
// be called before successfully, do not attempt to create a subordinate
// unit.
func (s *relationServiceSuite) TestEnterScopeNthTime(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	data := internal.SubordinateUnitStatusHistoryData{}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(internal.SubordinateUnitCreationInfo{}, false, nil)
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings,
		internal.SubordinateUnitStorageArgs{}).Return(data, relationerrors.RelationUnitAlreadyExists)

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
	)
	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnterScopeNthTimeStorageArgsDiscarded tests that pre-computed
// storage arguments are discarded when the unit entering scope is already
// in the relation: the state reports RelationUnitAlreadyExists, no
// subordinate unit is created, and the storage arguments are not used to
// provision anything.
func (s *relationServiceSuite) TestEnterScopeNthTimeStorageArgsDiscarded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	data := internal.SubordinateUnitStatusHistoryData{}
	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	// The subordinate application has no storage directives, so the
	// storage arguments made for it are zero-value.
	creationInfo := internal.SubordinateUnitCreationInfo{
		SubordinateApplicationUUID: appUUID,
		MachineNetNodeUUID:         machineNetNodeUUID,
	}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(creationInfo, true, nil)
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings,
		internal.SubordinateUnitStorageArgs{}).Return(data, relationerrors.RelationUnitAlreadyExists)

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
	)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnterScopeCreatingSubordinateNoStorage tests that a subordinate unit
// is created even when the subordinate application has no storage
// directives: zero-value storage arguments are supplied to the state,
// proving that subordinate creation is not blocked for charms without
// storage.
func (s *relationServiceSuite) TestEnterScopeCreatingSubordinateNoStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	data := internal.SubordinateUnitStatusHistoryData{
		UnitName: unitName.String(),
		UnitStatus: domainapplication.UnitStatusArg{
			AgentStatus: &domainstatus.StatusInfo[domainstatus.UnitAgentStatusType]{
				Status: domainstatus.UnitAgentStatusAllocating,
			},
			WorkloadStatus: &domainstatus.StatusInfo[domainstatus.WorkloadStatusType]{
				Status:  domainstatus.WorkloadStatusActive,
				Message: "message",
			},
		},
	}
	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	creationInfo := internal.SubordinateUnitCreationInfo{
		SubordinateApplicationUUID: appUUID,
		MachineNetNodeUUID:         machineNetNodeUUID,
	}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(creationInfo, true, nil)
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings,
		internal.SubordinateUnitStorageArgs{}).Return(data, nil)
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.UnitAgentNamespace.WithID(unitName.String()),
		status.StatusInfo{
			Status: status.Allocating,
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.UnitWorkloadNamespace.WithID(unitName.String()),
		status.StatusInfo{
			Status:  status.Active,
			Message: "message",
		})

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
	)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestEnterScopeCreatingSubordinate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	data := internal.SubordinateUnitStatusHistoryData{
		UnitName: unitName.String(),
		UnitStatus: domainapplication.UnitStatusArg{
			AgentStatus: &domainstatus.StatusInfo[domainstatus.UnitAgentStatusType]{
				Status: domainstatus.UnitAgentStatusAllocating,
			},
			WorkloadStatus: &domainstatus.StatusInfo[domainstatus.WorkloadStatusType]{
				Status:  domainstatus.WorkloadStatusActive,
				Message: "message",
			},
		},
	}
	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	creationInfo := internal.SubordinateUnitCreationInfo{
		SubordinateApplicationUUID: appUUID,
		MachineNetNodeUUID:         machineNetNodeUUID,
		StorageDirectives: []domainstorageprovisioning.StorageDirective{
			{
				CharmMetadataName: "sub-charm",
				CharmStorageType:  charm.StorageFilesystem,
				Count:             1,
				MaxCount:          charm.StorageNoMaxCount,
				Name:              "data",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
		},
	}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(creationInfo, true, nil)

	provider := NewMockStorageProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.storagePoolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	// The storage arguments are made by the service from the storage
	// directives of the subordinate application. The generated UUIDs are
	// not deterministic, so the arguments are captured and asserted
	// structurally.
	var subordinateStorageArgs internal.SubordinateUnitStorageArgs
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings, gomock.Any()).
		DoAndReturn(func(
			_ context.Context,
			_ corerelation.UUID,
			_ coreunit.Name,
			_ map[string]string,
			storageArgs internal.SubordinateUnitStorageArgs,
		) (internal.SubordinateUnitStatusHistoryData, error) {
			subordinateStorageArgs = storageArgs
			return data, nil
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.UnitAgentNamespace.WithID(unitName.String()),
		status.StatusInfo{
			Status: status.Allocating,
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.UnitWorkloadNamespace.WithID(unitName.String()),
		status.StatusInfo{
			Status:  status.Active,
			Message: "message",
		})

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
	)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)

	// The storage directive is passed through to the subordinate unit's
	// storage arguments, with a new instance attached to the net node of
	// the machine hosting the principal unit.
	unitStorageArgs := subordinateStorageArgs.UnitStorageArgs
	c.Check(unitStorageArgs.StorageDirectives, tc.DeepEquals, []domainstorage.DirectiveArg{
		{
			Count:    1,
			Name:     "data",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	})
	c.Check(unitStorageArgs.StorageInstances, tc.HasLen, 1)
	instance := unitStorageArgs.StorageInstances[0]
	c.Check(instance.Name, tc.Equals, domainstorage.Name("data"))
	c.Check(instance.CharmName, tc.Equals, "sub-charm")
	c.Check(instance.Kind, tc.Equals, domainstorage.StorageKindFilesystem)
	c.Check(instance.RequestSizeMiB, tc.Equals, uint64(1024))
	c.Check(instance.StoragePoolUUID, tc.Equals, poolUUID)
	c.Check(unitStorageArgs.StorageToAttach, tc.HasLen, 1)
	attachment := unitStorageArgs.StorageToAttach[0]
	c.Check(attachment.StorageInstanceUUID, tc.Equals, instance.UUID)
	if attachment.FilesystemAttachment != nil {
		c.Check(attachment.FilesystemAttachment.NetNodeUUID, tc.Equals, machineNetNodeUUID)
		c.Check(attachment.FilesystemAttachment.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)
	}
	c.Check(unitStorageArgs.StorageToOwn, tc.DeepEquals, []domainstorage.StorageInstanceUUID{
		instance.UUID,
	})

	// The machine scoped filesystem of the new storage instance is owned
	// by the machine hosting the principal unit.
	c.Check(subordinateStorageArgs.IAASUnitStorageArgs.FilesystemsToOwn, tc.DeepEquals,
		[]domainstorage.FilesystemUUID{instance.Filesystem.UUID})
	c.Check(subordinateStorageArgs.IAASUnitStorageArgs.VolumesToOwn, tc.IsNil)
}

// TestEnterScopeCreatingSubordinateStorageArgsError tests that an error
// making the storage arguments for a new subordinate unit is returned when
// entering scope.
func (s *relationServiceSuite) TestEnterScopeCreatingSubordinateStorageArgsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	creationInfo := internal.SubordinateUnitCreationInfo{
		SubordinateApplicationUUID: appUUID,
		MachineNetNodeUUID:         machineNetNodeUUID,
		StorageDirectives: []domainstorageprovisioning.StorageDirective{
			{
				CharmMetadataName: "sub-charm",
				CharmStorageType:  charm.StorageFilesystem,
				Count:             1,
				MaxCount:          charm.StorageNoMaxCount,
				Name:              "data",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
		},
	}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(creationInfo, true, nil)
	expectedError := errors.New("boom")
	s.storagePoolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).
		Return(nil, expectedError)

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		map[string]string{},
	)

	// Assert.
	c.Assert(err, tc.ErrorIs, expectedError)
}

// TestEnterScopeCreatingSubordinateNilPoolProvider tests that a service
// wired without a storage pool provider returns a descriptive error when
// entering scope would create a subordinate unit, instead of panicking
// with a nil interface dereference. Services wired without a provider may
// still enter scope of relations that do not create subordinate units.
func (s *relationServiceSuite) TestEnterScopeCreatingSubordinateNilPoolProvider(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange. A service wired without a storage pool provider.
	svc := NewService(s.state, nil, s.statusHistory, loggertesting.WrapCheckLog(c))

	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	creationInfo := internal.SubordinateUnitCreationInfo{
		SubordinateApplicationUUID: appUUID,
		MachineNetNodeUUID:         machineNetNodeUUID,
	}
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(creationInfo, true, nil)

	// Act.
	err := svc.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		map[string]string{},
	)

	// Assert.
	c.Assert(err, tc.ErrorMatches, "storage pool provider not configured")
}

// TestEnterScopeCreatingSubordinateCreationInfoError tests that an error
// getting the subordinate unit creation info is returned when entering
// scope.
func (s *relationServiceSuite) TestEnterScopeCreatingSubordinateCreationInfoError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	expectedError := errors.New("boom")
	s.state.EXPECT().GetSubordinateUnitCreationInfo(gomock.Any(), relationUUID, unitName).
		Return(internal.SubordinateUnitCreationInfo{}, false, expectedError)

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		map[string]string{},
	)

	// Assert.
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *relationServiceSuite) TestEnterScopeRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitName := coreunittesting.GenNewName(c, "app1/0")

	// Act.
	err := s.service.EnterScope(c.Context(), "bad-uuid", unitName, map[string]string{})

	// Assert.
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestEnterScopeRelationUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act.
	err := s.service.EnterScope(c.Context(), relationUUID, "", map[string]string{})

	// Assert.
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestSetRelationRemoteApplicationAndUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	expectedApplicationSettings := map[string]string{"foo": "bar"}
	expectedUnitSettings := map[string]map[string]string{
		unitName.String(): {"ingress": "x.x.x.x"},
	}
	s.state.EXPECT().SetRelationRemoteApplicationAndUnitSettings(gomock.Any(),
		applicationUUID.String(),
		relationUUID.String(),
		expectedApplicationSettings,
		expectedUnitSettings,
	).Return(nil)

	applicationSettings := map[string]string{"foo": "bar", "empty": ""}
	unitSettings := map[coreunit.Name]map[string]string{
		coreunit.Name("app1/0"): {"ingress": "x.x.x.x"},
	}

	// Act.
	err := s.service.SetRelationRemoteApplicationAndUnitSettings(
		c.Context(),
		applicationUUID,
		relationUUID,
		applicationSettings,
		unitSettings,
	)
	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestSetRelationRemoteApplicationAndUnitSettingsInvalidApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationSettings := map[string]string{"foo": "bar"}
	unitSettings := map[coreunit.Name]map[string]string{
		coreunit.Name("app1/0"): {"ingress": "x.x.x.x"},
	}

	// Act.
	err := s.service.SetRelationRemoteApplicationAndUnitSettings(
		c.Context(),
		"bad-uuid",
		relationUUID,
		applicationSettings,
		unitSettings,
	)
	// Assert.
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestSetRelationRemoteApplicationAndUnitSettingsInvalidRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	applicationSettings := map[string]string{"foo": "bar"}
	unitSettings := map[coreunit.Name]map[string]string{
		coreunit.Name("app1/0"): {"ingress": "x.x.x.x"},
	}

	// Act.
	err := s.service.SetRelationRemoteApplicationAndUnitSettings(
		c.Context(),
		applicationUUID,
		"bad-uuid",
		applicationSettings,
		unitSettings,
	)
	// Assert.
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestSetRelationRemoteApplicationAndUnitSettingsInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationSettings := map[string]string{"foo": "bar"}
	unitSettings := map[coreunit.Name]map[string]string{
		coreunit.Name("!!!"): {"ingress": "x.x.x.x"},
	}

	// Act.
	err := s.service.SetRelationRemoteApplicationAndUnitSettings(
		c.Context(),
		applicationUUID,
		relationUUID,
		applicationSettings,
		unitSettings,
	)
	// Assert.
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestGetRelationUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	expectedSettings := map[string]string{
		"key": "value",
	}

	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)
	s.state.EXPECT().GetRelationUnitSettings(gomock.Any(), relationUnitUUID).Return(expectedSettings, nil)

	// Act:
	settings, err := s.service.GetRelationUnitSettings(c.Context(), relationUUID, unitName)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, expectedSettings)
}

func (s *relationServiceSuite) TestGetRelationUnitSettingsUnitIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetRelationUnitSettings(c.Context(), corerelationtesting.GenRelationUUID(c), "bad-uuid")
	c.Check(err, tc.ErrorMatches, "invalid unit name.*")
}

func (s *relationServiceSuite) TestGetRelationUnitSettingsRelationIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetRelationUnitSettings(c.Context(), "nah", coreunittesting.GenNewName(c, "app/0"))
	c.Check(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitSettingsFallback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app/0")
	expectedSettings := map[string]string{
		"key": "value",
	}

	exp := s.state.EXPECT()
	exp.GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return("", relationerrors.RelationUnitNotFound)
	exp.GetRelationUnitSettingsArchive(gomock.Any(), relationUUID.String(), unitName.String()).Return(expectedSettings, nil)

	// Act:
	settings, err := s.service.GetRelationUnitSettings(c.Context(), relationUUID, unitName)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, expectedSettings)
}

func (s *relationServiceSuite) TestGetRelationApplicationSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := tc.Must(c, coreapplication.NewUUID)
	expectedSettings := map[string]string{
		"key": "value",
	}
	s.state.EXPECT().GetRelationApplicationSettings(gomock.Any(), relationUUID, applicationID).Return(expectedSettings, nil)

	// Act:
	settings, err := s.service.GetRelationApplicationSettings(c.Context(), relationUUID, applicationID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, expectedSettings)
}

func (s *relationServiceSuite) TestGetRelationApplicationSettingsApplicationIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	_, err := s.service.GetRelationApplicationSettings(c.Context(), relationUUID, "bad-uuid")

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationApplicationSettingsRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	applicationID := tc.Must(c, coreapplication.NewUUID)

	// Act:
	_, err := s.service.GetRelationApplicationSettings(c.Context(), "bad-uuid", applicationID)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestApplicationRelationsInfoApplicationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.ApplicationRelationsInfo(c.Context(), "bad-uuid")

	// Assert.
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appID := tc.Must(c, coreapplication.NewUUID)
	expected := []relation.GoalStateRelationData{
		{Status: status.Joined},
		{Status: status.Joining},
	}
	s.state.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).Return(expected, nil)

	// Act
	obtained, err := s.service.GetGoalStateRelationDataForApplication(c.Context(), appID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplicationNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act:
	_, err := s.service.GetGoalStateRelationDataForApplication(c.Context(), "bad-uuid")

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).Return(nil, relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetGoalStateRelationDataForApplication(c.Context(), appID)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

// TestInferRelationUUIDByEndpoints verifies the behavior of the
// inferRelationUUIDByEndpoints method for finding a relation uuid.
func (s *relationServiceSuite) TestInferRelationUUIDByEndpoints(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1"
	endpoint2 := "application-2:endpoint-2"

	expectedRelUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().InferRelationUUIDByEndpoints(gomock.Any(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "endpoint-2",
	}).Return(expectedRelUUID, nil)

	// Act
	obtainedRelUUID, err := s.service.inferRelationUUIDByEndpoints(c.Context(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedRelUUID, tc.Equals, expectedRelUUID)
}

// TestAddRelationFirstMalformed verifies that inferRelationUUIDByEndpoints
// returns an appropriate error when the first endpoint is malformed.
func (s *relationServiceSuite) TestInferRelationUUIDByEndpointsFirstMalformed(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "app:ep:is:malformed"
	endpoint2 := "application-2:endpoint-2"

	// Act
	_, err := s.service.inferRelationUUIDByEndpoints(c.Context(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, tc.ErrorMatches, "parsing endpoint identifier \"app:ep:is:malformed\": expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
}

// TestAddRelationFirstMalformed verifies that InferRelationUUIDByEndpoints
// returns an appropriate error when the second endpoint is malformed.
func (s *relationServiceSuite) TestinferRelationUUIDByEndpointsSecondMalformed(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1:endpoint-1"
	endpoint2 := "app:ep:is:malformed"

	// Act
	_, err := s.service.inferRelationUUIDByEndpoints(c.Context(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, tc.ErrorMatches, "parsing endpoint identifier \"app:ep:is:malformed\": expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
}

// TestAddRelationStateError validates the inferRelationUUIDByEndpoints method
// handles and returns the correct error when state addition fails.
func (s *relationServiceSuite) TestInferRelationUUIDByEndpointsStateError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")

	s.state.EXPECT().InferRelationUUIDByEndpoints(gomock.Any(), gomock.Any(), gomock.Any()).Return("", expectedError)

	// Act
	_, err := s.service.inferRelationUUIDByEndpoints(c.Context(), "app1", "app2")

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *relationServiceSuite) TestGetRelationUUIDForRemovalEndpoints(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1"
	endpoint2 := "application-2:endpoint-2"

	expectedRelUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().InferRelationUUIDByEndpoints(gomock.Any(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "endpoint-2",
	}).Return(expectedRelUUID, nil)

	args := relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{endpoint1, endpoint2},
	}

	// Act
	obtainedRelUUID, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedRelUUID.String(), tc.Equals, expectedRelUUID.String())
}

func (s *relationServiceSuite) TestGetRelationUUIDForRemovalEndpointsFail(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1"
	endpoint2 := "application-2:endpoint-2"

	s.state.EXPECT().InferRelationUUIDByEndpoints(gomock.Any(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "endpoint-2",
	}).Return("", relationerrors.RelationNotFound)

	args := relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{endpoint1, endpoint2},
	}

	// Act
	_, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUUIDForRemovalID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedRelUUID := corerelationtesting.GenRelationUUID(c)

	args := relation.GetRelationUUIDForRemovalArgs{
		RelationID: 42,
	}
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), args.RelationID).Return(expectedRelUUID, nil)
	s.state.EXPECT().IsPeerRelation(gomock.Any(), expectedRelUUID.String()).Return(false, nil)

	// Act
	obtainedRelUUID, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedRelUUID.String(), tc.Equals, expectedRelUUID.String())
}

func (s *relationServiceSuite) TestGetRelationUUIDForRemovalIDFail(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	args := relation.GetRelationUUIDForRemovalArgs{
		RelationID: 42,
	}
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), args.RelationID).Return("", relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUUIDForRemovalIDIsPeer(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedRelUUID := corerelationtesting.GenRelationUUID(c)

	args := relation.GetRelationUUIDForRemovalArgs{
		RelationID: 42,
	}
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), args.RelationID).Return(expectedRelUUID, nil)
	s.state.EXPECT().IsPeerRelation(gomock.Any(), expectedRelUUID.String()).Return(true, nil)

	// Act
	_, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.NotNil)
}

func (s *relationServiceSuite) TestGetRelationUUIDForRemovalIDIsPeerFail(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedRelUUID := corerelationtesting.GenRelationUUID(c)

	args := relation.GetRelationUUIDForRemovalArgs{
		RelationID: 42,
	}
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), args.RelationID).Return(expectedRelUUID, nil)
	s.state.EXPECT().IsPeerRelation(gomock.Any(), expectedRelUUID.String()).Return(false, relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relUUID := corerelationtesting.GenRelationUUID(c)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	expected := relation.RelationUnitChange{
		RelationUUID: relUUID,
		Life:         corelife.Alive,
	}
	s.state.EXPECT().GetRelationUnitsChanges(gomock.Any(), relUUID, appUUID).Return(expected, nil)

	// Act
	obtained, err := s.service.GetRelationUnits(c.Context(), relUUID, appUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *relationServiceSuite) TestGetRelationUnitsFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relUUID := corerelationtesting.GenRelationUUID(c)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnitsChanges(gomock.Any(), relUUID, appUUID).Return(relation.RelationUnitChange{}, boom)

	// Act
	_, err := s.service.GetRelationUnits(c.Context(), relUUID, appUUID)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitsRelationUUIDNotValid(c *tc.C) {
	// Act
	_, err := s.service.GetRelationUnits(c.Context(), "bad-uuid", tc.Must(c, coreapplication.NewUUID))

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitsApplicationIDNotValid(c *tc.C) {
	// Act
	_, err := s.service.GetRelationUnits(c.Context(), corerelationtesting.GenRelationUUID(c), "bad-uuid")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetConsumerRelationUnitsChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relUUID := corerelationtesting.GenRelationUUID(c)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	expected := relation.ConsumerRelationUnitsChange{
		DepartedUnits: []string{"gone/1"},
	}
	s.state.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), relUUID.String(), appUUID.String()).Return(expected, nil)

	// Act
	obtained, err := s.service.GetConsumerRelationUnitsChange(c.Context(), relUUID, appUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *relationServiceSuite) TestGetConsumerRelationUnitsChangeFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relUUID := corerelationtesting.GenRelationUUID(c)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), relUUID.String(), appUUID.String()).Return(relation.ConsumerRelationUnitsChange{}, boom)

	// Act
	_, err := s.service.GetConsumerRelationUnitsChange(c.Context(), relUUID, appUUID)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetConsumerRelationUnitsChangeRelationUUIDNotValid(c *tc.C) {
	// Act
	_, err := s.service.GetConsumerRelationUnitsChange(c.Context(), "bad-uuid", tc.Must(c, coreapplication.NewUUID))

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetConsumerRelationUnitsChangeApplicationIDNotValid(c *tc.C) {
	// Act
	_, err := s.service.GetConsumerRelationUnitsChange(c.Context(), corerelationtesting.GenRelationUUID(c), "bad-uuid")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetFullRelationUnitChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relUUID := corerelationtesting.GenRelationUUID(c)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	expected := relation.FullRelationUnitChange{
		RelationUnitChange: relation.RelationUnitChange{
			AllUnits: []int{1},
			Life:     corelife.Alive,
		},
	}
	s.state.EXPECT().GetFullRelationUnitsChange(gomock.Any(), relUUID, appUUID).Return(expected, nil)

	// Act
	obtained, err := s.service.GetFullRelationUnitChange(c.Context(), relUUID, appUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *relationServiceSuite) TestGetFullRelationUnitChangeFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relUUID := corerelationtesting.GenRelationUUID(c)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetFullRelationUnitsChange(gomock.Any(), relUUID, appUUID).Return(relation.FullRelationUnitChange{}, boom)

	// Act
	_, err := s.service.GetFullRelationUnitChange(c.Context(), relUUID, appUUID)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetInScopeUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appUUID := tc.Must(c, coreapplication.NewUUID)
	relUUID := corerelationtesting.GenRelationUUID(c)
	s.state.EXPECT().GetInScopeUnits(gomock.Any(), appUUID.String(), relUUID.String()).Return([]string{"foo/1", "foo/2"}, nil)

	// Act
	unitNames, err := s.service.GetInScopeUnits(c.Context(), appUUID, relUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.DeepEquals, []coreunit.Name{"foo/1", "foo/2"})
}

func (s *relationServiceSuite) TestGetInScopeUnitsFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appUUID := tc.Must(c, coreapplication.NewUUID)
	relUUID := corerelationtesting.GenRelationUUID(c)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetInScopeUnits(gomock.Any(), appUUID.String(), relUUID.String()).Return(nil, boom)

	// Act
	_, err := s.service.GetInScopeUnits(c.Context(), appUUID, relUUID)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetUnitSettingsForUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitNames := []coreunit.Name{"app/0", "app/1"}
	relUUID := tc.Must(c, corerelation.NewUUID)

	res := []relation.UnitSettings{{
		UnitID:   0,
		Settings: map[string]string{"foo": "bar"},
	}, {
		UnitID:   1,
		Settings: map[string]string{"foo": "baz"},
	}}
	s.state.EXPECT().GetUnitSettingsForUnits(gomock.Any(), relUUID.String(), []string{"app/0", "app/1"}).Return(res, nil)

	// Act
	obtained, err := s.service.GetUnitSettingsForUnits(c.Context(), relUUID, unitNames)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.DeepEquals, res)
}

func (s *relationServiceSuite) TestGetUnitSettingsForUnitsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitNames := []coreunit.Name{"app/0", "app/1"}
	relUUID := tc.Must(c, corerelation.NewUUID)

	boom := errors.Errorf("boom")
	s.state.EXPECT().GetUnitSettingsForUnits(gomock.Any(), relUUID.String(), []string{"app/0", "app/1"}).Return(nil, boom)

	// Act
	_, err := s.service.GetUnitSettingsForUnits(c.Context(), relUUID, unitNames)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetFullRelationUnitChangeRelationUUIDNotValid(c *tc.C) {
	// Act
	_, err := s.service.GetFullRelationUnitChange(c.Context(), "bad-uuid", tc.Must(c, coreapplication.NewUUID))

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetFullRelationUnitChangeApplicationIDNotValid(c *tc.C) {
	// Act
	_, err := s.service.GetFullRelationUnitChange(c.Context(), corerelationtesting.GenRelationUUID(c), "bad-uuid")

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationServiceSuite) TestSetRelationErrorStatus(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationUUID := corerelationtesting.GenRelationUUID(c)
	message := "some error message"

	s.state.EXPECT().SetRelationErrorStatus(gomock.Any(), relationUUID.String(), message).Return(nil)

	// Act
	err := s.service.SetRelationErrorStatus(c.Context(), relationUUID, message)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestSetRelationErrorStatusRelationUUIDNotValid(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.service.SetRelationErrorStatus(c.Context(), "bad-uuid", "error message")

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestSetRelationErrorStatusStateError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationUUID := corerelationtesting.GenRelationUUID(c)
	message := "some error message"
	expectedErr := errors.New("boom")

	s.state.EXPECT().SetRelationErrorStatus(gomock.Any(), relationUUID.String(), message).Return(expectedErr)

	// Act
	err := s.service.SetRelationErrorStatus(c.Context(), relationUUID, message)

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *relationServiceSuite) TestGetRelationUUIDsByUnitName_Success(c *tc.C) {
	defer s.setupMocks(c).Finish()
	ctx := c.Context()
	unitName := coreunit.Name("foo/0")
	fakeUUIDs := []string{"rel-uuid-1", "rel-uuid-2"}

	s.state.EXPECT().GetRelationUUIDsByUnitName(ctx, unitName.String()).Return(fakeUUIDs, nil)

	relUUIDs, err := s.service.GetRelationUUIDsByUnitName(ctx, unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relUUIDs, tc.DeepEquals, []corerelation.UUID{"rel-uuid-1", "rel-uuid-2"})
}

func (s *relationServiceSuite) TestGetRelationUUIDsByUnitName_Error(c *tc.C) {
	defer s.setupMocks(c).Finish()
	ctx := c.Context()
	unitName := coreunit.Name("foo/0")
	s.state.EXPECT().GetRelationUUIDsByUnitName(ctx, unitName.String()).Return(nil, errors.New("fail"))

	relUUIDs, err := s.service.GetRelationUUIDsByUnitName(ctx, unitName)
	c.Assert(err, tc.ErrorMatches, "fail")
	c.Check(relUUIDs, tc.IsNil)
}

func (s *relationServiceSuite) TestGetRelationUUIDsByUnitName_InvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()
	ctx := c.Context()
	invalidUnitName := coreunit.Name("")

	relUUIDs, err := s.service.GetRelationUUIDsByUnitName(ctx, invalidUnitName)
	c.Assert(err, tc.NotNil)
	c.Check(relUUIDs, tc.IsNil)
}

type relationLeadershipServiceSuite struct {
	baseServiceSuite

	leadershipService *LeadershipService
	leaderEnsurer     *MockEnsurer
}

func TestRelationLeadershipServiceSuite(t *testing.T) {
	tc.Run(t, &relationLeadershipServiceSuite{})
}

func (s *relationLeadershipServiceSuite) TestGetRelationApplicationSettingsWithLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := tc.Must(c, coreapplication.NewUUID)
	s.expectWithLeader(unitName)
	expectedSettings := map[string]string{
		"key": "value",
	}
	s.state.EXPECT().GetRelationApplicationSettings(gomock.Any(), relationUUID, applicationID).Return(expectedSettings, nil)

	// Act:
	settings, err := s.leadershipService.GetRelationApplicationSettingsWithLeader(c.Context(), unitName, relationUUID, applicationID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, expectedSettings)
}

func (s *relationLeadershipServiceSuite) TestGetRelationApplicationSettingsWithLeaderUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := tc.Must(c, coreapplication.NewUUID)

	// Act:
	_, err := s.leadershipService.GetRelationApplicationSettingsWithLeader(c.Context(), "", relationUUID, applicationID)

	// Assert:
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationLeadershipServiceSuite) TestGetRelationApplicationSettingsWithLeaderApplicationIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	_, err := s.leadershipService.GetRelationApplicationSettingsWithLeader(c.Context(), unitName, relationUUID, "bad-uuid")

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestGetRelationApplicationSettingsWithLeaderRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	applicationID := tc.Must(c, coreapplication.NewUUID)

	// Act:
	_, err := s.leadershipService.GetRelationApplicationSettingsWithLeader(c.Context(), unitName, "bad-uuid", applicationID)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestSetRelationUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")

	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	unitSettings := map[string]string{
		"unitKey": "unitValue",
	}
	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, unitSettings, nil).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationUnitSettings(c.Context(), unitName, relationUUID, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationUnitSettingsNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	err := s.leadershipService.SetRelationUnitSettings(c.Context(), unitName, relationUUID, nil)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseServiceSuite.setupMocks(c)

	s.leaderEnsurer = NewMockEnsurer(ctrl)
	s.leadershipService = NewLeadershipService(s.state, s.storagePoolProvider, s.leaderEnsurer, s.statusHistory, loggertesting.WrapCheckLog(c))

	return ctrl
}

// expectWithLeader expects a call to with leader and executes the function to
// be run with leadership.
func (s *relationLeadershipServiceSuite) expectWithLeader(unitName coreunit.Name) {
	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
}
