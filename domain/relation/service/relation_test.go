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
	corelife "github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationServiceSuite struct {
	jujutesting.IsolationSuite

	state *MockState

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

	s.state.EXPECT().EnterScope(gomock.Any(), relationUUID, unitName).Return(nil)

	// Act.
	err := s.service.EnterScope(context.Background(), relationUUID, unitName)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationServiceSuite) TestEnterScopeRelationUUIDNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	unitName := coreunittesting.GenNewName(c, "app1/0")

	// Act.
	err := s.service.EnterScope(context.Background(), "bad-uuid", unitName)

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationUUIDNotValid)
}

func (s *relationServiceSuite) TestEnterScopeRelationUnitNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act.
	err := s.service.EnterScope(context.Background(), relationUUID, "")

	// Assert.
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
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

func (s *relationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}
