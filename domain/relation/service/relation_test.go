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
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationServiceSuite struct {
	jujutesting.IsolationSuite

	state *MockState

	service *Service
}

var _ = gc.Suite(&relationServiceSuite{})

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
	relationUUID, err := corerelation.NewUUID()
	c.Assert(err, gc.IsNil, gc.Commentf("(Arrange) can't generate relationUUID: %v", err))
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

func (s *relationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}
