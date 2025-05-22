// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corelease "github.com/juju/juju/core/lease"
	corelife "github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type relationServiceSuite struct {
	testhelpers.IsolationSuite

	state              *MockState
	subordinateCreator *MockSubordinateCreator

	service *Service
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

	results := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
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
	c.Assert(err, tc.ErrorIs, relationerrors.UnitUUIDNotValid)
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

func (s *relationServiceSuite) TestGetRelationUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return(unitUUID, nil)

	// Act
	uuid, err := s.service.GetRelationUnit(c.Context(), relationUUID, unitName)

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
	_, err := s.service.GetRelationUnit(c.Context(), relationUUID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunit.Name("not-valid-name")

	// Act
	_, err := s.service.GetRelationUnit(c.Context(), relationUUID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestGetRelationUnitUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return("", boom)

	// Act
	_, err := s.service.GetRelationUnit(c.Context(), relationUUID, unitName)

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
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return(unitUUID, nil)

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
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return("", boom)

	// Act
	_, err := s.service.getRelationUnitByID(c.Context(), relationID, unitName)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	appUUIDs := []coreapplication.ID{
		coreapplicationtesting.GenApplicationUUID(c),
		coreapplicationtesting.GenApplicationUUID(c),
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
	s.state.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDS, appUUIDs).Return(expectedResult, nil)

	// Act
	result, err := s.service.GetRelationUnitChanges(c.Context(), unitUUIDS, appUUIDs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expectedResult)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesUnitUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	unitUUIDS := []coreunit.UUID{
		coreunittesting.GenUnitUUID(c),
		coreunit.UUID("not-valid-uuid"),
		coreunittesting.GenUnitUUID(c),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), unitUUIDS, nil)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.UnitUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesAppUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	appUUIDs := []coreapplication.ID{
		coreapplicationtesting.GenApplicationUUID(c),
		coreapplication.ID("not-valid-uuid"),
		coreapplicationtesting.GenApplicationUUID(c),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), nil, appUUIDs)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesUnitStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnitChanges(gomock.Any(), gomock.Any(),
		gomock.Any()).Return(relation.RelationUnitsChange{}, boom)

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), nil, nil)

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
	}

	s.state.EXPECT().GetRelationDetails(gomock.Any(), relationUUID).Return(relationDetailsResult, nil)

	expectedRelationDetails := relation.RelationDetails{
		Life:      relationDetailsResult.Life,
		UUID:      relationDetailsResult.UUID,
		ID:        relationDetailsResult.ID,
		Key:       corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1"),
		Endpoints: relationDetailsResult.Endpoints,
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

func (s *relationServiceSuite) TestEnterScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings).Return(nil)
	s.state.EXPECT().NeedsSubordinateUnit(gomock.Any(), relationUUID, unitName).Return(nil, nil)

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
		nil,
	)
	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestEnterScopeCreatingSubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings).Return(nil)

	subAppID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().NeedsSubordinateUnit(gomock.Any(), relationUUID, unitName).Return(&subAppID, nil)
	s.subordinateCreator.EXPECT().CreateSubordinate(gomock.Any(), subAppID, unitName).Return(nil)

	// Act.
	err := s.service.EnterScope(
		c.Context(),
		relationUUID,
		unitName,
		settings,
		s.subordinateCreator,
	)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestEnterScopeRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitName := coreunittesting.GenNewName(c, "app1/0")

	// Act.
	err := s.service.EnterScope(c.Context(), "bad-uuid", unitName, map[string]string{}, nil)

	// Assert.
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestEnterScopeRelationUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act.
	err := s.service.EnterScope(c.Context(), relationUUID, "", map[string]string{}, nil)

	// Assert.
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestLeaveScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	s.state.EXPECT().LeaveScope(gomock.Any(), relationUnitUUID).Return(nil)

	// Act.
	err := s.service.LeaveScope(c.Context(), relationUnitUUID)

	// Assert.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestLeaveScopeRelationUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Act.
	err := s.service.LeaveScope(c.Context(), "bad-relation-unit-uuid")

	// Assert.
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	expectedSettings := map[string]string{
		"key": "value",
	}
	s.state.EXPECT().GetRelationUnitSettings(gomock.Any(), relationUnitUUID).Return(expectedSettings, nil)

	// Act:
	settings, err := s.service.GetRelationUnitSettings(c.Context(), relationUnitUUID)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, expectedSettings)
}

