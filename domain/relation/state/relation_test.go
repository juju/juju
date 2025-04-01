// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/charm/testing"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type relationSuite struct {
	schematesting.ModelSuite

	state *State

	fakeCharmUUID1                string
	fakeCharmUUID2                string
	fakeApplicationUUID1          string
	fakeApplicationUUID2          string
	fakeApplicationName1          string
	fakeApplicationName2          string
	fakeCharmRelationProvidesUUID string
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.fakeCharmUUID1 = testing.GenCharmID(c).String()
	s.fakeCharmUUID2 = testing.GenCharmID(c).String()
	s.fakeApplicationUUID1 = coreapplicationtesting.GenApplicationUUID(c).String()
	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationUUID2 = coreapplicationtesting.GenApplicationUUID(c).String()
	s.fakeApplicationName2 = "fake-application-2"
	s.fakeCharmRelationProvidesUUID = "fake-charm-relation-provides-uuid"

	// Populate DB with one application and charm.
	s.addCharm(c, s.fakeCharmUUID1)
	s.addCharm(c, s.fakeCharmUUID2)
	s.addCharmRelationWithDefaults(c, s.fakeCharmUUID1, s.fakeCharmRelationProvidesUUID)
	s.addApplication(c, s.fakeCharmUUID1, s.fakeApplicationUUID1, s.fakeApplicationName1)
	s.addApplication(c, s.fakeCharmUUID2, s.fakeApplicationUUID2, s.fakeApplicationName2)
}

func (s *relationSuite) TestGetRelationID(c *gc.C) {
	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationID := 1
	s.addRelationWithID(c, relationUUID.String(), relationID)

	// Act.
	id, err := s.state.GetRelationID(context.Background(), relationUUID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, relationID)
}

func (s *relationSuite) TestGetRelationIDNotFound(c *gc.C) {
	// Act.
	_, err := s.state.GetRelationID(context.Background(), "fake-relation-uuid")

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationUUIDByID(c *gc.C) {
	// Arrange.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationID := 1
	s.addRelationWithID(c, relationUUID.String(), relationID)

	// Act.
	uuid, err := s.state.GetRelationUUIDByID(context.Background(), relationID)

	// Assert.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, relationUUID)
}

func (s *relationSuite) TestGetRelationUUIDByIDNotFound(c *gc.C) {
	// Act.
	_, err := s.state.GetRelationUUIDByID(context.Background(), 1)

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

// TestGetRelationEndpointUUID validates that the correct relation endpoint UUID
// is retrieved for given application and relation ids.
func (s *relationSuite) TestGetRelationEndpointUUID(c *gc.C) {
	// Arrange: create relation endpoint.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()
	relationEndpointUUID := "fake-relation-endpoint-uuid"
	applicationEndpointUUID := "fake-application-endpoint-uuid"
	s.addRelation(c, relationUUID)
	s.addApplicationEndpoint(c, applicationEndpointUUID, s.fakeApplicationUUID1,
		s.fakeCharmRelationProvidesUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, applicationEndpointUUID)

	// Act: get the relation endpoint UUID.
	uuid, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: coreapplication.ID(s.fakeApplicationUUID1),
		RelationUUID:  corerelation.UUID(relationUUID),
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", errors.ErrorStack(err)))

	// Assert: check the right relation has been fetched.
	c.Check(uuid, gc.Equals, corerelation.EndpointUUID(relationEndpointUUID),
		gc.Commentf("(Assert) wrong relation endpoint uuid"))
}

// TestGetRelationEndpointUUIDRelationNotFound verifies that attempting to retrieve
// a relation endpoint UUID for a nonexistent relation returns RelationNotFound.
func (s *relationSuite) TestGetRelationEndpointUUIDRelationNotFound(c *gc.C) {
	// Arrange: nothing to do, no relations.

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: coreapplication.ID(s.fakeApplicationUUID1),
		RelationUUID:  "not-found-relation-uuid",
	})

	// Assert: check that RelationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.RelationNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

// TestGetRelationEndpointUUIDApplicationNotFound verifies that attempting to
// fetch a relation endpoint UUID with a non-existent application ID returns
// the ApplicationNotFound error.
func (s *relationSuite) TestGetRelationEndpointUUIDApplicationNotFound(c *gc.C) {
	// Arrange: nothing to do, will fail on application fetch anyway.

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: "not-found-application-uuid ",
		RelationUUID:  "not-used-uuid",
	})

	// Assert: check that ApplicationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.ApplicationNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

