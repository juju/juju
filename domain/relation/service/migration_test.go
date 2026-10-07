// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type migrationServiceSuite struct {
	testhelpers.IsolationSuite

	state   *MockMigrationState
	service *MigrationService
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) TestImportRelations(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key1 := corerelationtesting.GenNewKey(c, "ubuntu:peer")
	ep1 := key1.EndpointIdentifiers()
	key2 := corerelationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info")
	ep2 := key2.EndpointIdentifiers()

	args := relation.ImportRelationsArgs{
		{
			UUID:  tc.Must(c, corerelation.NewUUID),
			ID:    7,
			Key:   key1,
			Scope: charm.ScopeContainer,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep1[0].ApplicationName,
					EndpointName:        ep1[0].EndpointName,
					ApplicationSettings: map[string]any{"five": "six"},
					UnitSettings: map[string]map[string]any{
						"ubuntu/0": {"one": "two"},
					},
				},
			},
		}, {
			UUID:  tc.Must(c, corerelation.NewUUID),
			ID:    8,
			Key:   key2,
			Scope: charm.ScopeGlobal,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep2[0].ApplicationName,
					EndpointName:        ep2[0].EndpointName,
					ApplicationSettings: map[string]any{"foo": "six"},
					UnitSettings: map[string]map[string]any{
						"ubuntu/0": {"test": "two"},
					},
				}, {
					ApplicationName:     ep2[1].ApplicationName,
					EndpointName:        ep2[1].EndpointName,
					ApplicationSettings: map[string]any{"three": "four"},
					UnitSettings: map[string]map[string]any{
						"ntp/0": {"seven": "six"},
					},
				},
			},
		},
	}

	peerRelUUID := args[0].UUID
	relUUID := args[1].UUID

	s.expectImportPeerRelation(peerRelUUID, ep1[0], uint64(7), charm.ScopeContainer)
	s.expectImportRelation(relUUID, ep2[0], ep2[1], uint64(8), charm.ScopeGlobal)
	app1ID := s.expectGetApplicationUUIDByName(c, args[0].Endpoints[0].ApplicationName)
	app2ID := s.expectGetApplicationUUIDByName(c, args[1].Endpoints[0].ApplicationName)
	app3ID := s.expectGetApplicationUUIDByName(c, args[1].Endpoints[1].ApplicationName)
	s.expectSetRelationApplicationSettings(peerRelUUID, app1ID, args[0].Endpoints[0].ApplicationSettings)
	s.expectSetRelationApplicationSettings(relUUID, app2ID, args[1].Endpoints[0].ApplicationSettings)
	s.expectSetRelationApplicationSettings(relUUID, app3ID, args[1].Endpoints[1].ApplicationSettings)
	settings := args[0].Endpoints[0].UnitSettings["ubuntu/0"]
	s.expectEnterScope(peerRelUUID, coreunittesting.GenNewName(c, "ubuntu/0"), settings)
	settings = args[1].Endpoints[0].UnitSettings["ubuntu/0"]
	s.expectEnterScope(relUUID, coreunittesting.GenNewName(c, "ubuntu/0"), settings)
	settings = args[1].Endpoints[1].UnitSettings["ntp/0"]
	s.expectEnterScope(relUUID, coreunittesting.GenNewName(c, "ntp/0"), settings)

	// Act
	err := s.service.ImportRelations(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnits(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	ep := key.EndpointIdentifiers()
	relUUID := tc.Must(c, corerelation.NewUUID)

	args := relation.ImportRelationSettingsAndUnitsArgs{
		{
			UUID: relUUID,
			Key:  key,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep[0].ApplicationName,
					EndpointName:        ep[0].EndpointName,
					ApplicationSettings: map[string]any{"password": "keep-me"},
				}, {
					ApplicationName:     ep[1].ApplicationName,
					EndpointName:        ep[1].EndpointName,
					ApplicationSettings: map[string]any{"database": "keep-me-too"},
					UnitSettings: map[string]map[string]any{
						"remote-13ea/0": {"request": "keep-unit-data"},
					},
				},
			},
		},
	}

	s.expectGetRelationEndpoints(relUUID, ep)
	app1ID := s.expectGetApplicationUUIDByName(c, args[0].Endpoints[0].ApplicationName)
	app2ID := s.expectGetApplicationUUIDByName(c, args[0].Endpoints[1].ApplicationName)
	s.expectSetRelationApplicationSettings(relUUID, app1ID, args[0].Endpoints[0].ApplicationSettings)
	s.expectSetRelationApplicationSettings(relUUID, app2ID, args[0].Endpoints[1].ApplicationSettings)
	s.expectEnterScope(relUUID, coreunittesting.GenNewName(c, "remote-13ea/0"), args[0].Endpoints[1].UnitSettings["remote-13ea/0"])

	// The relation is not created by ImportConsumerProxyRelationSettingsAndUnits,
	// it already exists.
	s.state.EXPECT().ImportRelation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	s.state.EXPECT().ImportPeerRelation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsInvalidUUID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: "not-a-uuid",
		Key:  key,
	}})

	// Assert
	c.Assert(err, tc.ErrorMatches, "validating relation UUID:.*")
}

