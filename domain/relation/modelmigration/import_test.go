// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/relation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite

	service *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImport(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	model := s.expectImportRelations(c, map[int]corerelation.Key{
		3: relationtesting.GenNewKey(c, "ubuntu:peer"),
		7: relationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info"),
	}, charm.ScopeGlobal)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) TestImportRelationsWithContainerScope(c *tc.C) {
	// Arrange
	defer s.setupMocks(c)

	model := s.expectImportRelations(c, map[int]corerelation.Key{
		3: relationtesting.GenNewKey(c, "ubuntu:peer"),
		7: relationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info"),
	}, charm.ScopeContainer)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) TestImportSkipsConsumerRemoteRelations(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	key := relationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info")

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel := model.AddRelation(description.RelationArgs{
		Id:  1,
		Key: key.String(),
	})

	eps := key.EndpointIdentifiers()
	for _, ep := range eps {
		rel.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
			Scope:           string(charm.ScopeGlobal),
		})
	}

	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "ubuntu",
		IsConsumerProxy: true,
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) TestImportSkipsConsumerRemoteRelationsWithOtherRelations(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	key0 := relationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info")
	key1 := relationtesting.GenNewKey(c, "mysql:db ntp:juju-info")

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel0 := model.AddRelation(description.RelationArgs{
		Id:  1,
		Key: key0.String(),
	})
	for _, ep := range key0.EndpointIdentifiers() {
		rel0.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
			Scope:           string(charm.ScopeGlobal),
		})
	}
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "ubuntu",
		IsConsumerProxy: true,
	})

	rel1 := model.AddRelation(description.RelationArgs{
		Id:  2,
		Key: key1.String(),
	})
	for _, ep := range key1.EndpointIdentifiers() {
		rel1.AddEndpoint(description.EndpointArgs{
			ApplicationName: ep.ApplicationName,
			Name:            ep.EndpointName,
			Role:            string(ep.Role),
			Scope:           string(charm.ScopeGlobal),
		})
	}
	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "mysql",
		IsConsumerProxy: false,
	})

	s.service.EXPECT().ImportRelations(gomock.Any(), []relation.ImportRelationArg{{
		ID:  2,
		Key: key1,
		Endpoints: []relation.ImportEndpoint{{
			ApplicationName:     "mysql",
			EndpointName:        "db",
			ApplicationSettings: map[string]any{},
			UnitSettings:        map[string]map[string]any{},
		}, {
			ApplicationName:     "ntp",
			EndpointName:        "juju-info",
			ApplicationSettings: map[string]any{},
			UnitSettings:        map[string]map[string]any{},
		}},
		Scope: charm.ScopeGlobal,
	}}).Return(nil)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) TestImportNoRelations(c *tc.C) {
	// Arrange
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) TestImportBadKey(c *tc.C) {
	// Arrange
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "failme",
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

// Ignore consumer proxies when matching remote applications for a relation, as
// these are not imported as part of the relation domain, but rather as part of
// the crossmodelrelation domain.
func (s *importSuite) TestImportConsumerRemoteRelation(c *tc.C) {
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel := model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "foo:sink dummy-sink:source",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
		Interface:       "dummy-token",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
		Interface:       "dummy-token",
	})

	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "foo",
		IsConsumerProxy: true,
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.IsNil)
}

// Ignore consumer proxies when matching remote applications for a relation for
// any endpoint application name. We can't guarantee the order of relation
// endpoints, so we need to check all endpoints for a relation for any consumer
// proxy remote application, and ignore the relation.
func (s *importSuite) TestImportConsumerRemoteRelationOtherEndpoint(c *tc.C) {
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel := model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "foo:sink dummy-sink:source",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
		Interface:       "dummy-token",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
		Interface:       "dummy-token",
	})

	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "dummy-sink",
		IsConsumerProxy: true,
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) TestImportOffererRemoteRelation(c *tc.C) {
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel := model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "foo:sink dummy-sink:source",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
		Interface:       "dummy-token",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
		Interface:       "dummy-token",
	})

	model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "foo",
	})

	s.service.EXPECT().ImportRelations(gomock.Any(), relation.ImportRelationsArgs{
		{
			ID:  32,
			Key: relationtesting.GenNewKey(c, "foo:sink dummy-sink:source"),
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     "foo",
					EndpointName:        "sink",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
				{
					ApplicationName:     "dummy-sink",
					EndpointName:        "source",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
			},
			Scope: charm.ScopeGlobal,
		},
	}).Return(nil)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.IsNil)
}