// TestGetRelationEndpointUUIDRelationEndPointNotFound verifies that attempting
// to fetch a relation endpoint UUID for an existing relation without a
// corresponding endpoint returns the RelationEndpointNotFound error.
func (s *relationSuite) TestGetRelationEndpointUUIDRelationEndPointNotFound(c *gc.C) {
	// Arrange: add a relation, but no relation endpoint between apps and relation.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()
	applicationEndpointUUID := "fake-application-endpoint-uuid"
	s.addRelation(c, relationUUID)
	s.addApplicationEndpoint(c, applicationEndpointUUID, s.fakeApplicationUUID1, s.fakeCharmRelationProvidesUUID)

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: coreapplication.ID(s.fakeApplicationUUID1),
		RelationUUID:  corerelation.UUID(relationUUID),
	})

	// Assert: check that ApplicationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.RelationEndpointNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

func (s *relationSuite) TestGetRelationEndpoints(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Act: Get relation endpoints.
	endpoints, err := s.state.GetRelationEndpoints(context.Background(), corerelation.UUID(relationUUID))

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, gc.HasLen, 2)
	c.Check(endpoints[0], gc.DeepEquals, endpoint1)
	c.Check(endpoints[1], gc.DeepEquals, endpoint2)
}

func (s *relationSuite) TestGetRelationEndpointsPeer(c *gc.C) {
	// Arrange: Add a single endpoint and relation over it.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name",
			Role:      internalcharm.RolePeer,
			Interface: "self",
			Optional:  true,
			Limit:     1,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)

	// Act: Get relation endpoints.
	endpoints, err := s.state.GetRelationEndpoints(context.Background(), corerelation.UUID(relationUUID))

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, gc.HasLen, 1)
	c.Check(endpoints[0], gc.DeepEquals, endpoint1)
}

// TestGetRelationEndpointsTooManyEndpoints checks that GetRelationEndpoints
// errors when it finds more than 2 endpoints in the database. This should never
// happen and indicates that the database has become corrupted.
func (s *relationSuite) TestGetRelationEndpointsTooManyEndpoints(c *gc.C) {
	// Arrange: Add three endpoints and a relation on them (shouldn't be
	// possible outside of tests!).
	relationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	charmRelationUUID3 := "fake-charm-relation-uuid-3"
	applicationEndpointUUID3 := "fake-application-endpoint-uuid-3"
	relationEndpointUUID3 := "fake-relation-endpoint-uuid-3"
	endpoint3 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     11,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID3, endpoint3.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addApplicationEndpoint(c, applicationEndpointUUID3, s.fakeApplicationUUID2, charmRelationUUID3)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)
	s.addRelationEndpoint(c, relationEndpointUUID3, relationUUID, applicationEndpointUUID3)

	// Act: Get relation endpoints.
	_, err := s.state.GetRelationEndpoints(context.Background(), corerelation.UUID(relationUUID))

	// Assert:
	c.Assert(err, gc.ErrorMatches, "internal error: expected 1 or 2 endpoints in relation, got 3")
}

func (s *relationSuite) TestGetRelationEndpointsRelationNotFound(c *gc.C) {
	// Arrange: Create relationUUID.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()

	// Act: Get relation endpoints.
	_, err := s.state.GetRelationEndpoints(context.Background(), corerelation.UUID(relationUUID))

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiers(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{

		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, expectedRelationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, expectedRelationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, expectedRelationUUID, applicationEndpointUUID2)

	// Act: Get relation UUID from endpoints.
	uuid, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		relation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
		relation.EndpointIdentifier{
			ApplicationName: endpoint2.ApplicationName,
			EndpointName:    endpoint2.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %s", errors.ErrorStack(err)))
	c.Assert(uuid.String(), gc.Equals, expectedRelationUUID)
}

// TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation
// checks that the function returns not found if only one of the endpoints
// exists (i.e. it is a peer relation).
func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation(c *gc.C) {
	// Arrange: Add an endpoint and a peer relation on it.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, expectedRelationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		relation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
		relation.EndpointIdentifier{
			ApplicationName: "fake-application-2",
			EndpointName:    "fake-endpoint-name-2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFound(c *gc.C) {
	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		relation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
		relation.EndpointIdentifier{
			ApplicationName: "fake-application-2",
			EndpointName:    "fake-endpoint-name-2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiers(c *gc.C) {
	// Arrange: Add an endpoint and a peer relation on it.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, expectedRelationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		relation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

// TestGetPeerRelationUUIDByEndpointIdentifiersRelationNotFoundRegularRelation
// checks that the function returns not found if the endpoint is part of a
// regular relation, not a peer relation.
func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiersRelationNotFoundRegularRelation(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, expectedRelationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, expectedRelationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, expectedRelationUUID, applicationEndpointUUID2)

	// Act: Try and get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		relation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiersNotFound(c *gc.C) {
	// Act: Try and get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		relation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationsStatusForUnit(c *gc.C) {
	// Arrange: Add a relation with two endpoints.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  internalcharm.RoleProvider,
			Scope: internalcharm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:  "fake-endpoint-name-2",
			Role:  internalcharm.RoleRequirer,
			Scope: internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Arrange: Add a unit.
	unitUUID := coreunittesting.GenUnitUUID(c).String()
	s.addUnit(c, unitUUID, "unit-name", s.fakeApplicationUUID1)

	// Arrange: Add unit to relation and set relation status.
	relUnitUUID := corerelationtesting.GenRelationUnitUUID(c).String()
	s.addRelationUnit(c, unitUUID, relationUUID, relUnitUUID, true)
	s.addRelationStatus(c, relationUUID, corestatus.Suspended)

	expectedResults := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
		InScope:   true,
		Suspended: true,
	}}

	// Act: Get relation status for unit.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), coreunit.UUID(unitUUID))

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert): %v",
		errors.ErrorStack(err)))
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].InScope, gc.Equals, expectedResults[0].InScope)
	c.Check(results[0].Suspended, gc.Equals, expectedResults[0].Suspended)
	c.Check(results[0].Endpoints, jc.SameContents, expectedResults[0].Endpoints)
}

