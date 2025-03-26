// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalrelation "github.com/juju/juju/internal/relation"
)

type relationSuite struct {
	schematesting.ModelSuite

	state *State

	constants struct {
		fakeApplicationUUID1          string
		fakeApplicationUUID2          string
		fakeApplicationName1          string
		fakeApplicationName2          string
		fakeCharmRelationProvidesUUID string
	}
}

var _ = gc.Suite(&relationSuite{})

const (
	fakeCharmUUID1 = "fake-charm-uuid-1"
	fakeCharmUUID2 = "fake-charm-uuid-2"
)

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.constants.fakeApplicationUUID1 = "fake-application-uuid-1"
	s.constants.fakeApplicationName1 = "fake-application-1"
	s.constants.fakeApplicationUUID2 = "fake-application-uuid-2"
	s.constants.fakeApplicationName2 = "fake-application-2"
	s.constants.fakeCharmRelationProvidesUUID = "fake-charm-relation-provides-uuid"

	// Populate DB with one application and charm.
	s.addCharm(c, fakeCharmUUID1)
	s.addCharm(c, fakeCharmUUID2)
	s.addCharmRelationWithDefaults(c, fakeCharmUUID1, s.constants.fakeCharmRelationProvidesUUID)
	s.addApplication(c, fakeCharmUUID1, s.constants.fakeApplicationUUID1, s.constants.fakeApplicationName1)
	s.addApplication(c, fakeCharmUUID2, s.constants.fakeApplicationUUID2, s.constants.fakeApplicationName2)
}

func (s *relationSuite) TestGetRelationID(c *gc.C) {
	// Arrange.
	relationUUID := corerelation.UUID("fake-relation-uuid")
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
	_, err := s.state.GetRelationID(context.Background(), corerelation.UUID("fake-relation-uuid"))

	// Assert.
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationUUIDByID(c *gc.C) {
	// Arrange.
	relationUUID := corerelation.UUID("fake-relation-uuid")
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
	relationUUID := "fake-relation-uuid"
	relationEndpointUUID := "fake-relation-endpoint-uuid"
	applicationEndpointUUID := "fake-application-endpoint-uuid"
	s.addRelation(c, relationUUID)
	s.addApplicationEndpoint(c, applicationEndpointUUID, s.constants.fakeApplicationUUID1,
		s.constants.fakeCharmRelationProvidesUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, applicationEndpointUUID)

	// Act: get the relation endpoint UUID.
	uuid, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: coreapplication.ID(s.constants.fakeApplicationUUID1),
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
		ApplicationID: coreapplication.ID(s.constants.fakeApplicationUUID1),
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
	relationUUID := "fake-relation-uuid"
	applicationEndpointUUID := "fake-application-endpoint-uuid"
	s.addRelation(c, relationUUID)
	s.addApplicationEndpoint(c, applicationEndpointUUID, s.constants.fakeApplicationUUID1, s.constants.fakeCharmRelationProvidesUUID)

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: coreapplication.ID(s.constants.fakeApplicationUUID1),
		RelationUUID:  corerelation.UUID(relationUUID),
	})

	// Assert: check that ApplicationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.RelationEndpointNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

func (s *relationSuite) TestGetRelationEndpoints(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationUUID := "fake-relation-uuid"

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := internalrelation.Endpoint{
		ApplicationName: s.constants.fakeApplicationName1,
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
	endpoint2 := internalrelation.Endpoint{
		ApplicationName: s.constants.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.constants.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.constants.fakeApplicationUUID2, charmRelationUUID2)
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
	relationUUID := "fake-relation-uuid"

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := internalrelation.Endpoint{
		ApplicationName: s.constants.fakeApplicationName1,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name",
			Role:      internalcharm.RolePeer,
			Interface: "self",
			Optional:  true,
			Limit:     1,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.constants.fakeApplicationUUID1, charmRelationUUID1)
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
	relationUUID := "fake-relation-uuid"

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := internalrelation.Endpoint{
		ApplicationName: s.constants.fakeApplicationName1,
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
	endpoint2 := internalrelation.Endpoint{
		ApplicationName: s.constants.fakeApplicationName2,
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
	endpoint3 := internalrelation.Endpoint{
		ApplicationName: s.constants.fakeApplicationName2,
		Relation: internalcharm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      internalcharm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     11,
			Scope:     internalcharm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addCharmRelation(c, fakeCharmUUID2, charmRelationUUID3, endpoint3.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.constants.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.constants.fakeApplicationUUID2, charmRelationUUID2)
	s.addApplicationEndpoint(c, applicationEndpointUUID3, s.constants.fakeApplicationUUID2, charmRelationUUID3)
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
	relationUUID := "fake-relation-uuid"

	// Act: Get relation endpoints.
	_, err := s.state.GetRelationEndpoints(context.Background(), corerelation.UUID(relationUUID))

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

// addApplicationEndpoint inserts a new application endpoint into the database with the specified UUIDs and relation data.
func (s *relationSuite) addApplicationEndpoint(c *gc.C, applicationEndpointUUID string, applicationUUID, charmRelationUUID string) {
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?,?,?,0)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID)
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

// addRelationEndpoint inserts a relation endpoint into the database using the provided UUIDs for relation and endpoint.
func (s *relationSuite) addRelationEndpoint(c *gc.C, relationEndpointUUID string, relationUUID string, applicationEndpointUUID string) {
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
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
