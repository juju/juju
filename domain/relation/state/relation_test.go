// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
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
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// addRelationSuite is a test suite dedicated to check functionalities
// related to adding relation.
// It extends baseRelationSuite to leverage common setup and utility methods
// for relation-related testing and provides more builder dedicated for this
// specific context.
type addRelationSuite struct {
	baseRelationSuite

	// charmByApp maps application IDs to their associated charm IDs for quick
	// lookup during tests.
	charmByApp map[coreapplication.ID]corecharm.ID
}

var _ = gc.Suite(&addRelationSuite{})

func (s *addRelationSuite) SetUpTest(c *gc.C) {
	s.baseRelationSuite.SetUpTest(c)
	s.charmByApp = make(map[coreapplication.ID]corecharm.ID)
}

func (s *addRelationSuite) TestAddRelation(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	epUUID1 := s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	epUUID2 := s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)
	epUUID3 := s.addApplicationEndpointFromRelation(c, appUUID2, relProvider)
	epUUID4 := s.addApplicationEndpointFromRelation(c, appUUID1, relRequirer)

	// Act
	ep1, ep2, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the first relation: %s",
		errors.ErrorStack(err)))
	ep3, ep4, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "req",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "prov",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the second relation: %s",
		errors.ErrorStack(err)))

	// Assert
	c.Check(ep1, gc.Equals, relation.Endpoint{
		ApplicationName: "application-1",
		Relation:        relProvider,
	})
	c.Check(ep2, gc.Equals, relation.Endpoint{
		ApplicationName: "application-2",
		Relation:        relRequirer,
	})
	c.Check(ep3, gc.Equals, relation.Endpoint{
		ApplicationName: "application-1",
		Relation:        relRequirer,
	})
	c.Check(ep4, gc.Equals, relation.Endpoint{
		ApplicationName: "application-2",
		Relation:        relProvider,
	})
	epUUIDsByRelID := s.fetchAllEndpointUUIDsByRelationIDs(c)
	c.Check(epUUIDsByRelID, gc.HasLen, 2)
	c.Check(epUUIDsByRelID[0], jc.SameContents, []corerelation.EndpointUUID{epUUID1, epUUID2},
		gc.Commentf("full map: %v", epUUIDsByRelID))
	c.Check(epUUIDsByRelID[1], jc.SameContents, []corerelation.EndpointUUID{epUUID3, epUUID4},
		gc.Commentf("full map: %v", epUUIDsByRelID))

	// check all relation have a status
	statuses := s.fetchAllRelationStatusesOrderByRelationIDs(c)
	c.Check(statuses, jc.DeepEquals, []corestatus.Status{corestatus.Joining, corestatus.Joining},
		gc.Commentf("all relations should have the same default status: %q", corestatus.Joining))

}

func (s *addRelationSuite) TestAddRelationErrorInfersEndpoint(c *gc.C) {
	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationEndpointNotFound)
}

func (s *addRelationSuite) TestAddRelationErrorAlreadyExists(c *gc.C) {
	// Arrange
	relProvider := charm.Relation{
		Name:  "prov",
		Role:  charm.RoleProvider,
		Scope: charm.ScopeGlobal,
	}
	relRequirer := charm.Relation{
		Name:  "req",
		Role:  charm.RoleRequirer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	s.addApplicationEndpointFromRelation(c, appUUID1, relProvider)
	s.addApplicationEndpointFromRelation(c, appUUID2, relRequirer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error while inserting the first relation: %s",
		errors.ErrorStack(err)))
	_, _, err = s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "prov",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "req",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.RelationAlreadyExists)
}

