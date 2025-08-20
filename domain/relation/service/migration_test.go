// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
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
			ID:    7,
			Key:   key1,
			Scope: charm.ScopeContainer,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep1[0].ApplicationName,
					EndpointName:        ep1[0].EndpointName,
					ApplicationSettings: map[string]interface{}{"five": "six"},
					UnitSettings: map[string]map[string]interface{}{
						"ubuntu/0": {"one": "two"},
					},
				},
			},
		}, {
			ID:    8,
			Key:   key2,
			Scope: charm.ScopeGlobal,
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     ep2[0].ApplicationName,
					EndpointName:        ep2[0].EndpointName,
					ApplicationSettings: map[string]interface{}{"foo": "six"},
					UnitSettings: map[string]map[string]interface{}{
						"ubuntu/0": {"test": "two"},
					},
				}, {
					ApplicationName:     ep2[1].ApplicationName,
					EndpointName:        ep2[1].EndpointName,
					ApplicationSettings: map[string]interface{}{"three": "four"},
					UnitSettings: map[string]map[string]interface{}{
						"ntp/0": {"seven": "six"},
					},
				},
			},
		},
	}
	peerRelUUID := s.expectGetPeerRelationUUIDByEndpointIdentifiers(c, ep1[0])
	relUUID := s.expectImportRelation(c, ep2[0], ep2[1], uint64(8), charm.ScopeGlobal)
	app1ID := s.expectGetApplicationIDByName(c, args[0].Endpoints[0].ApplicationName)
	app2ID := s.expectGetApplicationIDByName(c, args[1].Endpoints[0].ApplicationName)
	app3ID := s.expectGetApplicationIDByName(c, args[1].Endpoints[1].ApplicationName)
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

func (s *migrationServiceSuite) TestDeleteImportedRelationsError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeleteImportedRelations(gomock.Any()).Return(errors.New("boom"))

	// Act
	err := s.service.DeleteImportedRelations(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
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

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	s.service = NewMigrationService(s.state)

	return ctrl
}

func (s *migrationServiceSuite) expectGetPeerRelationUUIDByEndpointIdentifiers(
	c *tc.C,
	endpoint corerelation.EndpointIdentifier,
) corerelation.UUID {
	relUUID := corerelationtesting.GenRelationUUID(c)
	s.state.EXPECT().GetPeerRelationUUIDByEndpointIdentifiers(gomock.Any(), endpoint).Return(relUUID, nil)
	return relUUID
}

func (s *migrationServiceSuite) expectImportRelation(
	c *tc.C,
	ep2, ep3 corerelation.EndpointIdentifier,
	id uint64,
	scope charm.RelationScope,
) corerelation.UUID {
	relUUID := corerelationtesting.GenRelationUUID(c)
	s.state.EXPECT().ImportRelation(gomock.Any(), ep2, ep3, id, scope).Return(relUUID, nil)
	return relUUID
}

func (s *migrationServiceSuite) expectGetApplicationIDByName(c *tc.C, name string) coreapplication.ID {
	appID := coreapplicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), name).Return(appID, nil)
	return appID
}

func (s *migrationServiceSuite) expectSetRelationApplicationSettings(
	uuid corerelation.UUID,
	id coreapplication.ID,
	settings map[string]interface{},
) {
	appSettings, _ := settingsMap(settings)
	s.state.EXPECT().SetRelationApplicationSettings(gomock.Any(), uuid, id, appSettings).Return(nil)
}

func (s *migrationServiceSuite) expectEnterScope(
	uuid corerelation.UUID,
	name coreunit.Name,
	settings map[string]interface{},
) {
	unitSettings, _ := settingsMap(settings)
	s.state.EXPECT().EnterScope(gomock.Any(), uuid, name, unitSettings).Return(nil)
}
