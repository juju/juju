// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"errors"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type relationStatusSuite struct {
	testhelpers.IsolationSuite
	relationService *MockRelationService
	statusService   *MockStatusService
}

var _ = tc.Suite(&relationStatusSuite{})

func (s *relationStatusSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	logger = loggertesting.WrapCheckLog(c)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	return ctrl
}

// TestFetchRelation verifies the fetchRelations function correctly retrieves and organizes relation details.
func (s *relationStatusSuite) TestFetchRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: create a relation linked to two application
	relUUID := corerelationtesting.GenRelationUUID(c)
	expectedStatus := relationStatus{
		ID:  1,
		Key: corerelationtesting.GenNewKey(c, "sink:consumer source:provider"),
		Endpoints: []domainrelation.Endpoint{
			{
				ApplicationName: "source",
				Relation: charm.Relation{
					Name: "provider",
					Role: charm.RoleProvider,
				},
			},
			{
				ApplicationName: "sink",
				Relation: charm.Relation{
					Name: "consumer",
					Role: charm.RoleRequirer,
				},
			},
		},
		Status: status.StatusInfo{
			Status:  "joined",
			Message: "Hey man !",
			Data: map[string]interface{}{
				"foo": "bar",
			},
		},
	}
	expectedOut := map[string][]relationStatus{
		"source": {expectedStatus},
		"sink":   {expectedStatus},
	}
	expectedOutById := map[int]relationStatus{
		1: expectedStatus,
	}

	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return([]domainrelation.RelationDetailsResult{{
		UUID:      relUUID,
		ID:        expectedStatus.ID,
		Endpoints: expectedStatus.Endpoints,
	}}, nil)
	s.statusService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(map[corerelation.UUID]status.StatusInfo{
		relUUID: expectedStatus.Status,
	}, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService, s.statusService)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(out, tc.DeepEquals, expectedOut)
	c.Check(outByID, tc.DeepEquals, expectedOutById)
}

// TestFetchRelationWithError validates the behavior of fetchRelations when
// handling various error scenarios during processing.
// Since it is status report, not fetching information from service should
// generate a log instead of prevent to get the status.
func (s *relationStatusSuite) TestFetchRelationWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	okUUID := corerelationtesting.GenRelationUUID(c)

	// Arrange: create a relation linked to two application
	expectedStatus := relationStatus{
		ID:  42,
		Key: corerelationtesting.GenNewKey(c, "sink:consumer source:provider"),
		Endpoints: []domainrelation.Endpoint{
			{
				ApplicationName: "source",
				Relation: charm.Relation{
					Name: "provider",
					Role: charm.RoleProvider,
				},
			},
			{
				ApplicationName: "sink",
				Relation: charm.Relation{
					Name: "consumer",
					Role: charm.RoleRequirer,
				},
			},
		},
		Status: status.StatusInfo{},
	}
	expectedOut := map[string][]relationStatus{
		"source": {expectedStatus},
		"sink":   {expectedStatus},
	}
	expectedOutById := map[int]relationStatus{
		42: expectedStatus,
	}

	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return([]domainrelation.RelationDetailsResult{{
		UUID:      okUUID,
		ID:        expectedStatus.ID,
		Endpoints: expectedStatus.Endpoints,
	}}, nil)
	s.statusService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(map[corerelation.UUID]status.StatusInfo{
		// no status
	}, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService, s.statusService)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(out, tc.DeepEquals, expectedOut)
	c.Check(outByID, tc.DeepEquals, expectedOutById)
	c.Check(c.GetTestLog(), tc.Contains, `WARNING: no status for relation 42 "sink:consumer source:provider"`)
}

// TestFetchRelationNoRelation ensures that fetchRelations correctly handles
// scenarios where no relations are present.
func (s *relationStatusSuite) TestFetchRelationNoRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: No relation
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService, s.statusService)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(out, tc.DeepEquals, map[string][]relationStatus{})
	c.Check(outByID, tc.DeepEquals, map[int]relationStatus{})
}

// TestFetchRelationAllWithGetRelationError checks the behavior of fetchRelations when
// GetAllRelationDetails returns an error, ensuring proper error handling.
func (s *relationStatusSuite) TestFetchRelationAllWithGetRelationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: GetAll fails
	expectedError := errors.New("oh no !")

	// Valid calls
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, expectedError)

	// Act: fetch relation
	_, _, err := fetchRelations(context.Background(), s.relationService, s.statusService)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

// TestFetchRelationAllWithGetStatusesError checks the behavior of fetchRelations when
// AllRelations returns an error, ensuring proper error handling.
func (s *relationStatusSuite) TestFetchRelationAllWithGetStatusesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: GetAll fails
	expectedError := errors.New("oh no !")

	// Valid calls
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return([]domainrelation.RelationDetailsResult{
		{}, // doesn't matter
	}, nil)
	s.statusService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act: fetch relation
	_, _, err := fetchRelations(context.Background(), s.relationService, s.statusService)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}