func (s *relationServiceSuite) TestGetRelationUnitSettingsUnitIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act:
	_, err := s.service.GetRelationUnitSettings(c.Context(), "bad-uuid")

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationApplicationSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
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
	c.Assert(err, tc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationApplicationSettingsRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

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
	c.Assert(err, tc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appID := coreapplicationtesting.GenApplicationUUID(c)
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
	c.Assert(err, tc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).Return(nil, relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetGoalStateRelationDataForApplication(c.Context(), appID)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestImportRelations(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key1 := corerelationtesting.GenNewKey(c, "ubuntu:peer")
	ep1 := key1.EndpointIdentifiers()
	key2 := corerelationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info")
	ep2 := key2.EndpointIdentifiers()

	args := relation.ImportRelationsArgs{
		{
			ID:  7,
			Key: key1,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep1[0].ApplicationName,
					EndpointName:        ep1[0].EndpointName,
					ApplicationSettings: map[string]interface{}{"five": "six"},
					UnitSettings: map[string]map[string]interface{}{
						"ubuntu/0": {"one": "two"},
					},
				},
			},
		}, {
			ID:  8,
			Key: key2,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep2[0].ApplicationName,
					EndpointName:        ep2[0].EndpointName,
					ApplicationSettings: map[string]interface{}{"foo": "six"},
					UnitSettings: map[string]map[string]interface{}{
						"ubuntu/0": {"test": "two"},
					},
				}, {
					ApplicationName:     ep2[1].ApplicationName,
					EndpointName:        ep2[1].EndpointName,
					ApplicationSettings: map[string]interface{}{"three": "four"},
					UnitSettings: map[string]map[string]interface{}{
						"ntp/0": {"seven": "six"},
					},
				},
			},
		},
	}
	peerRelUUID := s.expectGetPeerRelationUUIDByEndpointIdentifiers(c, ep1[0])
	relUUID := s.expectSetRelationWithID(c, ep2[0], ep2[1], uint64(8))
	app1ID := s.expectGetApplicationIDByName(c, args[0].Endpoints[0].ApplicationName)
	app2ID := s.expectGetApplicationIDByName(c, args[1].Endpoints[0].ApplicationName)
	app3ID := s.expectGetApplicationIDByName(c, args[1].Endpoints[1].ApplicationName)
	s.expectSetRelationApplicationSettings(peerRelUUID, app1ID, args[0].Endpoints[0].ApplicationSettings)
	s.expectSetRelationApplicationSettings(relUUID, app2ID, args[1].Endpoints[0].ApplicationSettings)
	s.expectSetRelationApplicationSettings(relUUID, app3ID, args[1].Endpoints[1].ApplicationSettings)
	settings := args[0].Endpoints[0].UnitSettings["ubuntu/0"]
	s.expectEnterScope(peerRelUUID, coreunittesting.GenNewName(c, "ubuntu/0"), settings)
	settings = args[1].Endpoints[0].UnitSettings["ubuntu/0"]
	s.expectEnterScope(relUUID, coreunittesting.GenNewName(c, "ubuntu/0"), settings)
	settings = args[1].Endpoints[1].UnitSettings["ntp/0"]
	s.expectEnterScope(relUUID, coreunittesting.GenNewName(c, "ntp/0"), settings)

	// Act
	err := s.service.ImportRelations(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationServiceSuite) TestDeleteImportedRelationsError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeleteImportedRelations(gomock.Any()).Return(errors.New("boom"))

	// Act
	err := s.service.DeleteImportedRelations(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *relationServiceSuite) TestExportRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.state.EXPECT().ExportRelations(gomock.Any()).Return([]relation.ExportRelation{{
		Endpoints: []relation.ExportEndpoint{{
			ApplicationName: "app1",
			Name:            "ep1",
			Role:            internalcharm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			Name:            "ep2",
			Role:            internalcharm.RoleProvider,
		}},
	}}, nil)

	// Act:
	relations, err := s.service.ExportRelations(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relations, tc.DeepEquals, []relation.ExportRelation{{
		Endpoints: []relation.ExportEndpoint{{
			ApplicationName: "app1",
			Name:            "ep1",
			Role:            internalcharm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			Name:            "ep2",
			Role:            internalcharm.RoleProvider,
		}},
		Key: corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2"),
	}})
}