func (s *addRelationSuite) TestAddRelationErrorCandidateIsPeer(c *gc.C) {
	// Arrange
	relPeer := charm.Relation{
		Name:  "peer",
		Role:  charm.RolePeer,
		Scope: charm.ScopeGlobal,
	}
	appUUID1 := s.addApplication(c, "application")
	s.addApplicationEndpointFromRelation(c, appUUID1, relPeer)

	// Act
	_, _, err := s.state.AddRelation(context.Background(), relation.CandidateEndpointIdentifier{
		ApplicationName: "application",
		EndpointName:    "peer",
	}, relation.CandidateEndpointIdentifier{
		ApplicationName: "application",
		EndpointName:    "peer",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, relationerrors.CompatibleEndpointsNotFound)
}

func (s *addRelationSuite) TestInferEndpoints(c *gc.C) {
	// Arrange:
	db, err := s.state.DB()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot get the DB: %s", errors.ErrorStack(err)))

	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	appUUID3 := s.addApplication(c, "application-3")
	appSubUUID := s.addSubordinateApplication(c, "application-sub")

	// Create endpoints on applications:
	uuids := map[string]corerelation.EndpointUUID{
		// - Application 1: (all are providers)
		//   - interface: unique / name: whatever
		//   - interface: duplicated / name: pickme
		//   - interface: duplicated / name: filler
		"application-1:whatever": s.addApplicationEndpoint(c, appUUID1, "whatever", charm.RoleProvider, "unique"),
		"application-1:pickme": s.addApplicationEndpoint(c, appUUID1, "pickme", charm.RoleProvider,
			"duplicated"),
		"application-1:filler": s.addApplicationEndpoint(c, appUUID1, "filler", charm.RoleProvider,
			"duplicated"),
		// - Application 2: (all are requirers)
		//   - interface: unique / name: whatever
		//   - interface: duplicated / name: pickme
		//   - interface: duplicated / name: filler
		"application-2:whatever": s.addApplicationEndpoint(c, appUUID2, "whatever", charm.RoleRequirer, "unique"),
		"application-2:pickme": s.addApplicationEndpoint(c, appUUID2, "pickme", charm.RoleRequirer,
			"duplicated"),
		"application-2:filler": s.addApplicationEndpoint(c, appUUID2, "filler", charm.RoleRequirer,
			"duplicated"),
		// - Application 3: (all are requirers)
		//   - interface: unique / name: whatever
		"application-3:whatever": s.addApplicationEndpoint(c, appUUID3, "whatever", charm.RoleRequirer, "unique"),
		// - Application Sub: provider on Container scope
		"application-sub:whatever": s.addApplicationEndpointFromRelation(c, appSubUUID, charm.Relation{
			Name:      "whatever",
			Role:      charm.RoleProvider,
			Interface: "unique",
			Scope:     charm.ScopeContainer,
		}),
	}

	cases := []struct {
		description          string
		input1, input2       string
		expected1, expected2 string
	}{
		{
			description: "fully qualified",
			input1:      "application-1:pickme",
			input2:      "application-2:pickme",
			expected1:   "application-1:pickme",
			expected2:   "application-2:pickme",
		}, {
			description: "first identifier not fully qualified",
			input1:      "application-1",
			input2:      "application-2:whatever",
			expected1:   "application-1:whatever",
			expected2:   "application-2:whatever",
		}, {
			description: "second identifier not fully qualified",
			input1:      "application-1:whatever",
			input2:      "application-2",
			expected1:   "application-1:whatever",
			expected2:   "application-2:whatever",
		}, {
			description: "both identifier not fully qualified",
			input1:      "application-1",
			input2:      "application-3",
			expected1:   "application-1:whatever",
			expected2:   "application-3:whatever",
		}, {
			description: "both identifier not fully qualified, but one is subordinate",
			input1:      "application-sub",
			input2:      "application-3",
			expected1:   "application-sub:whatever",
			expected2:   "application-3:whatever",
		},
	}

	for i, tc := range cases {
		identifier1 := s.newEndpointIdentifier(c, tc.input1)
		identifier2 := s.newEndpointIdentifier(c, tc.input2)

		// Act
		var uuid1, uuid2 corerelation.EndpointUUID
		err := db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			ep1, ep2, err := s.state.inferEndpoints(ctx, tx, identifier1, identifier2)
			uuid1 = ep1.EndpointUUID
			uuid2 = ep2.EndpointUUID
			return err
		})

		// Assert
		c.Logf("test %d of %d: %s", i+1, len(cases), tc.description)
		if c.Check(err, jc.ErrorIsNil, gc.Commentf("(Assert) %s: unexpected error: %s", tc.description,
			errors.ErrorStack(err))) {
			c.Check(uuid1, gc.Equals, uuids[tc.expected1], gc.Commentf("(Assert) %s", tc.description))
			c.Check(uuid2, gc.Equals, uuids[tc.expected2], gc.Commentf("(Assert) %s", tc.description))
		}
	}
}

