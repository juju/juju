// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	baseRelationSuite

	fakeCharmUUID1       corecharm.ID
	fakeCharmUUID2       corecharm.ID
	fakeApplicationUUID1 coreapplication.ID
	fakeApplicationUUID2 coreapplication.ID
	fakeApplicationName1 string
	fakeApplicationName2 string
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) SetUpTest(c *tc.C) {
	s.baseRelationSuite.SetUpTest(c)

	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationName2 = "fake-application-2"

	// Populate DB with one application and charm.
	s.fakeCharmUUID1 = s.addCharm(c)
	s.fakeCharmUUID2 = s.addCharm(c)
	s.fakeApplicationUUID1 = s.addApplication(c, s.fakeCharmUUID1, s.fakeApplicationName1)
	s.fakeApplicationUUID2 = s.addApplication(c, s.fakeCharmUUID2, s.fakeApplicationName2)
}

func (s *migrationSuite) TestImportRelation(c *tc.C) {
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

	charm1 := s.addCharm(c)
	charm2 := s.addCharm(c)

	appUUID1 := s.addApplication(c, charm1, "application-1")
	appUUID2 := s.addApplication(c, charm2, "application-2")
	_ = s.addApplicationEndpointFromRelation(c, charm1, appUUID1, relProvider)
	_ = s.addApplicationEndpointFromRelation(c, charm1, appUUID2, relRequirer)
	_ = s.addApplicationEndpointFromRelation(c, charm2, appUUID2, relProvider)
	_ = s.addApplicationEndpointFromRelation(c, charm2, appUUID1, relRequirer)
	expectedRelID := uint64(42)

	// Act
	obtainedRelUUID, err := s.state.ImportRelation(c.Context(), corerelation.EndpointIdentifier{
		ApplicationName: "application-1",
		EndpointName:    "req",
	}, corerelation.EndpointIdentifier{
		ApplicationName: "application-2",
		EndpointName:    "prov",
	}, expectedRelID, charm.ScopeGlobal)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	foundRelUUID := s.fetchRelationUUIDByRelationID(c, expectedRelID)
	c.Assert(obtainedRelUUID, tc.Equals, foundRelUUID)
}

func (s *migrationSuite) TestGetApplicationIDByName(c *tc.C) {
	obtainedID, err := s.state.GetApplicationIDByName(c.Context(), s.fakeApplicationName1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedID, tc.Equals, s.fakeApplicationUUID1)
}

func (s *migrationSuite) TestGetApplicationIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationSuite) TestSetRelationApplicationSettings(c *tc.C) {
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
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key2": "value22",
		"key3": "",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value22",
	}
	for k, v := range initialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, tc.DeepEquals, expectedSettings)
}

func (s *migrationSuite) TestSetRelationApplicationSettingsNothingToSet(c *tc.C) {
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
	initialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	settingsUpdate := map[string]string{
		"key2": "",
		"key3": "",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
	}
	for k, v := range initialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, tc.DeepEquals, expectedSettings)
}

func (s *migrationSuite) TestSetRelationApplicationSettingsNothingToUnSet(c *tc.C) {
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
	initialSettings := map[string]string{
		"key1": "value1",
	}
	settingsUpdate := map[string]string{
		"key2": "value2",
		"key3": "value3",
	}
	expectedSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range initialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Act:
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		settingsUpdate,
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, tc.DeepEquals, expectedSettings)
}

func (s *migrationSuite) TestSetRelationApplicationSettingsNilMap(c *tc.C) {
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

	// Act:
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		nil,
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	foundSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Assert(foundSettings, tc.HasLen, 0)
}

