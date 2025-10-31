// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corelife "github.com/juju/juju/core/life"
	corelrelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/status"
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

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettings(c *tc.C) {
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

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
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

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsIdempotent(c *tc.C) {
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

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
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

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsUpdatesSettings(c *tc.C) {
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

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
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

	err = s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
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

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsMultiple(c *tc.C) {
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

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
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

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsMultipleMissingUnit(c *tc.C) {
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

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings1,
			"app1/2": settings3,
			"app1/4": {}, // Missing unit.
		},
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
	c.Check(err, tc.ErrorMatches, `.*missing: \[app1\/4\]`)
}

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsSubordinate(c *tc.C) {
	// Arrange: Populate charm metadata with subordinate data.
	s.addCharmMetadata(c, s.fakeCharmUUID1, true)
	s.addCharmMetadata(c, s.fakeCharmUUID2, false)

	// Arrange: Add container scoped endpoints on charm 1 and charm 2.
	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	endpoint2 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName2,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-2",
			Role:      charm.RoleRequirer,
			Interface: "ntp",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	charmRelationUUID2 := s.addCharmRelation(c, s.fakeCharmUUID2, endpoint2.Relation)

	// Arrange: Add a unit to application 1 and application 2, and make the unit
	// of application 1 a subordinate to the unit of application 2.
	unitName1 := coreunittesting.GenNewName(c, "app1/0")
	unitUUID1 := s.addUnit(c, unitName1, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	unitName2 := coreunittesting.GenNewName(c, "app2/0")
	unitUUID2 := s.addUnit(c, unitName2, s.fakeApplicationUUID2, s.fakeCharmUUID2)
	s.setUnitSubordinate(c, unitUUID1, unitUUID2)

	// Add a relation between application 1 and application 2.
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Act: Try and enter scope with the unit 1, which is a subordinate to an
	// application not in the relation.
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)
	c.Assert(err, tc.ErrorIs, relationerrors.CannotEnterScopeForSubordinate)
}

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsRelationNotAlive(c *tc.C) {
	// Arrange: Add two endpoints and a relation
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Dying, 17)

	// Arrange: Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Act: Enter scope.
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.CannotEnterScopeNotAlive)
}

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsUnitNotAlive(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)

	// Arrange: Add unit to application in the relation.
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnitWithLife(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1, corelife.Dead)

	// Act: Enter scope.
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsRelationNotFound(c *tc.C) {
	// Arrange: Add unit to application in the relation.
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := coreunittesting.GenNewName(c, "app1/0")
	s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)

	// Act: Try and enter scope.
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *remoteRelationSuite) TestSetRelationRemoteApplicationAndUnitSettingsUnitNotFound(c *tc.C) {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	// Act: Try and enter scope.
	settings := map[string]string{
		"ingress-address": "x.x.x.x",
		"another-key":     "another-value",
	}
	appSettings := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	err := s.state.SetRelationRemoteApplicationAndUnitSettings(c.Context(),
		s.fakeApplicationUUID1.String(),
		relationUUID.String(),
		appSettings,
		map[string]map[string]string{
			"app1/0": settings,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *remoteRelationSuite) TestSetRemoteRelationSuspendedStateOnNonRemoteRelation(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	err := s.state.SetRemoteRelationSuspendedState(c.Context(),
		relationUUID.String(),
		true,
		"foo reason",
	)
	c.Assert(err, tc.ErrorMatches, "relation must be a remote relation to be suspended")

	details, err := s.state.GetRelationDetails(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Suspended, tc.IsFalse)
}

func (s *remoteRelationSuite) TestSetRemoteRelationSuspendedStateFirstApplication(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Force the charm source to be a CMR.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = ?`, s.fakeCharmUUID1)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRemoteRelationSuspendedState(c.Context(),
		relationUUID.String(),
		true,
		"foo reason",
	)
	c.Assert(err, tc.ErrorIsNil)

	details, err := s.state.GetRelationDetails(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Suspended, tc.IsTrue)

	s.checkRelationStatus(c, relationUUID, status.RelationStatusTypeSuspending, "foo reason")
}

func (s *remoteRelationSuite) TestSetRemoteRelationSuspendedStateSecondApplication(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Force the charm source to be a CMR.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = ?`, s.fakeCharmUUID2)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRemoteRelationSuspendedState(c.Context(),
		relationUUID.String(),
		true,
		"foo reason",
	)
	c.Assert(err, tc.ErrorIsNil)

	details, err := s.state.GetRelationDetails(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Suspended, tc.IsTrue)

	s.checkRelationStatus(c, relationUUID, status.RelationStatusTypeSuspending, "foo reason")
}

func (s *remoteRelationSuite) TestSetRemoteRelationSuspendedStateFlipFlop(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Force the charm source to be a CMR.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = ?`, s.fakeCharmUUID1)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRemoteRelationSuspendedState(c.Context(),
		relationUUID.String(),
		true,
		"foo reason",
	)
	c.Assert(err, tc.ErrorIsNil)

	details, err := s.state.GetRelationDetails(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Suspended, tc.IsTrue)

	s.checkRelationStatus(c, relationUUID, status.RelationStatusTypeSuspending, "foo reason")

	err = s.state.SetRemoteRelationSuspendedState(c.Context(),
		relationUUID.String(),
		false,
		"foo reason",
	)
	c.Assert(err, tc.ErrorIsNil)

	details, err = s.state.GetRelationDetails(c.Context(), relationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details.Suspended, tc.IsFalse)

	s.checkRelationStatus(c, relationUUID, status.RelationStatusTypeJoining, "")
}

func (s *remoteRelationSuite) TestSetRelationErrorStatusNonCMR(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	err := s.state.SetRelationErrorStatus(c.Context(),
		relationUUID.String(),
		"error message",
	)
	c.Assert(err, tc.ErrorMatches, "relation must be a remote relation to set error status")
}

func (s *remoteRelationSuite) TestSetRelationErrorStatusFirstApplication(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Force the charm source to be a CMR.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = ?`, s.fakeCharmUUID1)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRelationErrorStatus(c.Context(),
		relationUUID.String(),
		"something went wrong",
	)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the status was set by querying the relation_status table.
	var statusID int
	var message string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT relation_status_type_id, message 
			FROM relation_status 
			WHERE relation_uuid = ?
		`, relationUUID.String()).Scan(&statusID, &message)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statusID, tc.Equals, 5) // RelationStatusTypeError = 5
	c.Check(message, tc.Equals, "something went wrong")
}

func (s *remoteRelationSuite) TestSetRelationErrorStatusSecondApplication(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Force the charm source to be a CMR on the second application.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = ?`, s.fakeCharmUUID2)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRelationErrorStatus(c.Context(),
		relationUUID.String(),
		"another error occurred",
	)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the status was set by querying the relation_status table.
	var statusID int
	var message string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT relation_status_type_id, message 
			FROM relation_status 
			WHERE relation_uuid = ?
		`, relationUUID.String()).Scan(&statusID, &message)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statusID, tc.Equals, 5) // RelationStatusTypeError = 5
	c.Check(message, tc.Equals, "another error occurred")
}

func (s *remoteRelationSuite) TestSetRelationErrorStatusUpdateExisting(c *tc.C) {
	// Arrange: Add two endpoints and a relation on them.
	endpoint1 := domainrelation.Endpoint{
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
	s.addApplicationEndpoint(c, s.fakeApplicationUUID2, charmRelationUUID2)
	relationUUID := s.addRelation(c)
	s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Force the charm source to be a CMR.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE charm SET source_id = 2, architecture_id = NULL WHERE uuid = ?`, s.fakeCharmUUID1)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Set the error status first time.
	err = s.state.SetRelationErrorStatus(c.Context(),
		relationUUID.String(),
		"first error",
	)
	c.Assert(err, tc.ErrorIsNil)

	// Update with a new error message.
	err = s.state.SetRelationErrorStatus(c.Context(),
		relationUUID.String(),
		"second error",
	)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the status was updated.
	var statusID int
	var message string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT relation_status_type_id, message 
			FROM relation_status 
			WHERE relation_uuid = ?
		`, relationUUID.String()).Scan(&statusID, &message)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statusID, tc.Equals, 5) // RelationStatusTypeError = 5
	c.Check(message, tc.Equals, "second error")
}

func (s *remoteRelationSuite) TestSetRelationErrorStatusRelationNotFound(c *tc.C) {
	err := s.state.SetRelationErrorStatus(c.Context(),
		corerelationtesting.GenRelationUUID(c).String(),
		"error message",
	)
	c.Assert(err, tc.ErrorMatches, ".*relation not found.*")
}

func (s *remoteRelationSuite) checkRelationStatus(c *tc.C, relationUUID corelrelation.UUID, expectedStatus status.RelationStatusType, expectedMessage string) {
	c.Helper()

	// Get the relation status to verify that the status became not suspended.
	var (
		statusID int
		message  string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT relation_status_type_id, message FROM relation_status WHERE relation_uuid = ?`, relationUUID)
		if err := row.Scan(&statusID, &message); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	statusType, err := status.DecodeRelationStatus(statusID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statusType, tc.Equals, expectedStatus)
	c.Check(message, tc.Equals, expectedMessage)
}