func (s *addRelationSuite) TestInferEndpointsError(c *gc.C) {
	// Arrange:
	db, err := s.state.DB()
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) cannot get the DB: %s", errors.ErrorStack(err)))

	// Create endpoints on applications:
	appUUID1 := s.addApplication(c, "application-1")
	appUUID2 := s.addApplication(c, "application-2")
	appUUID3 := s.addApplication(c, "application-3")

	// - Application 1: name == role
	//   - interface: test / name: provider
	//   - interface: test / name: requirer
	//   - interface: test / name: provider-container / scope container
	s.addApplicationEndpoint(c, appUUID1, "provider", charm.RoleProvider, "test")
	s.addApplicationEndpoint(c, appUUID1, "requirer", charm.RoleRequirer, "test")
	s.addApplicationEndpointFromRelation(c, appUUID1, charm.Relation{
		Name:      "provider-container",
		Role:      charm.RoleProvider,
		Interface: "test",
		Scope:     charm.ScopeContainer,
	})

	// - Application 2:  name == role
	//   - interface: test / name: provider
	//   - interface: test / name: requirer
	//   - interface: test / name: peer
	//   - interface: other / name: provider
	s.addApplicationEndpoint(c, appUUID2, "provider", charm.RoleProvider, "test")
	s.addApplicationEndpoint(c, appUUID2, "requirer", charm.RoleRequirer, "test")
	s.addApplicationEndpoint(c, appUUID2, "peer", charm.RolePeer, "test")
	s.addApplicationEndpoint(c, appUUID2, "other-provider", charm.RoleProvider, "other")

	// - Application 3: different interface than other app
	//   - interface: other / name: first-requirer
	//   - interface: other / name: second-requirer
	s.addApplicationEndpoint(c, appUUID3, "first-requirer", charm.RoleRequirer, "other")
	s.addApplicationEndpoint(c, appUUID3, "second-requirer", charm.RoleRequirer, "other")

	cases := []struct {
		description    string
		input1, input2 string
		expectedError  error
	}{
		{
			description:   "provider with provider",
			input1:        "application-1:provider",
			input2:        "application-2:provider",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "provider with peer",
			input1:        "application-1:provider",
			input2:        "application-2:peer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "requirer with requirer",
			input1:        "application-1:requirer",
			input2:        "application-2:requirer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "requirer with peer",
			input1:        "application-1:requirer",
			input2:        "application-2:peer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "unknown endpoints application-1",
			input1:        "application-1:oupsy",
			input2:        "application-2:peer",
			expectedError: relationerrors.RelationEndpointNotFound,
		},
		{
			description:   "unknown endpoints application-2",
			input1:        "application-1:provider",
			input2:        "application-2:oupsy",
			expectedError: relationerrors.RelationEndpointNotFound,
		},
		{
			description:   "no matches (no common interface)",
			input1:        "application-1",
			input2:        "application-3",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
		{
			description:   "ambiguous on interface 'other'",
			input1:        "application-2",
			input2:        "application-3",
			expectedError: relationerrors.AmbiguousRelation,
		},
		{
			description:   "possible match, but with one endpoint on container scope",
			input1:        "application-1:provider-container",
			input2:        "application-2:requirer",
			expectedError: relationerrors.CompatibleEndpointsNotFound,
		},
	}

	for i, tc := range cases {
		identifier1 := s.newEndpointIdentifier(c, tc.input1)
		identifier2 := s.newEndpointIdentifier(c, tc.input2)

		// Act
		err := db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			_, _, err = s.state.inferEndpoints(ctx, tx, identifier1, identifier2)
			return err
		})

		// Assert
		c.Logf("test %d of %d: %s", i+1, len(cases), tc.description)
		c.Check(err, jc.ErrorIs, tc.expectedError, gc.Commentf("(Assert) %s", tc.description))
	}
}

// addApplication creates and adds a new application with the specified name and
// returns its unique identifier.
// It creates a specific charm for this application.
func (s *addRelationSuite) addApplication(
	c *gc.C,
	applicationName string,
) coreapplication.ID {
	charmUUID := s.addCharm(c)
	appUUID := s.baseRelationSuite.addApplication(c, charmUUID, applicationName)
	s.charmByApp[appUUID] = charmUUID
	return appUUID
}

// addSubordinateApplication creates and adds a new subordinate application
// with the specified name and returns its unique identifier.
// It creates a specific charm for this application.
func (s *addRelationSuite) addSubordinateApplication(
	c *gc.C,
	applicationName string,
) coreapplication.ID {
	charmUUID := s.addCharm(c)
	s.setCharmSubordinate(c, charmUUID)
	appUUID := s.baseRelationSuite.addApplication(c, charmUUID, applicationName)
	s.charmByApp[appUUID] = charmUUID
	return appUUID
}

// addApplicationEndpoint adds a new application endpoint with the specified
// attributes and returns its unique identifier.
func (s *addRelationSuite) addApplicationEndpoint(
	c *gc.C,
	appUUID coreapplication.ID,
	name string,
	role charm.RelationRole,
	relInterface string) corerelation.EndpointUUID {

	return s.addApplicationEndpointFromRelation(c, appUUID, charm.Relation{
		Name:      name,
		Role:      role,
		Interface: relInterface,
		Scope:     charm.ScopeGlobal,
	})
}

// addApplicationEndpointFromRelation creates and associates a new application
// endpoint based on the provided relation.
func (s *addRelationSuite) addApplicationEndpointFromRelation(c *gc.C,
	appUUID coreapplication.ID,
	relation charm.Relation) corerelation.EndpointUUID {

	// Generate and get required UUIDs
	charmUUID := s.charmByApp[appUUID]
	// todo(gfouillet) introduce proper generation for this uuid
	charmRelationUUID := uuid.MustNewUUID()
	relationEndpointUUID := corerelationtesting.GenEndpointUUID(c)

	// Add relation to charm
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, name, interface, role_id, scope_id)
SELECT ?, ?, 0, ?, ?, crr.id, crs.id
FROM charm_relation_scope crs
JOIN charm_relation_role crr ON crr.name = ?
WHERE crs.name = ?
`, charmRelationUUID.String(), charmUUID.String(), relation.Name, relation.Interface, relation.Role, relation.Scope)

	// application endpoint
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?,?,?,?)
`, relationEndpointUUID.String(), appUUID.String(), charmRelationUUID.String(), network.AlphaSpaceId)

	return relationEndpointUUID
}

