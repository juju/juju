// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	corerelation "github.com/juju/juju/core/relation"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/deployment/charm"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

type unitRelationsSuite struct {
	commitHookBaseSuite
}

func TestUnitRelationsSuite(t *testing.T) {
	tc.Run(t, &unitRelationsSuite{})
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

func (s *unitRelationsSuite) TestSetRelationApplicationAndUnitSettings(c *tc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Declare settings and add initial settings.
	appInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	appSettingsUpdate := map[string]string{
		"key2": "value22",
	}
	appExpectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range appInitialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/7")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	unitInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	unitSettingsUpdate := map[string]string{
		"key2": "value22",
	}
	unitExpectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range unitInitialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act:
	err := s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setRelationApplicationAndUnitSettings(
			c.Context(),
			tx,
			unitUUID.String(),
			internal.RelationSettings{
				RelationUUID:     relationUUID,
				UnitSet:          unitSettingsUpdate,
				UnitUnset:        []string{"key3"},
				ApplicationSet:   appSettingsUpdate,
				ApplicationUnset: []string{"key3"},
			},
		)
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Check(foundAppSettings, tc.DeepEquals, appExpectedSettings)
	foundUnitSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(foundUnitSettings, tc.DeepEquals, unitExpectedSettings)
}

func (s *unitRelationsSuite) TestSetRelationApplicationAndUnitSettingsNilMap(c *tc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/3")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Act:
	err := s.Txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setRelationApplicationAndUnitSettings(
			c.Context(),
			tx,
			unitUUID.String(),
			internal.RelationSettings{
				RelationUUID: relationUUID,
			},
		)
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(foundSettings, tc.HasLen, 0)
	foundSettings = s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Check(foundSettings, tc.HasLen, 0)
}
