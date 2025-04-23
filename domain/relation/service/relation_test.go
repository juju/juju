// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	coreerrors "github.com/juju/juju/core/errors"
	corelease "github.com/juju/juju/core/lease"
	corelife "github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationServiceSuite struct {
	jujutesting.IsolationSuite

	state              *MockState
	subordinateCreator *MockSubordinateCreator

	service *Service
}

var _ = gc.Suite(&relationServiceSuite{})

// TestAddRelation verifies the behavior of the AddRelation method when adding
// a relation between two endpoints.
func (s *relationServiceSuite) TestAddRelation(c *gc.C) {
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
	gotEp1, gotEp2, err := s.service.AddRelation(context.Background(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotEp1, gc.Equals, fakeReturn1)
	c.Check(gotEp2, gc.Equals, fakeReturn2)
}

// TestAddRelationFirstMalformed verifies that AddRelation returns an
// appropriate error when the first endpoint is malformed.
func (s *relationServiceSuite) TestAddRelationFirstMalformed(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "app:ep:is:malformed"
	endpoint2 := "application-2:endpoint-2"

	// Act
	_, _, err := s.service.AddRelation(context.Background(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, gc.ErrorMatches, "parsing endpoint identifier \"app:ep:is:malformed\": expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
}

// TestAddRelationFirstMalformed verifies that AddRelation returns an
// appropriate error when the second endpoint is malformed.
func (s *relationServiceSuite) TestAddRelationSecondMalformed(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	endpoint1 := "application-1:endpoint-1"
	endpoint2 := "app:ep:is:malformed"

	// Act
	_, _, err := s.service.AddRelation(context.Background(), endpoint1, endpoint2)

	// Assert
	c.Assert(err, gc.ErrorMatches, "parsing endpoint identifier \"app:ep:is:malformed\": expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
}

// TestAddRelationStateError validates the AddRelation method handles and
// returns the correct error when state addition fails.
func (s *relationServiceSuite) TestAddRelationStateError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	var empty relation.Endpoint

	s.state.EXPECT().AddRelation(gomock.Any(), gomock.Any(), gomock.Any()).Return(empty, empty, expectedError)

	// Act
	_, _, err := s.service.AddRelation(context.Background(), "app1", "app2")

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}

// TestGetAllRelationDetails verifies that GetAllRelationDetails
// retrieves and returns the expected relation details without errors.
// Doesn't have logic, so the test doesn't need to be smart.
func (s *relationServiceSuite) TestGetAllRelationDetails(c *gc.C) {
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
	details, err := s.service.GetAllRelationDetails(context.Background())

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(details, gc.DeepEquals, expectedRelationDetails)
}

// TestGetAllRelationDetailsError verifies the behavior when GetAllRelationDetails
// encounters an error from the state layer.
func (s *relationServiceSuite) TestGetAllRelationDetailsError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	s.state.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.service.GetAllRelationDetails(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}

func (s *relationServiceSuite) TestGetApplicationRelations(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedRelations := []corerelation.UUID{
		corerelationtesting.GenRelationUUID(c),
		corerelationtesting.GenRelationUUID(c),
		corerelationtesting.GenRelationUUID(c),
	}
	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationRelations(gomock.Any(), appUUID).Return(expectedRelations, nil)

	// Act
	relations, err := s.service.GetApplicationRelations(context.Background(), appUUID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relations, gc.DeepEquals, expectedRelations)
}

func (s *relationServiceSuite) TestGetApplicationRelationsApplicationUUIDNotValid(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	_, err := s.service.GetApplicationRelations(context.Background(), "not valid app uuid")

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetApplicationRelationsStateError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	s.state.EXPECT().GetApplicationRelations(gomock.Any(), gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.service.GetApplicationRelations(context.Background(), coreapplicationtesting.GenApplicationUUID(c))

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}

// TestGetRelationEndpointUUID tests the GetRelationEndpointUUID method for
// valid input and expected successful execution.
func (s *relationServiceSuite) TestGetRelationEndpointUUID(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	appUUID, err := coreapplication.NewID()
	c.Assert(err, gc.IsNil, gc.Commentf("(Arrange) can't generate appUUID: %v", err))
	relationUUID := corerelationtesting.GenRelationUUID(c)

	args := relation.GetRelationEndpointUUIDArgs{
		ApplicationID: appUUID,
		RelationUUID:  relationUUID,
	}
	s.state.EXPECT().GetRelationEndpointUUID(gomock.Any(), args).Return("some-uuid", nil)

	// Act
	obtained, err := s.service.getRelationEndpointUUID(context.Background(), args)
	c.Assert(err, gc.IsNil, gc.Commentf("(Act) unexpected error: %v", err))

	// Assert
	c.Check(obtained, gc.Equals, corerelation.EndpointUUID("some-uuid"), gc.Commentf("(Assert) unexpected result: %v", obtained))
}

// TestGetRelationEndpointUUIDApplicationIDNotValid tests the failure case
// where the ApplicationID is not a valid UUID.
func (s *relationServiceSuite) TestGetRelationEndpointUUIDApplicationIDNotValid(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationUUID := corerelationtesting.GenRelationUUID(c)

	args := relation.GetRelationEndpointUUIDArgs{
		ApplicationID: "not-valid-uuid",
		RelationUUID:  relationUUID,
	}

	// Act
	_, err := s.service.getRelationEndpointUUID(context.Background(), args)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid, gc.Commentf("(Assert) unexpected error: %v", err))
}

// TestGetRelationEndpointUUIDRelationUUIDNotValid tests the failure scenario
// where the provided RelationUUID is not valid.
func (s *relationServiceSuite) TestGetRelationEndpointUUIDRelationUUIDNotValid(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	appUUID := coreapplicationtesting.GenApplicationUUID(c)

	args := relation.GetRelationEndpointUUIDArgs{
		ApplicationID: appUUID,
		RelationUUID:  "not-valid-uuid",
	}

	// Act
	_, err := s.service.getRelationEndpointUUID(context.Background(), args)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid, gc.Commentf("(Assert) unexpected error: %v", err))
}

func (s *relationServiceSuite) TestGetRelationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	expectedRelationID := 1

	s.state.EXPECT().GetRelationID(gomock.Any(), relationUUID).Return(expectedRelationID, nil)

	// Act.
	relationID, err := s.service.GetRelationID(context.Background(), relationUUID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationID, gc.Equals, expectedRelationID)
}

func (s *relationServiceSuite) TestGetRelationIDRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.GetRelationID(context.Background(), "bad-relation-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitEndpointName(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedName := "fake-endpoint-name"
	relationUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUnitEndpointName(gomock.Any(), relationUUID).Return(expectedName, nil)

	// Act
	name, err := s.service.GetRelationUnitEndpointName(context.Background(), relationUUID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, expectedName)
}

func (s *relationServiceSuite) TestGetRelationUnitEndpointNameRelationUnitUUIDNotValid(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationUnitUUID := corerelation.UnitUUID("not-valid-uuid")

	// Act
	_, err := s.service.GetRelationUnitEndpointName(context.Background(), relationUnitUUID)

	// Assert
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitEndpointNameStateError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUnitEndpointName(gomock.Any(), relationUnitUUID).Return("", expectedError)

	// Act
	_, err := s.service.GetRelationUnitEndpointName(context.Background(), relationUnitUUID)

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}

func (s *relationServiceSuite) TestGetRelationUUIDByID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)
	relationID := 1

	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(expectedRelationUUID, nil)

	// Act.
	relationUUID, err := s.service.GetRelationUUIDByID(context.Background(), relationID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationUUID, gc.Equals, expectedRelationUUID)
}