// If there are multiple remote applications with the same offer UUID and
// endpoints, but different names, we should de-duplicate these remote
// applications and import the relation using one of these remote applications,
// as they are effectively the same offerer application.
func (s *importSuite) TestImportOffererRemoteRelationMultipleMatchingRemoteApplications(c *tc.C) {
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel := model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "foo:sink dummy-sink:source",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
		Interface:       "dummy-token",
	})
	rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
		Interface:       "dummy-token",
	})

	remoteApp0 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "foo",
		OfferUUID:       "b9424bc3-1053-43a8-8386-c2faf56c4e9a",
		SourceModelUUID: "7e2421b5-4605-42d8-8636-bfcf7c35129f",
	})
	remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "sink",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	remoteApp1 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "bar",
		OfferUUID:       "b9424bc3-1053-43a8-8386-c2faf56c4e9a",
		SourceModelUUID: "7e2421b5-4605-42d8-8636-bfcf7c35129f",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "sink",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	s.service.EXPECT().ImportRelations(gomock.Any(), relation.ImportRelationsArgs{
		{
			ID:  32,
			Key: relationtesting.GenNewKey(c, "foo:sink dummy-sink:source"),
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     "foo",
					EndpointName:        "sink",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
				{
					ApplicationName:     "dummy-sink",
					EndpointName:        "source",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
			},
			Scope: charm.ScopeGlobal,
		},
	}).Return(nil)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.IsNil)
}

// Test to ensure that relations that refer to the same application with a
// different name, that are not remote do not interfere with the import of
// remote relations.
func (s *importSuite) TestImportRemoteRelations(c *tc.C) {
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	rel0 := model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "foo:sink dummy-sink:source",
	})
	rel0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
		Name:            "sink",
		Interface:       "dummy-token",
	})
	rel0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
		Interface:       "dummy-token",
	})

	rel1 := model.AddRelation(description.RelationArgs{
		Id:  33,
		Key: "dummy-source:sink dummy-sink:source",
	})
	rel1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-source",
		Name:            "sink",
		Interface:       "dummy-token",
	})
	rel1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-sink",
		Name:            "source",
		Interface:       "dummy-token",
	})

	remoteApp0 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "foo",
		OfferUUID:       "b9424bc3-1053-43a8-8386-c2faf56c4e9a",
		SourceModelUUID: "7e2421b5-4605-42d8-8636-bfcf7c35129f",
	})
	remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "sink",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	remoteApp1 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "bar",
		OfferUUID:       "b9424bc3-1053-43a8-8386-c2faf56c4e9a",
		SourceModelUUID: "7e2421b5-4605-42d8-8636-bfcf7c35129f",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "sink",
		Interface: "dummy-token",
		Role:      "requirer",
	})

	s.service.EXPECT().ImportRelations(gomock.Any(), relation.ImportRelationsArgs{
		{
			ID:  32,
			Key: relationtesting.GenNewKey(c, "foo:sink dummy-sink:source"),
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     "foo",
					EndpointName:        "sink",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
				{
					ApplicationName:     "dummy-sink",
					EndpointName:        "source",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
			},
			Scope: charm.ScopeGlobal,
		},
		{
			ID:  33,
			Key: relationtesting.GenNewKey(c, "dummy-source:sink dummy-sink:source"),
			Endpoints: []relation.ImportEndpoint{
				{
					ApplicationName:     "dummy-source",
					EndpointName:        "sink",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
				{
					ApplicationName:     "dummy-sink",
					EndpointName:        "source",
					ApplicationSettings: map[string]any{},
					UnitSettings:        map[string]map[string]any{},
				},
			},
			Scope: charm.ScopeGlobal,
		},
	}).Return(nil)

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.IsNil)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockImportService(ctrl)
	return ctrl
}

func (s *importSuite) expectImportRelations(c *tc.C, data map[int]corerelation.Key, scope charm.RelationScope) description.Model {
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	args := []relation.ImportRelationArg{}
	for id, key := range data {
		rel := model.AddRelation(description.RelationArgs{
			Id:  id,
			Key: key.String(),
		})
		arg := relation.ImportRelationArg{
			ID:    id,
			Key:   key,
			Scope: scope,
		}
		eps := key.EndpointIdentifiers()
		arg.Endpoints = make([]relation.ImportEndpoint, len(eps))
		for j, ep := range eps {
			rel.AddEndpoint(description.EndpointArgs{
				ApplicationName: ep.ApplicationName,
				Name:            ep.EndpointName,
				Role:            string(ep.Role),
				Scope:           string(scope),
			})

			arg.Endpoints[j] = relation.ImportEndpoint{
				ApplicationName:     ep.ApplicationName,
				EndpointName:        ep.EndpointName,
				ApplicationSettings: map[string]any{},
				UnitSettings:        map[string]map[string]any{},
			}
		}
		args = append(args, arg)
	}

	s.service.EXPECT().ImportRelations(gomock.Any(), relationArgMatcher{c: c, expected: args}).Return(nil)
	return model
}

type relationArgMatcher struct {
	c        *tc.C
	expected relation.ImportRelationsArgs
}

func (m relationArgMatcher) Matches(x any) bool {
	obtained, ok := x.(relation.ImportRelationsArgs)
	if !ok {
		return false
	}
	return m.c.Check(obtained, tc.SameContents, m.expected)
}

func (relationArgMatcher) String() string {
	return "matches relation args for import"
}