func (s *relationServiceSuite) TestExportRelationsStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	boom := errors.New("boom")
	s.state.EXPECT().ExportRelations(gomock.Any()).Return(nil, boom)

	// Act:
	_, err := s.service.ExportRelations(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIs, boom)
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
	s.state.EXPECT().IsPeerRelation(gomock.Any(), expectedRelUUID).Return(false, nil)

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
	s.state.EXPECT().IsPeerRelation(gomock.Any(), expectedRelUUID).Return(true, nil)

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
	s.state.EXPECT().IsPeerRelation(gomock.Any(), expectedRelUUID).Return(false, relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetRelationUUIDForRemoval(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.subordinateCreator = NewMockSubordinateCreator(ctrl)

	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}

func (s *relationServiceSuite) expectGetPeerRelationUUIDByEndpointIdentifiers(
	c *tc.C,
	endpoint corerelation.EndpointIdentifier,
) corerelation.UUID {
	relUUID := corerelationtesting.GenRelationUUID(c)
	s.state.EXPECT().GetPeerRelationUUIDByEndpointIdentifiers(gomock.Any(), endpoint).Return(relUUID, nil)
	return relUUID
}

func (s *relationServiceSuite) expectSetRelationWithID(
	c *tc.C,
	ep2, ep3 corerelation.EndpointIdentifier,
	id uint64,
) corerelation.UUID {
	relUUID := corerelationtesting.GenRelationUUID(c)
	s.state.EXPECT().SetRelationWithID(gomock.Any(), ep2, ep3, id).Return(relUUID, nil)
	return relUUID
}

func (s *relationServiceSuite) expectGetApplicationIDByName(c *tc.C, name string) coreapplication.ID {
	appID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), name).Return(appID, nil)
	return appID
}

func (s *relationServiceSuite) expectSetRelationApplicationSettings(
	uuid corerelation.UUID,
	id coreapplication.ID,
	settings map[string]interface{},
) {
	appSettings, _ := settingsMap(settings)
	s.state.EXPECT().SetRelationApplicationSettings(gomock.Any(), uuid, id, appSettings).Return(nil)
}

func (s *relationServiceSuite) expectEnterScope(
	uuid corerelation.UUID,
	name coreunit.Name,
	settings map[string]interface{},
) {
	unitSettings, _ := settingsMap(settings)
	s.state.EXPECT().EnterScope(gomock.Any(), uuid, name, unitSettings).Return(nil)
}

type relationLeadershipServiceSuite struct {
	relationServiceSuite

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
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	s.expectWithLeader(c, unitName)
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
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

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
	c.Assert(err, tc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestGetRelationApplicationSettingsWithLeaderRelationUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	// Act:
	_, err := s.leadershipService.GetRelationApplicationSettingsWithLeader(c.Context(), unitName, "bad-uuid", applicationID)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	s.expectWithLeader(c, unitName)

	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	appSettings := map[string]string{
		"appKey": "appValue",
	}
	unitSettings := map[string]string{
		"unitKey": "unitValue",
	}
	s.state.EXPECT().SetRelationApplicationAndUnitSettings(gomock.Any(), relationUnitUUID, appSettings, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUnitUUID, appSettings, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetRelationApplicationAndUnitSettingsOnlyUnit checks that if only the
// unit settings are changed, leadership is not checked.
func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsOnlyUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	unitSettings := map[string]string{
		"unitKey": "unitValue",
	}
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUnitUUID, nil, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUnitUUID, nil, nil)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, "bad-uuid", make(map[string]string), make(map[string]string))

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsLeaseNotHeld(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).Return(corelease.ErrNotHeld)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUnitUUID, settings, settings)

	// Assert:
	c.Assert(err, tc.ErrorIs, corelease.ErrNotHeld)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), "", relationUnitUUID, settings, nil)

	// Assert:
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsRelationUnitUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, "bad-uuid", nil, settings)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.relationServiceSuite.setupMocks(c)

	s.leaderEnsurer = NewMockEnsurer(ctrl)
	s.leadershipService = NewLeadershipService(s.state, s.leaderEnsurer, loggertesting.WrapCheckLog(c))

	return ctrl
}

// expectWithLeader expects a call to with leader and executes the function to
// be run with leadership.
func (s *relationLeadershipServiceSuite) expectWithLeader(c *tc.C, unitName coreunit.Name) {
	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
}