func (s *relationServiceSuite) TestGetRelationKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

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

	expectedKey := corerelationtesting.GenNewKey(c, "app-2:fake-endpoint-name-2 app-1:fake-endpoint-name-1")
	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relationUUID).Return([]relation.Endpoint{
		endpoint1,
		endpoint2,
	}, nil)

	// Act:
	key, err := s.service.GetRelationKey(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key, gc.DeepEquals, expectedKey)
}

func (s *relationServiceSuite) TestGetRelationKeyPeer(c *gc.C) {
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

	expectedKey := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1")
	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relationUUID).Return([]relation.Endpoint{
		endpoint1,
	}, nil)

	// Act:
	key, err := s.service.GetRelationKey(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key, gc.DeepEquals, expectedKey)
}

func (s *relationServiceSuite) TestGetRelationKeyRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relationUUID).Return(nil, relationerrors.RelationNotFound)

	// Act:
	_, err := s.service.GetRelationKey(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationKeyRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Act:
	_, err := s.service.GetRelationKey(context.Background(), "bad-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyPeer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1")

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetPeerRelationUUIDByEndpointIdentifiers(
		gomock.Any(), key.EndpointIdentifiers()[0],
	).Return(expectedRelationUUID, nil)

	// Act:
	uuid, err := s.service.GetRelationUUIDByKey(context.Background(), key)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, expectedRelationUUID)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRegular(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	eids := key.EndpointIdentifiers()

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), eids[0], eids[1],
	).Return(expectedRelationUUID, nil)

	// Act:
	uuid, err := s.service.GetRelationUUIDByKey(context.Background(), key)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, expectedRelationUUID)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return("", relationerrors.RelationNotFound)

	// Act:
	_, err := s.service.GetRelationUUIDByKey(
		context.Background(),
		corerelationtesting.GenNewKey(c, "app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2"),
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRelationKeyNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act:
	_, err := s.service.GetRelationUUIDByKey(context.Background(), corerelation.Key{})

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationKeyNotValid)
}