type relationSuite struct {
	baseRelationSuite

	fakeCharmUUID1                corecharm.ID
	fakeCharmUUID2                corecharm.ID
	fakeApplicationUUID1          coreapplication.ID
	fakeApplicationUUID2          coreapplication.ID
	fakeApplicationName1          string
	fakeApplicationName2          string
	fakeCharmRelationProvidesUUID string
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.baseRelationSuite.SetUpTest(c)

	s.fakeCharmUUID1 = testing.GenCharmID(c)
	s.fakeCharmUUID2 = testing.GenCharmID(c)
	s.fakeApplicationUUID1 = coreapplicationtesting.GenApplicationUUID(c)
	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationUUID2 = coreapplicationtesting.GenApplicationUUID(c)
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
	s.addRelationWithID(c, relationUUID, relationID)

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
	s.addRelationWithID(c, relationUUID, relationID)

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
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationEndpointUUID := "fake-relation-endpoint-uuid"
	applicationEndpointUUID := "fake-application-endpoint-uuid"
	s.addRelation(c, relationUUID)
	s.addApplicationEndpoint(c, applicationEndpointUUID, s.fakeApplicationUUID1,
		s.fakeCharmRelationProvidesUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, applicationEndpointUUID)

	// Act: get the relation endpoint UUID.
	uuid, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: s.fakeApplicationUUID1,
		RelationUUID:  relationUUID,
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
		ApplicationID: s.fakeApplicationUUID1,
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
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationEndpointUUID := "fake-application-endpoint-uuid"
	s.addRelation(c, relationUUID)
	s.addApplicationEndpoint(c, applicationEndpointUUID, s.fakeApplicationUUID1, s.fakeCharmRelationProvidesUUID)

	// Act: get a relation.
	_, err := s.state.GetRelationEndpointUUID(context.Background(), relation.GetRelationEndpointUUIDArgs{
		ApplicationID: s.fakeApplicationUUID1,
		RelationUUID:  relationUUID,
	})

	// Assert: check that ApplicationNotFound is returned.
	c.Check(err, jc.ErrorIs, relationerrors.RelationEndpointNotFound, gc.Commentf("(Assert) wrong error: %v", errors.ErrorStack(err)))
}

func (s *relationSuite) TestGetRelationEndpoints(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
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
	endpoints, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoints, gc.HasLen, 2)
	c.Check(endpoints[0], gc.DeepEquals, endpoint1)
	c.Check(endpoints[1], gc.DeepEquals, endpoint2)
}

func (s *relationSuite) TestGetRelationEndpointsPeer(c *gc.C) {
	// Arrange: Add a single endpoint and relation over it.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name",
			Role:      charm.RolePeer,
			Interface: "self",
			Optional:  true,
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)

	// Act: Get relation endpoints.
	endpoints, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

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
	relationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID3 := "fake-charm-relation-uuid-3"
	applicationEndpointUUID3 := "fake-application-endpoint-uuid-3"
	relationEndpointUUID3 := "fake-relation-endpoint-uuid-3"
	endpoint3 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     11,
			Scope:     charm.ScopeGlobal,
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
	_, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, gc.ErrorMatches, "internal error: expected 1 or 2 endpoints in relation, got 3")
}