// The key is validated before the relation is located, so an invalid key
// fails the import even when the relation exists.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsInvalidKey(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: tc.Must(c, corerelation.NewUUID),
		Key:  corerelation.Key{},
	}})

	// Assert
	c.Assert(err, tc.ErrorMatches, "validating relation key:.*")
}

// The relations of remote application consumers are created by the cross
// model relation import, so a relation that does not exist is an ordering
// error rather than a missing relation to create.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsRelationNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	relUUID := tc.Must(c, corerelation.NewUUID)

	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relUUID.String()).
		Return(nil, relationerrors.RelationNotFound)

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
	}})

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*relation.*not found.*created by the cross model relation import.*")
}

// The key of the located relation is checked against the key of the
// argument, so a token pointing at another relation fails the import
// instead of attaching the data to it.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsKeyMismatch(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	otherKey := corerelationtesting.GenNewKey(c, "mysql:db mediawiki:db")
	relUUID := tc.Must(c, corerelation.NewUUID)

	s.expectGetRelationEndpoints(relUUID, otherKey.EndpointIdentifiers())

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
	}})

	// Assert
	c.Assert(err, tc.ErrorMatches, `relation ".*" has key "mysql:db mediawiki:db", not "wordpress:db remote-13ea:db"`)
}

// Any error other than RelationNotFound from the relation lookup surfaces
// unchanged.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsRelationLookupError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	relUUID := tc.Must(c, corerelation.NewUUID)
	boom := errors.New("boom")

	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relUUID.String()).
		Return(nil, boom)

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
	}})

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

// An import that failed part way through is retried, so a unit that is already
// in the relation scope is not an error: it is in scope, with the settings it
// was given when it first entered, which EnterScope records alongside the
// scope membership.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsUnitAlreadyInScope(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	ep := key.EndpointIdentifiers()
	relUUID := tc.Must(c, corerelation.NewUUID)

	unitSettings := map[string]any{"request": "keep-unit-data"}
	args := relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
		Endpoints: []relation.ImportEndpoint{{
			ApplicationName: ep[1].ApplicationName,
			EndpointName:    ep[1].EndpointName,
			UnitSettings: map[string]map[string]any{
				"remote-13ea/0": unitSettings,
			},
		}},
	}}

	s.expectGetRelationEndpoints(relUUID, ep)
	appID := s.expectGetApplicationUUIDByName(c, ep[1].ApplicationName)
	s.expectSetRelationApplicationSettings(relUUID, appID, nil)
	converted, _ := settingsMap(func(string) {}, unitSettings)
	s.state.EXPECT().EnterScope(gomock.Any(), relUUID,
		coreunittesting.GenNewName(c, "remote-13ea/0"), converted).
		Return(internal.SubordinateUnitStatusHistoryData{},
			relationerrors.RelationUnitAlreadyExists)

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// Only RelationUnitAlreadyExists is tolerated: any other error from the
// state layer surfaces, so unit data is never dropped silently.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsStateError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	ep := key.EndpointIdentifiers()
	relUUID := tc.Must(c, corerelation.NewUUID)

	unitSettings := map[string]any{"request": "keep-unit-data"}
	args := relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
		Endpoints: []relation.ImportEndpoint{{
			ApplicationName: ep[1].ApplicationName,
			EndpointName:    ep[1].EndpointName,
			ApplicationSettings: map[string]any{
				"database": "keep-me-too",
			},
			UnitSettings: map[string]map[string]any{
				"remote-13ea/0": unitSettings,
			},
		}},
	}}

	s.expectGetRelationEndpoints(relUUID, ep)
	appID := s.expectGetApplicationUUIDByName(c, ep[1].ApplicationName)
	s.expectSetRelationApplicationSettings(relUUID, appID, args[0].Endpoints[0].ApplicationSettings)
	converted, _ := settingsMap(func(string) {}, unitSettings)
	s.state.EXPECT().EnterScope(gomock.Any(), relUUID,
		coreunittesting.GenNewName(c, "remote-13ea/0"), converted).
		Return(internal.SubordinateUnitStatusHistoryData{},
			relationerrors.RelationNotFound)

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

// A failure to write the application settings surfaces, so the endpoint
// data is never dropped silently.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsSetSettingsError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	ep := key.EndpointIdentifiers()
	relUUID := tc.Must(c, corerelation.NewUUID)
	boom := errors.New("boom")

	args := relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
		Endpoints: []relation.ImportEndpoint{{
			ApplicationName:     ep[0].ApplicationName,
			EndpointName:        ep[0].EndpointName,
			ApplicationSettings: map[string]any{"password": "keep-me"},
		}},
	}}

	s.expectGetRelationEndpoints(relUUID, ep)
	appID := s.expectGetApplicationUUIDByName(c, ep[0].ApplicationName)
	appSettings, err := settingsMap(func(string) {}, args[0].Endpoints[0].ApplicationSettings)
	c.Assert(err, tc.ErrorIsNil)
	s.state.EXPECT().SetRelationApplicationSettings(gomock.Any(), relUUID, appID, appSettings).
		Return(boom)

	// Act
	err = s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