func (s *relationServiceSuite) TestGetRelationsStatusForUnit(c *gc.C) {
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
	statuses, err := s.service.GetRelationsStatusForUnit(context.Background(), unitUUID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statuses, gc.DeepEquals, expectedStatuses)
}

func (s *relationServiceSuite) TestGetRelationsStatusForUnitUnitUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.GetRelationsStatusForUnit(context.Background(), "bad-unit-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.UnitUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationsStatusForUnitStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitUUID := coreunittesting.GenUnitUUID(c)
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationsStatusForUnit(gomock.Any(), unitUUID).Return(nil, boom)

	// Act.
	_, err := s.service.GetRelationsStatusForUnit(context.Background(), unitUUID)

	// Assert.
	c.Assert(err, jc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return(unitUUID, nil)

	// Act
	uuid, err := s.service.GetRelationUnit(context.Background(), relationUUID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, unitUUID)
}

func (s *relationServiceSuite) TestGetRelationUnitRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelation.UUID("not-valid-uuid")
	unitName := coreunittesting.GenNewName(c, "app1/0")

	// Act
	_, err := s.service.GetRelationUnit(context.Background(), relationUUID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunit.Name("not-valid-name")

	// Act
	_, err := s.service.GetRelationUnit(context.Background(), relationUUID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestGetRelationUnitUnitStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return("", boom)

	// Act
	_, err := s.service.GetRelationUnit(context.Background(), relationUUID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitByID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationID := 42
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(relationUUID, nil)
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return(unitUUID, nil)

	// Act
	uuid, err := s.service.GetRelationUnitByID(context.Background(), relationID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, unitUUID)
}

func (s *relationServiceSuite) TestGetRelationUnitByIDRelationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationID := 42
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return("", relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetRelationUnitByID(context.Background(), relationID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUnitByIDUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	unitName := coreunit.Name("not-valid-name")

	// Act
	_, err := s.service.GetRelationUnitByID(context.Background(), 42, unitName)

	// Assert
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestGetRelationUnitByIDUnitStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	relationID := 42
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(relationUUID, nil)
	s.state.EXPECT().GetRelationUnit(gomock.Any(), relationUUID, unitName).Return("", boom)

	// Act
	_, err := s.service.GetRelationUnitByID(context.Background(), relationID, unitName)

	// Assert
	c.Assert(err, jc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationUnitChanges(c *gc.C) {
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
	expectedResult := watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			unitUUIDS[1].String(): {Version: 42},
			unitUUIDS[2].String(): {Version: 43},
		},
		AppChanged: map[string]int64{
			appUUIDs[0].String(): 42,
			appUUIDs[1].String(): 43,
		},
		Departed: []string{unitUUIDS[0].String()},
	}
	s.state.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDS, appUUIDs).Return(expectedResult, nil)

	// Act
	result, err := s.service.GetRelationUnitChanges(context.Background(), unitUUIDS, appUUIDs)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expectedResult)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesUnitUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	unitUUIDS := []coreunit.UUID{
		coreunittesting.GenUnitUUID(c),
		coreunit.UUID("not-valid-uuid"),
		coreunittesting.GenUnitUUID(c),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(context.Background(), unitUUIDS, nil)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.UnitUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesAppUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	appUUIDs := []coreapplication.ID{
		coreapplicationtesting.GenApplicationUUID(c),
		coreapplication.ID("not-valid-uuid"),
		coreapplicationtesting.GenApplicationUUID(c),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(context.Background(), nil, appUUIDs)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesUnitStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	boom := errors.Errorf("boom")
	s.state.EXPECT().GetRelationUnitChanges(gomock.Any(), gomock.Any(),
		gomock.Any()).Return(watcher.RelationUnitsChange{}, boom)

	// Act
	_, err := s.service.GetRelationUnitChanges(context.Background(), nil, nil)

	// Assert
	c.Assert(err, jc.ErrorIs, boom)
}

func (s *relationServiceSuite) TestGetRelationDetails(c *gc.C) {
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
	relationDetails, err := s.service.GetRelationDetails(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(relationDetails, gc.DeepEquals, expectedRelationDetails)
}

// TestGetRelationEndpointUUIDRelationUUIDNotValid tests the failure scenario
// where the provided RelationUUID is not valid.
func (s *relationServiceSuite) TestGetRelationDetailsRelationUUIDNotValid(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	_, err := s.service.GetRelationDetails(context.Background(), "bad-relation-uuid")

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid, gc.Commentf("(Assert) unexpected error: %v", err))
}

func (s *relationServiceSuite) TestEnterScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	settings := map[string]string{"ingress": "x.x.x.x"}
	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName, settings).Return(nil)
	s.state.EXPECT().NeedsSubordinateUnit(gomock.Any(), relationUUID, unitName).Return(nil, nil)

	// Act.
	err := s.service.EnterScope(
		context.Background(),
		relationUUID,
		unitName,
		settings,
		nil,
	)
	// Assert.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestEnterScopeCreatingSubordinate(c *gc.C) {
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
		context.Background(),
		relationUUID,
		unitName,
		settings,
		s.subordinateCreator,
	)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestEnterScopeRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitName := coreunittesting.GenNewName(c, "app1/0")

	// Act.
	err := s.service.EnterScope(context.Background(), "bad-uuid", unitName, map[string]string{}, nil)

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestEnterScopeRelationUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act.
	err := s.service.EnterScope(context.Background(), relationUUID, "", map[string]string{}, nil)

	// Assert.
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationServiceSuite) TestLeaveScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	s.state.EXPECT().LeaveScope(gomock.Any(), relationUnitUUID).Return(nil)

	// Act.
	err := s.service.LeaveScope(context.Background(), relationUnitUUID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestLeaveScopeRelationUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Act.
	err := s.service.LeaveScope(context.Background(), "bad-relation-unit-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	expectedEndpoints := []relation.Endpoint{{
		ApplicationName: "app-2",
	}, {
		ApplicationName: "app-1",
	}}

	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relationUUID).Return(expectedEndpoints, nil)

	// Act:
	endpoints, err := s.service.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(endpoints, gc.DeepEquals, expectedEndpoints)
}

