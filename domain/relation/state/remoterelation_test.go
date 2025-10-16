// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
)

type remoteRelationSuite struct {
	baseRelationSuite

	fakeCharmUUID1                corecharm.ID
	fakeCharmUUID2                corecharm.ID
	fakeApplicationUUID1          coreapplication.UUID
	fakeApplicationUUID2          coreapplication.UUID
	fakeApplicationName1          string
	fakeApplicationName2          string
	fakeCharmRelationProvidesUUID string
}

func TestRemoteRelationSuite(t *testing.T) {
	tc.Run(t, &remoteRelationSuite{})
}

func (s *remoteRelationSuite) SetUpTest(c *tc.C) {
	s.baseRelationSuite.SetUpTest(c)

	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationName2 = "fake-application-2"

	// Populate DB with one application and charm.
	s.fakeCharmUUID1 = s.addCharm(c)
	s.fakeCharmUUID2 = s.addCharm(c)
	s.fakeCharmRelationProvidesUUID = s.addCharmRelationWithDefaults(c, s.fakeCharmUUID1)
	s.fakeApplicationUUID1 = s.addApplication(c, s.fakeCharmUUID1, s.fakeApplicationName1)
	s.fakeApplicationUUID2 = s.addApplication(c, s.fakeCharmUUID2, s.fakeApplicationName2)
}

func (s *remoteRelationSuite) TestRemoteUnitsEnterScope(c *tc.C) {
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	relationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(relationUUID.Validate(), tc.ErrorIsNil)

	obtainedSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(obtainedSettings, tc.DeepEquals, settings)

	obtainedHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(obtainedHash, tc.Not(tc.Equals), "")

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundAppSettings, tc.DeepEquals, appSettings)
}

func (s *remoteRelationSuite) TestRemoteUnitsEnterScopeIdempotent(c *tc.C) {
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	relationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(relationUUID.Validate(), tc.ErrorIsNil)

	obtainedSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(obtainedSettings, tc.DeepEquals, settings)

	obtainedHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(obtainedHash, tc.Not(tc.Equals), "")

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundAppSettings, tc.DeepEquals, appSettings)
}

func (s *remoteRelationSuite) TestRemoteUnitsEnterScopeUpdatesSettings(c *tc.C) {
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	unitName := coreunittesting.GenNewName(c, "app1/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	settings = map[string]string{
		"ingress-address": "y.y.y.y", // Updated.
		"new-key":         "new-value",
	}
	appSettings = map[string]string{
		"foo": "new-bar", // Updated.
		"baz": "qux",
		"new": "setting",
	}

	err = s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	relationUnitUUID := s.getRelationUnitInScope(c, relationUUID, unitUUID)
	c.Check(relationUUID.Validate(), tc.ErrorIsNil)

	obtainedSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(obtainedSettings, tc.DeepEquals, settings)

	obtainedHash := s.getRelationUnitSettingsHash(c, relationUnitUUID)
	c.Assert(obtainedHash, tc.Not(tc.Equals), "")

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundAppSettings, tc.DeepEquals, appSettings)
}

func (s *remoteRelationSuite) TestRemoteUnitsEnterScopeMultiple(c *tc.C) {
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)

	relationUUID := s.addRelation(c)
	relationEndpointUUID := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	unitName3 := coreunittesting.GenNewName(c, "app1/2")
	unitUUID1 := s.addUnit(c, unitName1, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	unitUUID3 := s.addUnit(c, unitName3, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	settings1 := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	settings3 := map[string]string{
		"ingress-address": "y.y.y.y",
		"other-key":       "other-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings1,
			"app1/2": settings3,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	relationUnitUUID1 := s.getRelationUnitInScope(c, relationUUID, unitUUID1)
	c.Check(relationUUID.Validate(), tc.ErrorIsNil)

	relationUnitUUID3 := s.getRelationUnitInScope(c, relationUUID, unitUUID3)
	c.Check(relationUUID.Validate(), tc.ErrorIsNil)

	obtainedSettings1 := s.getRelationUnitSettings(c, relationUnitUUID1)
	c.Check(obtainedSettings1, tc.DeepEquals, settings1)

	obtainedSettings3 := s.getRelationUnitSettings(c, relationUnitUUID3)
	c.Check(obtainedSettings3, tc.DeepEquals, settings3)

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID)
	c.Assert(foundAppSettings, tc.DeepEquals, appSettings)
}

func (s *remoteRelationSuite) TestRemoteUnitsEnterScopeMultipleMissingUnit(c *tc.C) {
	s.addCharmMetadata(c, s.fakeCharmUUID1, false)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	endpoint2 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "database",
			Scope:     charm.ScopeGlobal,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)

	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	unitName3 := coreunittesting.GenNewName(c, "app1/2")
	s.addUnit(c, unitName1, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	s.addUnit(c, unitName3, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	settings1 := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	settings3 := map[string]string{
		"ingress-address": "y.y.y.y",
		"other-key":       "other-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.RemoteUnitsEnterScope(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings1,
			"app1/2": settings3,
			"app1/4": {}, // Missing unit.
		},
	)
	c.Assert(err, tc.ErrorIs, relationerrors.UnitNotFound)
	c.Check(err, tc.ErrorMatches, `.*missing: \[app1\/4\]`)
}