// An application that does not exist is reported rather than silently
// dropping the endpoint data.
func (s *migrationServiceSuite) TestImportConsumerProxyRelationSettingsAndUnitsApplicationNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	key := corerelationtesting.GenNewKey(c, "wordpress:db remote-13ea:db")
	ep := key.EndpointIdentifiers()
	relUUID := tc.Must(c, corerelation.NewUUID)

	args := relation.ImportRelationSettingsAndUnitsArgs{{
		UUID: relUUID,
		Key:  key,
		Endpoints: []relation.ImportEndpoint{{
			ApplicationName:     ep[0].ApplicationName,
			EndpointName:        ep[0].EndpointName,
			ApplicationSettings: map[string]any{"password": "keep-me"},
		}},
	}}

	s.expectGetRelationEndpoints(relUUID, ep)
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), ep[0].ApplicationName).
		Return("", applicationerrors.ApplicationNotFound)

	// Act
	err := s.service.ImportConsumerProxyRelationSettingsAndUnits(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationServiceSuite) TestExportRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.state.EXPECT().ExportRelations(gomock.Any()).Return([]relation.ExportRelation{{
		Endpoints: []relation.ExportEndpoint{{
			ApplicationName: "app1",
			Name:            "ep1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			Name:            "ep2",
			Role:            charm.RoleProvider,
		}},
	}}, nil)

	// Act:
	relations, err := s.service.ExportRelations(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relations, tc.DeepEquals, []relation.ExportRelation{{
		Endpoints: []relation.ExportEndpoint{{
			ApplicationName: "app1",
			Name:            "ep1",
			Role:            charm.RoleRequirer,
		}, {
			ApplicationName: "app2",
			Name:            "ep2",
			Role:            charm.RoleProvider,
		}},
		Key: corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2"),
	}})
}

func (s *migrationServiceSuite) TestExportRelationsStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	boom := errors.New("boom")
	s.state.EXPECT().ExportRelations(gomock.Any()).Return(nil, boom)

	// Act:
	_, err := s.service.ExportRelations(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *migrationServiceSuite) TestImportNoEmptySettingsValues(c *tc.C) {
	// Arrange
	in := map[string]any{
		"one":   "two",
		"three": "",
	}
	expected := map[string]string{
		"one": "two",
	}

	// Act
	obtained, err := settingsMap(func(string) {}, in)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *migrationServiceSuite) TestImportSettingsValuesMustBeStrings(c *tc.C) {
	// Arrange
	in := map[string]any{
		"one":   "two",
		"three": map[string]string{"foo": "bar"},
	}

	// Act
	_, err := settingsMap(func(string) {}, in)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".* not a string")
}

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	s.service = NewMigrationService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}

func (s *migrationServiceSuite) expectImportPeerRelation(
	relUUID corerelation.UUID,
	endpoint corerelation.EndpointIdentifier,
	id uint64,
	scope charm.RelationScope,
) {
	s.state.EXPECT().ImportPeerRelation(gomock.Any(), relUUID.String(), endpoint, id, scope).Return(nil)
}

func (s *migrationServiceSuite) expectImportRelation(
	relUUID corerelation.UUID,
	ep2, ep3 corerelation.EndpointIdentifier,
	id uint64,
	scope charm.RelationScope,
) {
	s.state.EXPECT().ImportRelation(gomock.Any(), relUUID.String(), ep2, ep3, id, scope).Return(nil)
}

func (s *migrationServiceSuite) expectGetApplicationUUIDByName(c *tc.C, name string) coreapplication.UUID {
	appID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), name).Return(appID, nil)
	return appID
}

func (s *migrationServiceSuite) expectSetRelationApplicationSettings(
	uuid corerelation.UUID,
	id coreapplication.UUID,
	settings map[string]any,
) {
	appSettings, _ := settingsMap(func(string) {}, settings)
	s.state.EXPECT().SetRelationApplicationSettings(gomock.Any(), uuid, id, appSettings).Return(nil)
}

func (s *migrationServiceSuite) expectGetRelationEndpoints(
	relUUID corerelation.UUID,
	eps []corerelation.EndpointIdentifier,
) {
	endpoints := make([]relation.Endpoint, len(eps))
	for i, ep := range eps {
		endpoints[i] = relation.Endpoint{
			ApplicationName: ep.ApplicationName,
			Relation: charm.Relation{
				Name: ep.EndpointName,
				Role: ep.Role,
			},
		}
	}
	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), relUUID.String()).Return(endpoints, nil)
}

func (s *migrationServiceSuite) expectEnterScope(
	uuid corerelation.UUID,
	name coreunit.Name,
	settings map[string]any,
) {
	unitSettings, _ := settingsMap(func(string) {}, settings)
	data := internal.SubordinateUnitStatusHistoryData{}
	s.state.EXPECT().EnterScope(gomock.Any(), uuid, name, unitSettings).Return(data, nil)
}