func (s *relationSuite) TestGetRelationEndpointsRelationNotFound(c *gc.C) {
	// Arrange: Create relationUUID.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act: Get relation endpoints.
	_, err := s.state.GetRelationEndpoints(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiers(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{

		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
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
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint2.ApplicationName,
			EndpointName:    endpoint2.Name,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %s", errors.ErrorStack(err)))
	c.Assert(uuid, gc.Equals, expectedRelationUUID)
}

// TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation
// checks that the function returns not found if only one of the endpoints
// exists (i.e. it is a peer relation).
func (s *relationSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation(c *gc.C) {
	// Arrange: Add an endpoint and a peer relation on it.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, expectedRelationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
		corerelation.EndpointIdentifier{
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
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-2",
			EndpointName:    "fake-endpoint-name-2",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetPeerRelationUUIDByEndpointIdentifiers(c *gc.C) {
	// Arrange: Add an endpoint and a peer relation on it.
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, expectedRelationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		context.Background(),
		corerelation.EndpointIdentifier{
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
	expectedRelationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
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
		corerelation.EndpointIdentifier{
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
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetRelationsStatusForUnit(c *gc.C) {
	// Arrange: Add a relation with two endpoints.
	relationUUID := corerelationtesting.GenRelationUUID(c)

	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  charm.RoleProvider,
			Scope: charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-2",
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
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
	unitUUID := coreunittesting.GenUnitUUID(c)
	s.addUnit(c, unitUUID, "unit-name", s.fakeApplicationUUID1)

	// Arrange: Add unit to relation and set relation status.
	relUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.addRelationUnit(c, unitUUID, relationUUID, relUnitUUID, true)
	s.addRelationStatus(c, relationUUID, corestatus.Suspended)

	expectedResults := []relation.RelationUnitStatusResult{{
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
		InScope:   true,
		Suspended: true,
	}}

	// Act: Get relation status for unit.
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), unitUUID)

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
	relationUUID1 := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := "fake-charm-relation-uuid-1"
	applicationEndpointUUID1 := "fake-application-endpoint-uuid-1"
	relationEndpointUUID1 := "fake-relation-endpoint-uuid-1"
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-1",
			Role:  charm.RolePeer,
			Scope: charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, relationUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID1, applicationEndpointUUID1)

	relationUUID2 := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID2 := "fake-charm-relation-uuid-2"
	applicationEndpointUUID2 := "fake-application-endpoint-uuid-2"
	relationEndpointUUID2 := "fake-relation-endpoint-uuid-2"
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:  "fake-endpoint-name-2",
			Role:  charm.RolePeer,
			Scope: charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID1, charmRelationUUID2)
	s.addRelation(c, relationUUID2)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID2, applicationEndpointUUID2)

	// Arrange: Add a unit.
	unitUUID := coreunittesting.GenUnitUUID(c)
	s.addUnit(c, unitUUID, "unit-name", s.fakeApplicationUUID1)

	// Arrange: Add unit to both the relation and set their status.
	relUnitUUID1 := corerelationtesting.GenRelationUnitUUID(c)
	s.addRelationUnit(c, unitUUID, relationUUID1, relUnitUUID1, true)
	s.addRelationStatus(c, relationUUID1, corestatus.Suspended)
	relUnitUUID2 := corerelationtesting.GenRelationUnitUUID(c)
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
	results, err := s.state.GetRelationsStatusForUnit(context.Background(), unitUUID)

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
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationID := 7

	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
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
		UUID:      relationUUID,
		ID:        relationID,
		Endpoints: []relation.Endpoint{endpoint1, endpoint2},
	}

	// Act: Get relation details.
	details, err := s.state.GetRelationDetails(context.Background(), relationUUID)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Life, gc.Equals, expectedDetails.Life)
	c.Assert(details.UUID, gc.Equals, expectedDetails.UUID)
	c.Assert(details.ID, gc.Equals, expectedDetails.ID)
	c.Assert(details.Endpoints, jc.SameContents, expectedDetails.Endpoints)
}

func (s *relationSuite) TestGetRelationDetailsNotFound(c *gc.C) {
	// Act: Get relation details.
	_, err := s.state.GetRelationDetails(context.Background(), "unknown-relation-uuid")

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestGetAllRelationDetails(c *gc.C) {
	// Arrange: Add three endpoints and two relations on them.
	relationUUID1 := corerelationtesting.GenRelationUUID(c)
	relationID1 := 7
	relationUUID2 := corerelationtesting.GenRelationUUID(c)
	relationID2 := 8

	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1bis := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}

	charmRelationUUID3 := uuid.MustNewUUID().String()
	applicationEndpointUUID3 := uuid.MustNewUUID().String()
	relationEndpointUUID3 := uuid.MustNewUUID().String()
	endpoint3 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Optional:  false,
			Limit:     10,
			Scope:     charm.ScopeGlobal,
		},
	}

	// This is a lot of code to build two relation:
	// - application-1:endpoint-1 application-2:endpoint-2
	// - application-1:endpoint-1 application-2:endpoint-3
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID3, endpoint3.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addApplicationEndpoint(c, applicationEndpointUUID3, s.fakeApplicationUUID2, charmRelationUUID3)
	s.addRelationWithLifeAndID(c, relationUUID1, corelife.Dying, relationID1)
	s.addRelationWithLifeAndID(c, relationUUID2, corelife.Alive, relationID2)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID1, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID1, applicationEndpointUUID2)
	s.addRelationEndpoint(c, relationEndpointUUID1bis, relationUUID2, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID3, relationUUID2, applicationEndpointUUID3)

	expectedDetails := map[int]relation.RelationDetailsResult{
		relationID1: {
			Life:      corelife.Dying,
			UUID:      relationUUID1,
			ID:        relationID1,
			Endpoints: []relation.Endpoint{endpoint1, endpoint2},
		},
		relationID2: {
			Life:      corelife.Alive,
			UUID:      relationUUID2,
			ID:        relationID2,
			Endpoints: []relation.Endpoint{endpoint1, endpoint3},
		},
	}

	// Act: Get relation details.
	details, err := s.state.GetAllRelationDetails(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 2)
	detailsByRelationID := make(map[int]relation.RelationDetailsResult)
	for _, detail := range details {
		detailsByRelationID[detail.ID] = detail
	}
	// First relation
	c.Check(detailsByRelationID[relationID1].Life, gc.Equals, expectedDetails[relationID1].Life)
	c.Check(detailsByRelationID[relationID1].UUID, gc.Equals, expectedDetails[relationID1].UUID)
	c.Check(detailsByRelationID[relationID1].ID, gc.Equals, expectedDetails[relationID1].ID)
	c.Check(detailsByRelationID[relationID1].Endpoints, jc.SameContents, expectedDetails[relationID1].Endpoints)
	// Second relation
	c.Check(detailsByRelationID[relationID2].Life, gc.Equals, expectedDetails[relationID2].Life)
	c.Check(detailsByRelationID[relationID2].UUID, gc.Equals, expectedDetails[relationID2].UUID)
	c.Check(detailsByRelationID[relationID2].ID, gc.Equals, expectedDetails[relationID2].ID)
	c.Check(detailsByRelationID[relationID2].Endpoints, jc.SameContents, expectedDetails[relationID2].Endpoints)
}

