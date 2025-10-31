// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	corelease "github.com/juju/juju/core/lease"
	corelife "github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitUUIDNotValid)
}

func (s *relationServiceSuite) TestGetRelationUnitChangesAppUUIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	appUUIDs := []coreapplication.UUID{
		tc.Must(c, coreapplication.NewUUID),
		coreapplication.UUID("not-valid-uuid"),
		tc.Must(c, coreapplication.NewUUID),
	}

	// Act
	_, err := s.service.GetRelationUnitChanges(c.Context(), nil, appUUIDs)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationUUIDNotValid)
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

	subAppID := tc.Must(c, coreapplication.NewUUID)
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

func (s *relationServiceSuite) TestSetRelationRemoteApplicationAndUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange.
	applicationUUID := tc.Must(c, coreapplication.NewUUID)
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	applicationSettings := map[string]string{"foo": "bar"}
	unitSettings := map[coreunit.Name]map[string]string{
		coreunit.Name("app1/0"): {"ingress": "x.x.x.x"},
	}
	expectedUnitSettings := map[string]map[string]string{
		unitName.String(): unitSettings[unitName],
	}
	s.state.EXPECT().SetRelationRemoteApplicationAndUnitSettings(gomock.Any(), applicationUUID.String(), relationUUID.String(), applicationSettings, expectedUnitSettings).Return(nil)

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
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationUnitSettings(c.Context(), unitName, relationUUID, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationUnitSettingsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")

	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	unitSettings := make(map[string]string)

	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, unitSettings).Return(nil)

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

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	s.expectWithLeader(c, unitName)

	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	appSettings := map[string]string{
		"appKey": "appValue",
	}
	unitSettings := map[string]string{
		"unitKey": "unitValue",
	}
	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)
	s.state.EXPECT().SetRelationApplicationAndUnitSettings(gomock.Any(), relationUnitUUID, appSettings, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUUID, appSettings, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	s.expectWithLeader(c, unitName)

	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	applicationSettings := make(map[string]string)
	unitSettings := make(map[string]string)

	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)
	s.state.EXPECT().SetRelationApplicationAndUnitSettings(gomock.Any(), relationUnitUUID, applicationSettings, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUUID, applicationSettings, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsBothNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUUID, nil, nil)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsAppSettingsNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	unitSettings := make(map[string]string)

	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)
	s.state.EXPECT().SetRelationUnitSettings(gomock.Any(), relationUnitUUID, unitSettings).Return(nil)

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUUID, nil, unitSettings)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsLeaseNotHeld(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunittesting.GenNewName(c, "app/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)

	s.state.EXPECT().GetRelationUnitUUID(gomock.Any(), relationUUID, unitName).Return(relationUnitUUID, nil)

	s.leaderEnsurer.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).Return(corelease.ErrNotHeld)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), unitName, relationUUID, settings, settings)

	// Assert:
	c.Assert(err, tc.ErrorIs, corelease.ErrNotHeld)
}

func (s *relationLeadershipServiceSuite) TestSetRelationApplicationAndUnitSettingsUnitNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)
	settings := map[string]string{
		"key": "value",
	}

	// Act:
	err := s.leadershipService.SetRelationApplicationAndUnitSettings(c.Context(), "", relationUUID, settings, nil)

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
	ctrl := s.baseServiceSuite.setupMocks(c)

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