// TestSetRelationApplicationSettingsCheckHash checks that the settings hash is
// updated when the settings are updated.
func (s *migrationSuite) TestSetRelationApplicationSettingsHashUpdated(c *tc.C) {
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

	// Arrange: Add some initial settings, this will also set the hash.
	initialSettings := map[string]string{
		"key1": "value1",
	}
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		initialSettings,
	)
	c.Assert(err, tc.ErrorIsNil)

	initialHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)

	// Act:
	err = s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		map[string]string{
			"key1": "value2",
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	// Assert: Check the hash has changed.
	foundHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)
	c.Assert(initialHash, tc.Not(tc.Equals), foundHash)
}

// TestSetRelationApplicationSettingsHashConstant checks that the settings hash
// is stays the same if the update does not actually change the settings.
func (s *migrationSuite) TestSetRelationApplicationSettingsHashConstant(c *tc.C) {
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

	// Arrange: Add some initial settings, this will also set the hash.
	settings := map[string]string{
		"key1": "value1",
	}
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		settings,
	)
	c.Assert(err, tc.ErrorIsNil)

	initialHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)

	// Act:
	err = s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		settings,
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil, tc.Commentf(errors.ErrorStack(err)))

	// Assert: Check the hash has changed.
	foundHash := s.getRelationApplicationSettingsHash(c, relationEndpointUUID1)
	c.Assert(initialHash, tc.Equals, foundHash)
}

func (s *migrationSuite) TestSetRelationApplicationSettingsApplicationNotFoundInRelation(c *tc.C) {
	// Arrange: Add relation.
	relationUUID := s.addRelation(c)

	// Act:
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		relationUUID,
		s.fakeApplicationUUID1,
		nil,
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.ApplicationNotFoundForRelation)
}