func (s *relationSuite) TestGetAllRelationDetailsNone(c *gc.C) {
	// Act: Get relation details.
	result, err := s.state.GetAllRelationDetails(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

func (s *relationSuite) TestEnterScope(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.setCharmSubordinate(c, s.fakeCharmUUID1, false)
	s.setCharmSubordinate(c, s.fakeCharmUUID2, false)

	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Arrange: Add unit to application in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID, unitName, s.fakeApplicationUUID1)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	inScope := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(inScope, jc.IsTrue)
}

// TestEnterScopeRowAlreadyExists checks that unit still enters scope when there
// already exists a row in the relation_unit table.
func (s *relationSuite) TestEnterScopeRowAlreadyExists(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.setCharmSubordinate(c, s.fakeCharmUUID1, false)
	s.setCharmSubordinate(c, s.fakeCharmUUID2, false)

	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Arrange: Add unit to application in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID, unitName, s.fakeApplicationUUID1)

	// Arrange: Add relation unit for the unit and relation with in_scope set to
	// false.
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.addRelationUnit(c, unitUUID, relationUUID, relationUnitUUID, false)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: in_scope is true:
	inScope := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(inScope, jc.IsTrue)
}

// TestEnterScopeIdempotent checks that no error is returned if the unit is
// already in scope.
func (s *relationSuite) TestEnterScopeIdempotent(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.setCharmSubordinate(c, s.fakeCharmUUID1, false)
	s.setCharmSubordinate(c, s.fakeCharmUUID2, false)

	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Arrange: Add unit to application in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID, unitName, s.fakeApplicationUUID1)

	// Arrange: Add relation unit for the unit and relation with in_scope set to
	// false.
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.addRelationUnit(c, unitUUID, relationUUID, relationUnitUUID, true)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIsNil, gc.Commentf(errors.ErrorStack(err)))

	// Assert: in_scope is true:
	inScope := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(inScope, jc.IsTrue)
}

// TestEnterScopeSubordinate checks that a subordinate unit can enter scope to
// with its principle application.
func (s *relationSuite) TestEnterScopeSubordinate(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.setCharmSubordinate(c, s.fakeCharmUUID1, true)
	s.setCharmSubordinate(c, s.fakeCharmUUID2, false)

	// Arrange: Add container scoped endpoints on charm 1 and charm 2.
	charmRelationUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)

	// Arrange: Add a unit to application 1 and application 2, and make the unit
	// of application 1 a subordinate to the unit of application 2.
	unitUUID1 := coreunittesting.GenUnitUUID(c)
	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID1, unitName1, s.fakeApplicationUUID1)
	unitUUID2 := coreunittesting.GenUnitUUID(c)
	unitName2 := coreunittesting.GenNewName(c, "app2/0")
	s.addUnit(c, unitUUID2, unitName2, s.fakeApplicationUUID2)
	s.setUnitSubordinate(c, unitUUID1, unitUUID2)

	// Add a relation between application 1 and application 2.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Act: Try and enter scope with the unit 1, which is a subordinate to an
	// application not in the relation.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName1)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)

	// Assert: in_scope is true:
	inScope := s.getRelationUnitInScope(c, relationUUID, unitUUID1)
	c.Check(inScope, jc.IsTrue)
}

// TestEnterScopePotentialRelationUnitNotValidSubordinate checks the right error
// is returned if the unit is a subordinate of an application that is not in the
// relation.
func (s *relationSuite) TestEnterScopePotentialRelationUnitNotValidSubordinate(c *gc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.setCharmSubordinate(c, s.fakeCharmUUID1, true)
	s.setCharmSubordinate(c, s.fakeCharmUUID2, false)

	// Arrange: Add container scoped endpoints on charm 1 and charm 2.
	charmRelationUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)

	// Arrange: Add a unit to application 1 and application 2, and make the unit
	// of application 1 a subordinate to the unit of application 2.
	unitUUID1 := coreunittesting.GenUnitUUID(c)
	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID1, unitName1, s.fakeApplicationUUID1)
	unitUUID2 := coreunittesting.GenUnitUUID(c)
	unitName2 := coreunittesting.GenNewName(c, "app2/0")
	s.addUnit(c, unitUUID2, unitName2, s.fakeApplicationUUID2)
	s.setUnitSubordinate(c, unitUUID1, unitUUID2)

	// Arrange: Add a third application which is an instance of charm 2, so also
	// a principle, and enter application 3 into a relation with the subordinate
	// application (application 1).
	applicationUUID3 := coreapplicationtesting.GenApplicationUUID(c)
	applicationName3 := "application-name-3"
	s.addApplication(c, s.fakeCharmUUID2, applicationUUID3, applicationName3)

	relationUUID := corerelationtesting.GenRelationUUID(c)
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	relationEndpointUUID2 := uuid.MustNewUUID().String()
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, applicationUUID3, charmRelationUUID2)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationEndpointUUID2, relationUUID, applicationEndpointUUID2)

	// Act: Try and enter scope with the unit 1, which is a subordinate to an
	// application not in the relation.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName1)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.PotentialRelationUnitNotValid)
}

