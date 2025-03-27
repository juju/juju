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

	s.relationService.EXPECT().AllRelations(gomock.Any()).Return([]corerelation.UUID{expectedStatus.UUID}, nil)
	s.relationService.EXPECT().GetRelationID(gomock.Any(), expectedStatus.UUID).Return(expectedStatus.ID, nil)
	s.relationService.EXPECT().GetRelationEndpoints(gomock.Any(), expectedStatus.UUID).Return(expectedStatus.Endpoints, nil)
	s.relationService.EXPECT().GetRelationStatus(gomock.Any(), expectedStatus.UUID).Return(expectedStatus.Status, nil)

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

	// Arrange: Several relation, one ok, and three with various defect
	okUUID := corerelation.UUID("ok-uuid")
	noIDUUID := corerelation.UUID("no-id-uuid")
	noEndpointsUUID := corerelation.UUID("no-endpoints-uuid")
	noStatusUUID := corerelation.UUID("no-status-uuid")

	errNoID := errors.New("no id")
	errNoEndpoints := errors.New("no endpoints")
	errNoStatus := errors.New("no status")

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
		42: expectedStatus,
	}

	uuids := []corerelation.UUID{
		noIDUUID, noEndpointsUUID, noStatusUUID, okUUID,
	}

	// Arrange: Valid calls
	s.relationService.EXPECT().AllRelations(gomock.Any()).Return(uuids, nil)
	s.relationService.EXPECT().GetRelationID(gomock.Any(), expectedStatus.UUID).Return(expectedStatus.ID, nil)
	s.relationService.EXPECT().GetRelationEndpoints(gomock.Any(), expectedStatus.UUID).Return(expectedStatus.Endpoints, nil)
	s.relationService.EXPECT().GetRelationStatus(gomock.Any(), expectedStatus.UUID).Return(expectedStatus.Status, nil)

	// Arrange: Error calls
	s.relationService.EXPECT().GetRelationID(gomock.Any(), noIDUUID).Return(0, errNoID)
	s.relationService.EXPECT().GetRelationEndpoints(gomock.Any(), noEndpointsUUID).Return(nil, errNoEndpoints)
	s.relationService.EXPECT().GetRelationStatus(gomock.Any(), noStatusUUID).Return(status.StatusInfo{}, errNoStatus)

	// Arrange: Passing call (to avoid test breakage if the order change between
	// get ID, status and endpoints)
	s.relationService.EXPECT().GetRelationID(gomock.Any(), gomock.Any()).Return(42, nil).AnyTimes()
	s.relationService.EXPECT().GetRelationEndpoints(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.relationService.EXPECT().GetRelationStatus(gomock.Any(), gomock.Any()).Return(status.StatusInfo{}, nil).AnyTimes()

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Check(out, gc.DeepEquals, expectedOut)
	c.Check(outByID, gc.DeepEquals, expectedOutById)
	c.Check(c.GetTestLog(), jc.Contains, `failed to get relation id for "no-id-uuid": no id`)
	c.Check(c.GetTestLog(), jc.Contains, `failed to get relation endpoints for "no-endpoints-uuid": no endpoints`)
	c.Check(c.GetTestLog(), jc.Contains, `failed to get relation status for "no-status-uuid": no status`)
}

// TestFetchRelationNoRelation ensures that fetchRelations correctly handles
// scenarios where no relations are present.
func (s *relationStatusSuite) TestFetchRelationNoRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: No relation
	uuids := []corerelation.UUID{}
	s.relationService.EXPECT().AllRelations(gomock.Any()).Return(uuids, nil)

	// Act: fetch relation
	out, outByID, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Check(out, gc.DeepEquals, map[string][]relationStatus{})
	c.Check(outByID, gc.DeepEquals, map[int]relationStatus{})
}

// TestFetchRelationAllWithError checks the behavior of fetchRelations when
// AllRelations returns an error, ensuring proper error handling.
func (s *relationStatusSuite) TestFetchRelationAllWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: GetAll fails
	expectedError := errors.New("oh no !")

	// Valid calls
	s.relationService.EXPECT().AllRelations(gomock.Any()).Return(nil, expectedError)

	// Act: fetch relation
	_, _, err := fetchRelations(context.Background(), s.relationService)

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}