// TestGetRelationsStatusForUnit checks that GetRelationStatusesForUnit works
// well with peer relations.
func (s *relationSuite) TestGetRelationsStatusForUnitPeer(c *gc.C) {
	// Arrange: Add two peer relations with one endpoint each.
	relationUUID1 := corerelationtesting.GenRelationUUID(c).String()
	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  internalcharm.RolePeer,
			Scope: internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, relationUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID1, applicationEndpointUUID1)

	relationUUID2 := corerelationtesting.GenRelationUUID(c).String()
	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:  "fake-endpoint-name-2",
			Role:  internalcharm.RolePeer,
			Scope: internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID1, charmRelationUUID2)
	s.addRelation(c, relationUUID2)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID2, applicationEndpointUUID2)

	// Arrange: Add a unit.
	unitUUID := coreunittesting.GenUnitUUID(c).String()
	s.addUnit(c, unitUUID, "unit-name", s.fakeApplicationUUID1)

	// Arrange: Add unit to both the relation and set their status.
	relUnitUUID1 := corerelationtesting.GenRelationUnitUUID(c).String()
	s.addRelationUnit(c, unitUUID, relationUUID1, relUnitUUID1, true)
	s.addRelationStatus(c, relationUUID1, corestatus.Suspended)
	relUnitUUID2 := corerelationtesting.GenRelationUnitUUID(c).String()
	s.addRelationUnit(c, unitUUID, relationUUID2, relUnitUUID2, false)
	s.addRelationStatus(c, relationUUID2, corestatus.Joined)

	expectedResults := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint1},
		InScope:   true,
		Suspended: true,
	}, {
		Endpoints: []relation.Endpoint{endpoint2},
		InScope:   false,
		Suspended: false,
	}}

	// Act: Get relation status for unit.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), coreunit.UUID(unitUUID))

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert): %v",
		errors.ErrorStack(err)))
	c.Assert(results, jc.SameContents, expectedResults)
}

// TestGetRelationStatusesForUnitEmptyResult checks that an empty slice is
// returned when a unit is in no relations.
func (s *relationSuite) TestGetRelationsStatusForUnitEmptyResult(c *gc.C) {
	// Act: Get relation endpoints.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), "fake-unit-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("%v", errors.ErrorStack(err)))
	c.Check(results, gc.HasLen, 0)
}

func (s *relationSuite) TestGetRelationDetails(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c).String()
	relationID := 7

	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      internalcharm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelationWithLifeAndID(c, relationUUID, corelife.Dying, relationID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	expectedDetails := relation.RelationDetailsResult{
		Life:      corelife.Dying,
		UUID:      corerelation.UUID(relationUUID),
		ID:        relationID,
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
	}

	// Act: Get relation details.
	details, err := s.state.GetRelationDetails(context.Background(), relationID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Life, gc.Equals, expectedDetails.Life)
	c.Assert(details.UUID, gc.Equals, expectedDetails.UUID)
	c.Assert(details.ID, gc.Equals, expectedDetails.ID)
	c.Assert(details.Endpoints, jc.SameContents, expectedDetails.Endpoints)
}

func (s *relationSuite) TestGetRelationDetailsNotFound(c *gc.C) {
	// Act: Get relation details.
	_, err := s.state.GetRelationDetails(context.Background(), 7)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

// addApplication adds a new application to the database with the specified UUID and name.
func (s *relationSuite) addApplication(c *gc.C, charmUUID, appUUID, appName string) {
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID, network.AlphaSpaceId)
}

