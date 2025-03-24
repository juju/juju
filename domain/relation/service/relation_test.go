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
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalrelation "github.com/juju/juju/internal/relation"
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

func (s *relationServiceSuite) TestGetRelationKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	endpoint1 := internalrelation.Endpoint{
		ApplicationName: "app-1",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-1",
		},
	}
	endpoint2 := internalrelation.Endpoint{
		ApplicationName: "app-2",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-2",
		},
	}

	expectedKey := corerelation.Key("app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relationUUID).Return([]internalrelation.Endpoint{
		endpoint1,
		endpoint2,
	}, nil)

	// Act:
	key, err := s.service.GetRelationKey(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key, gc.Equals, expectedKey)
}

func (s *relationServiceSuite) TestGetRelationKeyPeer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	relationUUID := corerelationtesting.GenRelationUUID(c)

	endpoint1 := internalrelation.Endpoint{
		ApplicationName: "app-1",
		Relation: internalcharm.Relation{
			Name: "fake-endpoint-name-1",
		},
	}

	expectedKey := corerelation.Key("app-1:fake-endpoint-name-1")
	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relationUUID).Return([]internalrelation.Endpoint{
		endpoint1,
	}, nil)

	// Act:
	key, err := s.service.GetRelationKey(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key, gc.Equals, expectedKey)
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
	key := corerelation.Key("app-1:fake-endpoint-name-1")

	endpointID := relation.EndpointIdentifier{
		ApplicationName: "app-1",
		EndpointName:    "fake-endpoint-name-1",
	}

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetPeerRelationUUIDByEndpointIdentifiers(
		gomock.Any(), endpointID,
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
	key := corerelation.Key("app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2")
	endpointID1 := relation.EndpointIdentifier{
		ApplicationName: "app-1",
		EndpointName:    "fake-endpoint-name-1",
	}
	endpointID2 := relation.EndpointIdentifier{
		ApplicationName: "app-2",
		EndpointName:    "fake-endpoint-name-2",
	}

	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(
		gomock.Any(), endpointID1, endpointID2,
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
		"app-1:fake-endpoint-name-1 app-2:fake-endpoint-name-2",
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationServiceSuite) TestGetRelationUUIDByKeyRelationKeyNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	key := corerelation.Key("invalid-key")

	// Act:
	_, err := s.service.GetRelationUUIDByKey(context.Background(), key)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationKeyNotValid)
}

func (s *relationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}

type parseKeySuite struct{}

var _ = gc.Suite(&parseKeySuite{})

func (s *parseKeySuite) TestParseRelationKey(c *gc.C) {
	tests := []struct {
		summary             string
		relationKey         string
		endpointIdentifiers []relation.EndpointIdentifier
	}{{
		summary:     "regular relation",
		relationKey: "app1:end1 app2:end2",
		endpointIdentifiers: []relation.EndpointIdentifier{
			{
				ApplicationName: "app1",
				EndpointName:    "end1",
			}, {
				ApplicationName: "app2",
				EndpointName:    "end2",
			},
		},
	}, {
		summary:     "peer relation",
		relationKey: "app1:end1",
		endpointIdentifiers: []relation.EndpointIdentifier{
			{
				ApplicationName: "app1",
				EndpointName:    "end1",
			},
		},
	}, {
		summary:     "regular relation with many spaces",
		relationKey: "app_1:end_1              app_2:end_2",
		endpointIdentifiers: []relation.EndpointIdentifier{
			{
				ApplicationName: "app_1",
				EndpointName:    "end_1",
			}, {
				ApplicationName: "app_2",
				EndpointName:    "end_2",
			},
		},
	}}

	count := len(tests)
	for i, test := range tests {
		result, err := parseRelationKeyEndpoints(corerelation.Key(test.relationKey))
		c.Logf("test %d of %d, key: %s, summary: %s", i+1, count, test.relationKey, test.summary)
		c.Check(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, test.endpointIdentifiers)
	}
}

func (s *parseKeySuite) TestParseRelationKeyError(c *gc.C) {
	tests := []struct {
		summary     string
		relationKey string
		errorRegex  string
	}{{
		summary:     "too many endpoints",
		relationKey: "app1:end1 app2:end2 app3:end3",
		errorRegex:  "expected 1 or 2 endpoints in relation key, found 3.*",
	}, {
		summary:     "zero endpoints",
		relationKey: "",
		errorRegex:  "expected 1 or 2 endpoints in relation key, found 0.*",
	}, {
		summary:     "no space",
		relationKey: "app_1:end_1app2:end2",
		errorRegex:  "expected endpoints of form <application-name>:<endpoint-name>, got.*",
	}}

	count := len(tests)
	for i, test := range tests {
		_, err := parseRelationKeyEndpoints(corerelation.Key(test.relationKey))
		c.Logf("test %d of %d, key: %s, summary: %s", i+1, count, test.relationKey, test.summary)
		c.Check(err, gc.ErrorMatches, test.errorRegex)
	}
}