// TestEnterScopePotentialRelationUnitNotValid checks that the correct error
// is returned when the unit specified is not a unit of the application in the
// relation.
func (s *relationSuite) TestEnterScopePotentialRelationUnitNotValid(c *gc.C) {
	// Arrange: Add a peer relation on application 1.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	relationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RolePeer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID1, relationUUID, applicationEndpointUUID1)

	// Arrange: Add unit to application not in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app2/0")
	s.addUnit(c, unitUUID, unitName, s.fakeApplicationUUID2)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotInRelation)
}

func (s *relationSuite) TestEnterScopeRelationNotAlive(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelationWithLifeAndID(c, relationUUID, corelife.Dying, 17)

	// Arrange: Add unit to application in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID, unitName, s.fakeApplicationUUID1)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotAlive)
}

func (s *relationSuite) TestEnterScopeUnitNotAlive(c *gc.C) {
	// Arrange: Add two endpoints and a relation on them.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	charmRelationUUID1 := uuid.MustNewUUID().String()
	applicationEndpointUUID1 := uuid.MustNewUUID().String()
	endpoint1 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID2 := uuid.MustNewUUID().String()
	applicationEndpointUUID2 := uuid.MustNewUUID().String()
	endpoint2 := relation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	s.addCharmRelation(c, s.fakeCharmUUID1, charmRelationUUID1, endpoint1.Relation)
	s.addCharmRelation(c, s.fakeCharmUUID2, charmRelationUUID2, endpoint2.Relation)
	s.addApplicationEndpoint(c, applicationEndpointUUID1, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, applicationEndpointUUID2, s.fakeApplicationUUID2, charmRelationUUID2)
	s.addRelation(c, relationUUID)

	// Arrange: Add unit to application in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnitWithLife(c, unitUUID, unitName, s.fakeApplicationUUID1, corelife.Dead)

	// Act: Enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotAlive)
}

func (s *relationSuite) TestEnterScopeRelationNotFound(c *gc.C) {
	// Arrange: Add unit to application in the relation.
	unitUUID := coreunittesting.GenUnitUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitUUID, unitName, s.fakeApplicationUUID1)

	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act: Try and enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, unitName)

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationSuite) TestEnterScopeUnitNotFound(c *gc.C) {
	relationUUID := corerelationtesting.GenRelationUUID(c)

	// Act: Try and enter scope.
	err := s.state.EnterScope(context.Background(), relationUUID, coreunittesting.GenNewName(c, "app1/0"))

	// Assert:
	c.Assert(err, jc.ErrorIs, relationerrors.UnitNotFound)
}

// addApplication adds a new application to the database with the specified UUID and name.
func (s *relationSuite) addApplication(c *gc.C, charmUUID corecharm.ID, appUUID coreapplication.ID, appName string) {
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID, network.AlphaSpaceId)
}

// addUnit adds a new unit to the specified application in the database with
// the given UUID and name.
func (s *relationSuite) addUnit(c *gc.C, unitUUID coreunit.UUID, unitName coreunit.Name, appUUID coreapplication.ID) {
	fakeNetNodeUUID := "fake-net-node-uuid"
	s.query(c, `
INSERT INTO net_node (uuid) 
VALUES (?)
ON CONFLICT DO NOTHING
`, fakeNetNodeUUID)

	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid)
VALUES (?,?,?,?,?)
`, unitUUID, unitName, 0 /* alive */, appUUID, fakeNetNodeUUID)
}

// addUnitWithLife adds a new unit to the specified application in the database with
// the given UUID, name and life.
func (s *relationSuite) addUnitWithLife(c *gc.C, unitUUID coreunit.UUID, unitName coreunit.Name, appUUID coreapplication.ID, life corelife.Value) {
	fakeNetNodeUUID := "fake-net-node-uuid"
	s.query(c, `
INSERT INTO net_node (uuid) 
VALUES (?)
ON CONFLICT DO NOTHING
`, fakeNetNodeUUID)

	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid)
SELECT ?, ?, id, ?, ?
FROM life
WHERE value = ?
`, unitUUID, unitName, appUUID, fakeNetNodeUUID, life)
}

// addApplicationEndpoint inserts a new application endpoint into the database with the specified UUIDs and relation data.
func (s *relationSuite) addApplicationEndpoint(c *gc.C, applicationEndpointUUID string, applicationUUID coreapplication.ID, charmRelationUUID string) {
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
}

// addCharm inserts a new charm into the database with the given UUID.
func (s *relationSuite) addCharm(c *gc.C, charmUUID corecharm.ID) {
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
}