func (s *relationServiceSuite) TestGetRelationEndpointsRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.GetRelationEndpoints(context.Background(), "bad-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestGetApplicationEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	expectedEndpoints := []relation.Endpoint{{
		ApplicationName: "app-2",
	}, {
		ApplicationName: "app-1",
	}}

	s.state.EXPECT().GetApplicationEndpoints(gomock.Any(), applicationID).Return(expectedEndpoints, nil)

	// Act:
	endpoints, err := s.service.GetApplicationEndpoints(context.Background(), applicationID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(endpoints, jc.SameContents, expectedEndpoints)
}

func (s *relationServiceSuite) TestGetApplicationEndpointsEmptySlice(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationEndpoints(gomock.Any(), applicationID).Return(nil, nil)

	// Act:
	endpoints, err := s.service.GetApplicationEndpoints(context.Background(), applicationID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(endpoints, gc.HasLen, 0)
}

func (s *relationServiceSuite) TestGetApplicationEndpointsApplicationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.GetApplicationEndpoints(context.Background(), "bad-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	expectedSettings := map[string]string{
		"key": "value",
	}
	s.state.EXPECT().GetRelationUnitSettings(gomock.Any(), relationUnitUUID).Return(expectedSettings, nil)

	// Act:
	settings, err := s.service.GetRelationUnitSettings(context.Background(), relationUnitUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expectedSettings)
}

func (s *relationServiceSuite) TestGetRelationUnitSettingsUnitIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act:
	_, err := s.service.GetRelationUnitSettings(context.Background(), "bad-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestSetRelationUnitSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{
		"key": "value",
	}
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, settings).Return(nil)

	// Act:
	err := s.service.SetRelationUnitSettings(context.Background(), relationUnitUUID, settings)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestApplicationRelationsInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationEndpoints(gomock.Any(), applicationID).Return(nil, nil)

	// Act:
	_, err := s.service.GetApplicationEndpoints(context.Background(), applicationID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestApplicationRelationsInfoError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationEndpoints(gomock.Any(), applicationID).Return(nil, relationerrors.UnitNotFound)

	// Act:
	_, err := s.service.GetApplicationEndpoints(context.Background(), applicationID)

	// Assert: service returned the error from state without translation.
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotFound)
}