func (s *migrationSuite) TestSetRelationApplicationSettingsRelationNotFound(c *tc.C) {
	// Act:
	err := s.state.SetRelationApplicationSettings(
		c.Context(),
		"bad-uuid",
		s.fakeApplicationUUID1,
		nil,
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *migrationSuite) TestDeleteImportedRelations(c *tc.C) {
	// Arrange: Add a peer relation with one endpoint.
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
	for k, v := range appInitialSettings {
		s.addRelationApplicationSetting(c, relationEndpointUUID1, k, v)
	}

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/0")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	unitInitialSettings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range unitInitialSettings {
		s.addRelationUnitSetting(c, relationUnitUUID, k, v)
	}

	// Act
	err := s.state.DeleteImportedRelations(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkTableEmpty(c, "relation_unit_uuid", "relation_unit_settings")
	s.checkTableEmpty(c, "relation_unit_uuid", "relation_unit_settings_hash")
	s.checkTableEmpty(c, "uuid", "relation_unit")
	s.checkTableEmpty(c, "relation_endpoint_uuid", "relation_application_settings")
	s.checkTableEmpty(c, "relation_endpoint_uuid", "relation_application_settings_hash")
	s.checkTableEmpty(c, "uuid", "relation_endpoint")
	s.checkTableEmpty(c, "uuid", "relation")
}

func (s *migrationSuite) TestExportRelations(c *tc.C) {
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
	relationScope := charm.ScopeGlobal
	relationUUID := s.addRelationWithScope(c, relationScope)
	relEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	relEndpointUUID2 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	// Arrange: add application settings.
	s.addRelationApplicationSetting(c, relEndpointUUID2, "app-foo", "app-bar")

	// Arrange: add two relation units on endpoint 1.
	unitUUID1 := s.addUnit(c, "app1/0", s.fakeApplicationUUID1, s.fakeCharmUUID1)
	unitUUID2 := s.addUnit(c, "app1/1", s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relUnitUUID1 := s.addRelationUnit(c, unitUUID1, relEndpointUUID1)
	relUnitUUID2 := s.addRelationUnit(c, unitUUID2, relEndpointUUID1)

	// Arrange: add unit settings.
	s.addRelationUnitSetting(c, relUnitUUID1, "unit1-foo", "unit1-bar")
	s.addRelationUnitSetting(c, relUnitUUID2, "unit2-foo", "unit2-bar")

	// Arrange: add peer relation.
	peerEndpoint := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-3",
			Role:      charm.RolePeer,
			Interface: "peer",
			Optional:  true,
			Limit:     20,
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID3 := s.addCharmRelation(c, s.fakeCharmUUID1, peerEndpoint.Relation)
	applicationEndpointUUID3 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID3)
	peerScope := charm.ScopeGlobal
	peerRelationUUID := s.addRelationWithScope(c, peerScope)
	s.addRelationEndpoint(c, peerRelationUUID, applicationEndpointUUID3)

	// Act:
	exported, err := s.state.ExportRelations(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exported, tc.SameContents, []domainrelation.ExportRelation{{
		ID: 1,
		Endpoints: []domainrelation.ExportEndpoint{{
			ApplicationName: s.fakeApplicationName1,
			Name:            endpoint1.Name,
			Role:            endpoint1.Role,
			Interface:       endpoint1.Interface,
			Optional:        endpoint1.Optional,
			Limit:           endpoint1.Limit,
			Scope:           relationScope,
			AllUnitSettings: map[string]map[string]any{
				"app1/0": {
					"unit1-foo": "unit1-bar",
				},
				"app1/1": {
					"unit2-foo": "unit2-bar",
				},
			},
			ApplicationSettings: make(map[string]any),
		}, {
			ApplicationName: s.fakeApplicationName2,
			Name:            endpoint2.Name,
			Role:            endpoint2.Role,
			Interface:       endpoint2.Interface,
			Optional:        endpoint2.Optional,
			Limit:           endpoint2.Limit,
			Scope:           relationScope,
			ApplicationSettings: map[string]any{
				"app-foo": "app-bar",
			},
			AllUnitSettings: make(map[string]map[string]any),
		}},
	}, {
		ID: 2,
		Endpoints: []domainrelation.ExportEndpoint{{
			ApplicationName:     s.fakeApplicationName1,
			Name:                peerEndpoint.Name,
			Role:                peerEndpoint.Role,
			Interface:           peerEndpoint.Interface,
			Optional:            peerEndpoint.Optional,
			Limit:               peerEndpoint.Limit,
			Scope:               peerScope,
			AllUnitSettings:     make(map[string]map[string]any),
			ApplicationSettings: make(map[string]any),
		}},
	}})
}

// addApplicationEndpointFromRelation creates and associates a new application
// endpoint based on the provided relation.
func (s *migrationSuite) addApplicationEndpointFromRelation(c *tc.C,
	charmUUID corecharm.ID,
	appUUID coreapplication.ID,
	relation charm.Relation) corerelation.EndpointUUID {

	// todo(gfouillet) introduce proper generation for this uuid
	charmRelationUUID := uuid.MustNewUUID()
	relationEndpointUUID := corerelationtesting.GenEndpointUUID(c)

	// Add relation to charm
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, interface, capacity, role_id,  scope_id)
SELECT ?, ?, ?, ?, ?, crr.id, crs.id
FROM charm_relation_scope crs
JOIN charm_relation_role crr ON crr.name = ?
WHERE crs.name = ?
`, charmRelationUUID.String(), charmUUID.String(), relation.Name,
		relation.Interface, relation.Limit, relation.Role, relation.Scope)

	// application endpoint
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?,?,?,?)
`, relationEndpointUUID.String(), appUUID.String(), charmRelationUUID.String(), network.AlphaSpaceId)

	return relationEndpointUUID
}

func (s *migrationSuite) checkTableEmpty(c *tc.C, colName, tableName string) {
	query := fmt.Sprintf(`
SELECT %s
FROM   %s
`, colName, tableName)

	values := []string{}
	_ = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)

		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				return errors.Capture(err)
			}
			values = append(values, value)
		}
		return nil
	})
	c.Check(values, tc.DeepEquals, []string{}, tc.Commentf("table %q first value: %q", tableName, strings.Join(values, ", ")))
}
