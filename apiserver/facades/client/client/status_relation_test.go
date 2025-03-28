// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationStatusSuite struct {
	testing.IsolationSuite
	relationService *MockRelationService
}

var _ = gc.Suite(&relationStatusSuite{})

func (s *relationStatusSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	logger = loggertesting.WrapCheckLog(c)
	s.relationService = NewMockRelationService(ctrl)
	return ctrl
}

// TestFetchRelation verifies the fetchRelations function correctly retrieves and organizes relation details.
func (s *relationStatusSuite) TestFetchRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: create a relation linked to two application
	expectedStatus := relationStatus{
		UUID: "relation-uuid",
		ID:   1,
		Endpoints: []domainrelation.Endpoint{
			{
				ApplicationName: "source",
				Relation: charm.Relation{
					Name: "provider",
				},
			},
			{
				ApplicationName: "sink",
				Relation: charm.Relation{
					Name: "consumer",
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
		UUID:      expectedStatus.UUID,
		ID:        expectedStatus.ID,
		Endpoints: expectedStatus.Endpoints,
	}}, nil)
	s.relationService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(map[corerelation.UUID]status.StatusInfo{
		expectedStatus.UUID: expectedStatus.Status,
	}, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Check(out, gc.DeepEquals, expectedOut)
	c.Check(outByID, gc.DeepEquals, expectedOutById)
}

// TestFetchRelationWithError validates the behavior of fetchRelations when
// handling various error scenarios during processing.
// Since it is status report, not fetching information from service should
// generate a log instead of prevent to get the status.
func (s *relationStatusSuite) TestFetchRelationWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	okUUID := corerelationtesting.GenRelationUUID(c)

	// Arrange: create a relation linked to two application
	expectedStatus := relationStatus{
		UUID: okUUID,
		ID:   42,
		Endpoints: []domainrelation.Endpoint{
			{
				ApplicationName: "source",
				Relation: charm.Relation{
					Name: "provider",
				},
			},
			{
				ApplicationName: "sink",
				Relation: charm.Relation{
					Name: "consumer",
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
		1: expectedStatus,
	}

	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return([]domainrelation.RelationDetails{{
		UUID:      expectedStatus.UUID,
		ID:        expectedStatus.ID,
		Endpoints: expectedStatus.Endpoints,
		Key:       "key-in-log",
	}}, nil)
	s.relationService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(map[corerelation.UUID]status.StatusInfo{
		// no status
	}, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Check(out, gc.DeepEquals, expectedOut)
	c.Check(outByID, gc.DeepEquals, expectedOutById)
	c.Check(c.GetTestLog(), jc.Contains, `"no status for relation 1 "key-in-log"`)
}

// TestFetchRelationNoRelation ensures that fetchRelations correctly handles
// scenarios where no relations are present.
func (s *relationStatusSuite) TestFetchRelationNoRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: No relation
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Check(out, gc.IsNil, map[string][]relationStatus{})
	c.Check(outByID, gc.DeepEquals, map[int]relationStatus{})
}

// TestFetchRelationAllWithGetRelationError checks the behavior of fetchRelations when
// GetAllRelationDetails returns an error, ensuring proper error handling.
func (s *relationStatusSuite) TestFetchRelationAllWithGetRelationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: GetAll fails
	expectedError := errors.New("oh no !")

	// Valid calls
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, expectedError)

	// Act: fetch relation
	_, _, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}

// GetAllRelationStatuses checks the behavior of fetchRelations when
// AllRelations returns an error, ensuring proper error handling.
func (s *relationStatusSuite) TestFetchRelationAllWithGetStatusesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: GetAll fails
	expectedError := errors.New("oh no !")

	// Valid calls
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act: fetch relation
	_, _, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}