// addUnit adds a new unit to the specified application in the database with
// the given UUID and name.
func (s *relationSuite) addUnit(c *gc.C, unitUUID, unitName, appUUID string) {
	fakeNetNodeUUID := "fake-net-node-uuid"
	s.query(c, `
INSERT INTO net_node (uuid) 
VALUES (?)
`, fakeNetNodeUUID)

	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, risk, os_id, architecture_id)
VALUES (?,?,?,?,?,?,?,?)
`, unitUUID, unitName, 0 /* alive */, appUUID, fakeNetNodeUUID, "stable", "0", "0")
}

// addApplicationEndpoint inserts a new application endpoint into the database with the specified UUIDs and relation data.
func (s *relationSuite) addApplicationEndpoint(c *gc.C, applicationEndpointUUID string, applicationUUID, charmRelationUUID string) {
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
}

// addCharm inserts a new charm into the database with the given UUID.
func (s *relationSuite) addCharm(c *gc.C, charmUUID string) {
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
}

// addCharmRelationWithDefaults inserts a new charm relation into the database with the given UUID and predefined attributes.
func (s *relationSuite) addCharmRelationWithDefaults(c *gc.C, charmUUID, charmRelationUUID string) {
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, name) 
VALUES (?, ?, 0, 'fake-provides')
`, charmRelationUUID, charmUUID)
}

// addCharmRelation inserts a new charm relation into the database with the given UUID and attributes.
func (s *relationSuite) addCharmRelation(c *gc.C, charmUUID, charmRelationUUID string, r internalcharm.Relation) {
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, 0, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
}

// encodeRoleID returns the ID used in the database for the given charm role. This
// reflects the contents of the charm_relation_role table.
func (s *relationSuite) encodeRoleID(role internalcharm.RelationRole) int {
	return map[internalcharm.RelationRole]int{
		internalcharm.RoleProvider: 0,
		internalcharm.RoleRequirer: 1,
		internalcharm.RolePeer:     2,
	}[role]
}

// encodeStatusID returns the ID used in the database for the given relation
// status. This reflects the contents of the relation_status_type table.
func (s *relationSuite) encodeStatusID(status corestatus.Status) int {
	return map[corestatus.Status]int{
		corestatus.Joining:    0,
		corestatus.Joined:     1,
		corestatus.Broken:     2,
		corestatus.Suspending: 3,
		corestatus.Suspended:  4,
	}[status]
}

// encodeScopeID returns the ID used in the database for the given charm scope. This
// reflects the contents of the charm_relation_scope table.
func (s *relationSuite) encodeScopeID(role internalcharm.RelationScope) int {
	return map[internalcharm.RelationScope]int{
		internalcharm.ScopeGlobal:    0,
		internalcharm.ScopeContainer: 1,
	}[role]
}

// addRelation inserts a new relation into the database with the given UUID and default relation and life IDs.
func (s *relationSuite) addRelation(c *gc.C, relationUUID string) {
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, 1)
}

// addRelationWithID inserts a new relation into the database with the given
// UUID, ID, and default life ID.
func (s *relationSuite) addRelationWithID(c *gc.C, relationUUID string, relationID int) {
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, relationID)
}

// addRelationWithLifeAndID inserts a new relation into the database with the
// given details.
func (s *relationSuite) addRelationWithLifeAndID(c *gc.C, relationUUID string, life corelife.Value, relationID int) {
	s.query(c, `
INSERT INTO relation (uuid, relation_id, life_id)
SELECT ?,  ?, id
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
}

// addRelationEndpoint inserts a relation endpoint into the database using the provided UUIDs for relation and endpoint.
func (s *relationSuite) addRelationEndpoint(c *gc.C, relationEndpointUUID string, relationUUID string, applicationEndpointUUID string) {
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
}

// addRelationUnit inserts a relation unit into the database using the provided UUIDs for relation and unit.
func (s *relationSuite) addRelationUnit(c *gc.C, unitUUID, relationUUID, relationUnitUUID string, inScope bool) {
	s.query(c, `
INSERT INTO relation_unit (uuid, relation_uuid, unit_uuid, in_scope)
VALUES (?,?,?,?)
`, relationUnitUUID, relationUUID, unitUUID, inScope)
}

// addRelationStatus inserts a relation status into the relation_status table.
func (s *relationSuite) addRelationStatus(c *gc.C, relationUUID string, status corestatus.Status) {
	s.query(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?,?,?)
`, relationUUID, s.encodeStatusID(status), time.Now())
}

// query executes a given SQL query with optional arguments within a transactional context using the test database.
func (s *relationSuite) query(c *gc.C, query string, args ...any) {

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}