func (s *relationServiceSuite) TestSetRelationUnitSettingsEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	// Act:
	err := s.service.SetRelationUnitSettings(context.Background(), relationUnitUUID, make(map[string]string))

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestSetRelationUnitSettingsNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	// Act:
	err := s.service.SetRelationUnitSettings(context.Background(), relationUnitUUID, nil)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestSetRelationUnitSettingsRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.service.SetRelationUnitSettings(context.Background(), "bad-uuid", settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestApplicationRelationsInfoApplicationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act.
	_, err := s.service.ApplicationRelationsInfo(context.Background(), "bad-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appID := coreapplicationtesting.GenApplicationUUID(c)
	expected := []relation.GoalStateRelationData{
		{Status: status.Joined},
		{Status: status.Joining},
	}
	s.state.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).Return(expected, nil)

	// Act
	obtained, err := s.service.GetGoalStateRelationDataForApplication(context.Background(), appID)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, gc.DeepEquals, expected)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplicationNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Act:
	_, err := s.service.GetGoalStateRelationDataForApplication(context.Background(), "bad-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationServiceSuite) TestGetGoalStateRelationDataForApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).Return(nil, relationerrors.RelationNotFound)

	// Act
	_, err := s.service.GetGoalStateRelationDataForApplication(context.Background(), appID)

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.subordinateCreator = NewMockSubordinateCreator(ctrl)

	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}

type relationLeadershipServiceSuite struct {
	relationServiceSuite

	leadershipService *LeadershipService
	leaderEnsurer     *MockEnsurer
}

var _ = gc.Suite(&relationLeadershipServiceSuite{})

func (s *relationLeadershipServiceSuite) TestGetLocalRelationApplicationSettings(c *gc.C) {
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
	settings, err := s.leadershipService.GetLocalRelationApplicationSettings(context.Background(), unitName, relationUUID, applicationID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, expectedSettings)
}

func (s *relationLeadershipServiceSuite) TestGetLocalRelationApplicationSettingsUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	// Act:
	_, err := s.leadershipService.GetLocalRelationApplicationSettings(context.Background(), "", relationUUID, applicationID)

	// Assert:
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationLeadershipServiceSuite) TestGetLocalRelationApplicationSettingsApplicationIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	_, err := s.leadershipService.GetLocalRelationApplicationSettings(context.Background(), unitName, relationUUID, "bad-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestGetLocalRelationApplicationSettingsRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	// Act:
	_, err := s.leadershipService.GetLocalRelationApplicationSettings(context.Background(), unitName, "bad-uuid", applicationID)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	s.expectWithLeader(c, unitName)
	settings := map[string]string{
		"key": "value",
	}
	s.state.EXPECT().SetRelationApplicationSettings(gomock.Any(), relationUUID, applicationID, settings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), unitName, relationUUID, applicationID, settings)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettingsNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), unitName, "bad-uuid", applicationID, nil)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettingsEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	applicationID := coreapplicationtesting.GenApplicationUUID(c)

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), unitName, "bad-uuid", applicationID, make(map[string]string))

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettingsLeaseNotHeld(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).Return(corelease.ErrNotHeld)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), unitName, relationUUID, applicationID, settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, corelease.ErrNotHeld)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettingsUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), "", relationUUID, applicationID, settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettingsApplicationIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), unitName, relationUUID, "bad-uuid", settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.ApplicationIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationSettingsRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	applicationID := coreapplicationtesting.GenApplicationUUID(c)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationSettings(context.Background(), unitName, "bad-uuid", applicationID, settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettings(c *gc.C) {
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
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), unitName, relationUnitUUID, appSettings, unitSettings)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

// TestSetRelationApplicationAndUnitSettingsOnlyUnit checks that if only the
// unit settings are changed, leadership is not checked.
func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsOnlyUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	unitSettings := map[string]string{
		"unitKey": "unitValue",
	}
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), unitName, relationUnitUUID, nil, unitSettings)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), unitName, relationUnitUUID, nil, nil)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), unitName, "bad-uuid", make(map[string]string), make(map[string]string))

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsLeaseNotHeld(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).Return(corelease.ErrNotHeld)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), unitName, relationUnitUUID, settings, settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, corelease.ErrNotHeld)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), "", relationUnitUUID, settings, nil)

	// Assert:
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsRelationUnitUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(context.Background(), unitName, "bad-uuid", nil, settings)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationLeadershipServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.relationServiceSuite.setupMocks(c)

	s.leaderEnsurer = NewMockEnsurer(ctrl)
	s.leadershipService = NewLeadershipService(s.state, s.leaderEnsurer, loggertesting.WrapCheckLog(c))

	return ctrl
}

// expectWithLeader expects a call to with leader and executes the function to
// be run with leadership.
func (s *relationLeadershipServiceSuite) expectWithLeader(c *gc.C, unitName coreunit.Name) {
	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
}
