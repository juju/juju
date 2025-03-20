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
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type relationSuite struct {
	schematesting.ModelSuite

	state *State

	constants struct {
		fakeApplicationUUID1          string
		fakeApplicationName1          string
		fakeCharmRelationProvidesUUID string
	}
}

var _ = gc.Suite(&relationSuite{})

const fakeCharmUUID = "fake-charm-uuid"

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.constants.fakeApplicationUUID1 = "fake-application-1-uuid"
	s.constants.fakeApplicationName1 = "fake-application-1"
	s.constants.fakeCharmRelationProvidesUUID = "fake-charm-relation-provides-uuid"

	// Populate DB with one application and charm.
	s.addCharm(c)
	s.addCharmRelation(c, s.constants.fakeCharmRelationProvidesUUID)
	s.addApplication(c, s.constants.fakeApplicationUUID1, s.constants.fakeApplicationName1)
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

// addApplication adds a new application to the database with the specified UUID and name.
func (s *relationSuite) addApplication(c *gc.C, appUUID, appName string) {
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid) 
VALUES (?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, fakeCharmUUID)
}

// addApplicationEndpoint inserts a new application endpoint into the database with the specified UUIDs and relation data.
func (s *relationSuite) addApplicationEndpoint(c *gc.C, applicationEndpointUUID string, applicationUUID, charmRelationUUID string) {
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?,?,?,0)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID)
}

// addCharm inserts a new charm into the database with a predefined UUID, reference name, and architecture ID.
func (s *relationSuite) addCharm(c *gc.C) {
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, 'app', 0)
`, fakeCharmUUID)
}

// addCharmRelation inserts a new charm relation into the database with the given UUID and predefined attributes.
func (s *relationSuite) addCharmRelation(c *gc.C, charmRelationUUID string) {
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, "key") 
VALUES (?, ?, 0, 'fake-provides')
`, charmRelationUUID, fakeCharmUUID)
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
