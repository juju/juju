// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/deployment/charm"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/uuid"
)

type unitRelationsSuite struct {
	baseSuite

	fakeCharmUUID1                string
	fakeCharmUUID2                string
	fakeApplicationUUID1          string
	fakeApplicationUUID2          string
	fakeApplicationName1          string
	fakeApplicationName2          string
	fakeCharmRelationProvidesUUID string

	// relationCount helps generation of consecutive relation_id
	relationCount int
}

func TestUnitRelationsSuite(t *testing.T) {
	tc.Run(t, &unitRelationsSuite{})
}

func (s *unitRelationsSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationName2 = "fake-application-2"

	// Populate DB with one application and charm.
	s.fakeCharmUUID1 = s.addCharm(c)
	s.fakeCharmUUID2 = s.addCharm(c)
	s.fakeCharmRelationProvidesUUID = s.addCharmRelationWithDefaults(c, s.fakeCharmUUID1)
	s.fakeApplicationUUID1 = s.addApplication(c, s.fakeCharmUUID1, s.fakeApplicationName1, network.AlphaSpaceId.String())
	s.fakeApplicationUUID2 = s.addApplication(c, s.fakeCharmUUID2, s.fakeApplicationName2, network.AlphaSpaceId.String())

	c.Cleanup(func() {
		s.fakeCharmUUID1 = ""
		s.fakeCharmUUID2 = ""
		s.fakeApplicationName1 = ""
		s.fakeApplicationName2 = ""
		s.fakeApplicationUUID1 = ""
		s.fakeApplicationUUID2 = ""
		s.fakeCharmRelationProvidesUUID = ""
		s.relationCount = 0
	})
}

func (s *unitRelationsSuite) TestGetRegularRelationUUIDByEndpointIdentifiers(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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

	endpoint2 := domainrelation.Endpoint{
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
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID2)

	// Act: Get relation UUID from endpoints.
	uuid, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, expectedRelationUUID)
}

// TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation
// checks that the function returns not found if only one of the endpoints
// exists (i.e. it is a peer relation).
func (s *unitRelationsSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFoundPeerRelation(c *tc.C) {
	// Arrange: Add an endpoint and a peer relation on it.
	endpoint1 := domainrelation.Endpoint{
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

	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *unitRelationsSuite) TestGetRegularRelationUUIDByEndpointIdentifiersRelationNotFound(c *tc.C) {
	// Act: Try and get relation UUID from endpoints.
	_, err := s.state.GetRegularRelationUUIDByEndpointIdentifiers(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *unitRelationsSuite) TestGetPeerRelationUUIDByEndpointIdentifiers(c *tc.C) {
	// Arrange: Add an endpoint and a peer relation on it.

	endpoint1 := domainrelation.Endpoint{
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

	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)

	// Act: Get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

// TestGetPeerRelationUUIDByEndpointIdentifiersRelationNotFoundRegularRelation
// checks that the function returns not found if the endpoint is part of a
// regular relation, not a peer relation.
func (s *unitRelationsSuite) TestGetPeerRelationUUIDByEndpointIdentifiersRelationNotFoundRegularRelation(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.

	endpoint1 := domainrelation.Endpoint{
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

	endpoint2 := domainrelation.Endpoint{
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
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	expectedRelationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, expectedRelationUUID, applicationEndpointUUID2)

	// Act: Try and get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: endpoint1.ApplicationName,
			EndpointName:    endpoint1.Name,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *unitRelationsSuite) TestGetPeerRelationUUIDByEndpointIdentifiersNotFound(c *tc.C) {
	// Act: Try and get relation UUID from endpoint.
	_, err := s.state.GetPeerRelationUUIDByEndpointIdentifiers(
		c.Context(),
		corerelation.EndpointIdentifier{
			ApplicationName: "fake-application-1",
			EndpointName:    "fake-endpoint-name-1",
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and attributes. Returns the relation UUID.
func (s *unitRelationsSuite) addCharmRelation(c *tc.C, charmUUID string, r charm.Relation) string {
	charmRelationUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
	return charmRelationUUID
}

// addApplicationEndpoint inserts a new application endpoint into the database
// with the specified UUIDs. Returns the endpoint uuid.
func (s *unitRelationsSuite) addApplicationEndpoint(
	c *tc.C, applicationUUID string, charmRelationUUID string,
) string {
	applicationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
	return applicationEndpointUUID
}

// addRelation inserts a new relation into the database with default relation
// and life IDs. Returns the relation UUID.
func (s *unitRelationsSuite) addRelation(c *tc.C) corerelation.UUID {
	relationUUID := tc.Must(c, corerelation.NewUUID)
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id) 
VALUES (?,0,?,0)
`, relationUUID, s.relationCount)
	s.relationCount++
	return relationUUID
}

// addRelationEndpoint inserts a relation endpoint into the database
// using the provided UUIDs for relation. Returns the endpoint UUID.
func (s *unitRelationsSuite) addRelationEndpoint(
	c *tc.C, relationUUID corerelation.UUID, applicationEndpointUUID string,
) string {
	relationEndpointUUID := tc.Must(c, corerelation.NewEndpointUUID).String()
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
	return relationEndpointUUID
}

// addCharmRelationWithDefaults inserts a new charm relation into the database
// with the given UUID and predefined attributes. Returns the relation UUID.
func (s *unitRelationsSuite) addCharmRelationWithDefaults(c *tc.C, charmUUID string) string {
	charmRelationUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name)
VALUES (?, ?, 0, 0, 'fake-provides')
`, charmRelationUUID, charmUUID)
	return charmRelationUUID
}

// encodeRoleID returns the ID used in the database for the given charm role. This
// reflects the contents of the charm_relation_role table.
func (s *unitRelationsSuite) encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
	}[role]
}

// encodeScopeID returns the ID used in the database for the given charm scope. This
// reflects the contents of the charm_relation_scope table.
func (s *unitRelationsSuite) encodeScopeID(role charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[role]
}