// addCharmRelationWithDefaults inserts a new charm relation into the database with the given UUID and predefined attributes.
func (s *relationSuite) addCharmRelationWithDefaults(c *gc.C, charmUUID corecharm.ID, charmRelationUUID string) {
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, scope_id, role_id, name) 
VALUES (?, ?, 0, 0, 0, 'fake-provides')
`, charmRelationUUID, charmUUID)
}

// addCharmRelation inserts a new charm relation into the database with the given UUID and attributes.
func (s *relationSuite) addCharmRelation(c *gc.C, charmUUID corecharm.ID, charmRelationUUID string, r charm.Relation) {
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, 0, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
}

// encodeRoleID returns the ID used in the database for the given charm role. This
// reflects the contents of the charm_relation_role table.
func (s *relationSuite) encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
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
func (s *relationSuite) encodeScopeID(role charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[role]
}

// addRelation inserts a new relation into the database with the given UUID and default relation and life IDs.
func (s *relationSuite) addRelation(c *gc.C, relationUUID corerelation.UUID) {
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, 1)
}

// addRelationWithID inserts a new relation into the database with the given
// UUID, ID, and default life ID.
func (s *relationSuite) addRelationWithID(c *gc.C, relationUUID corerelation.UUID, relationID int) {
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, relationID)
}

// addRelationWithLifeAndID inserts a new relation into the database with the
// given details.
func (s *relationSuite) addRelationWithLifeAndID(c *gc.C, relationUUID corerelation.UUID, life corelife.Value, relationID int) {
	s.query(c, `
INSERT INTO relation (uuid, relation_id, life_id)
SELECT ?,  ?, id
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
}

// addRelationEndpoint inserts a relation endpoint into the database using the provided UUIDs for relation and endpoint.
func (s *relationSuite) addRelationEndpoint(c *gc.C, relationEndpointUUID string, relationUUID corerelation.UUID, applicationEndpointUUID string) {
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
}

// addRelationUnit inserts a relation unit into the database using the provided UUIDs for relation and unit.
func (s *relationSuite) addRelationUnit(c *gc.C, unitUUID coreunit.UUID, relationUUID corerelation.UUID, relationUnitUUID corerelation.UnitUUID, inScope bool) {
	s.query(c, `
INSERT INTO relation_unit (uuid, relation_uuid, unit_uuid, in_scope)
VALUES (?,?,?,?)
`, relationUnitUUID, relationUUID, unitUUID, inScope)
}

// addRelationStatus inserts a relation status into the relation_status table.
func (s *relationSuite) addRelationStatus(c *gc.C, relationUUID corerelation.UUID, status corestatus.Status) {
	s.query(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?,?,?)
`, relationUUID, s.encodeStatusID(status), time.Now())
}

// fetchAllRelationStatusesOrderByRelationIDs retrieves all relation statuses
// ordered by their relation IDs.
// It executes a database query within a transaction and returns a slice of
// corestatus.Status objects.
func (s *addRelationSuite) fetchAllRelationStatusesOrderByRelationIDs(c *gc.C) []corestatus.Status {
	var statuses []corestatus.Status
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		query := `
SELECT rst.name
FROM relation r 
JOIN relation_status rs ON r.uuid = rs.relation_uuid
JOIN relation_status_type rst ON rs.relation_status_type_id = rst.id
ORDER BY r.relation_id
`
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()
		for rows.Next() {
			var status corestatus.Status
			if err := rows.Scan(&status); err != nil {
				return errors.Capture(err)
			}
			statuses = append(statuses, status)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) fetching inserted relation statuses: %s",
		errors.ErrorStack(err)))
	return statuses
}

// fetchAllEndpointUUIDsByRelationIDs retrieves a mapping of relation IDs to their
// associated endpoint UUIDs from the database.
// It executes a query within a transaction to fetch data from the
// `relation_endpoint` and `relation` tables.  The result is returned as a map
// where the key is the relation ID and the value is a slice of EndpointUUIDs.
func (s *addRelationSuite) fetchAllEndpointUUIDsByRelationIDs(c *gc.C) map[int][]corerelation.EndpointUUID {
	epUUIDsByRelID := make(map[int][]corerelation.EndpointUUID)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		query := `
SELECT re.endpoint_uuid, r.relation_id
FROM relation_endpoint re 
JOIN relation r  ON re.relation_uuid = r.uuid
`
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()
		for rows.Next() {
			var epUUID string
			var relID int
			if err := rows.Scan(&epUUID, &relID); err != nil {
				return errors.Capture(err)
			}
			epUUIDsByRelID[relID] = append(epUUIDsByRelID[relID], corerelation.EndpointUUID(epUUID))
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) fetching inserted relation endpoint: %s", errors.ErrorStack(err)))
	return epUUIDsByRelID
}

// getRelationUnitInScope gets the in_scope column from the relation_unit table.
func (s *relationSuite) getRelationUnitInScope(c *gc.C, relationUUID corerelation.UUID, unitUUID coreunit.UUID) bool {
	var inScope bool
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT in_scope
FROM   relation_unit
WHERE  relation_uuid = ?
AND    unit_uuid = ?
`, relationUUID, unitUUID).Scan(&inScope)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return inScope
}

// setCharmSubordinate updates the charm's metadata to mark it as subordinate,
// or inserts it if not present in the database.
func (s *relationSuite) setCharmSubordinate(c *gc.C, charmUUID corecharm.ID, subordinate bool) {
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate)
VALUES (?,?,true)
ON CONFLICT DO UPDATE SET subordinate = ?
`, charmUUID, charmUUID, subordinate)
}

// setUnitSubordinate sets unit 1 to be a subordinate of unit 2.
func (s *relationSuite) setUnitSubordinate(c *gc.C, unitUUID1, unitUUID2 coreunit.UUID) {
	s.query(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?,?)
`, unitUUID1, unitUUID2)
}
