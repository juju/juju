// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	importService *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{})
	offerUUID := uuid.MustNewUUID()
	offerArgs := description.ApplicationOfferArgs{
		OfferUUID:       offerUUID.String(),
		OfferName:       "test",
		Endpoints:       map[string]string{"db-admin": "db-admin"},
		ApplicationName: "test",
	}
	app.AddOffer(offerArgs)
	offerUUID2 := uuid.MustNewUUID()
	offerArgs2 := description.ApplicationOfferArgs{
		OfferUUID:       offerUUID2.String(),
		OfferName:       "second",
		Endpoints:       map[string]string{"identity": "identity"},
		ApplicationName: "apple",
	}
	app.AddOffer(offerArgs2)
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            offerUUID,
			Name:            "test",
			ApplicationName: "test",
			Endpoints:       []string{"db-admin"},
		}, {
			UUID:            offerUUID2,
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.importService.EXPECT().ImportOffers(
		gomock.Any(),
		input,
	).Return(nil)

	// Act
	err := s.newImportOperation(c).importOffers(c.Context(), []description.Application{app})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationOfferers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})
	remoteApp := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-mysql",
		OfferUUID:       "offer-uuid-1234",
		URL:             "ctrl:admin/model.mysql",
		SourceModelUUID: "source-model-uuid",
		Macaroon:        "macaroon-data",
		Bindings:        map[string]string{"db": "alpha"},
	})
	remoteApp.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "db",
		Role:      "provider",
		Interface: "mysql",
	})
	relation := model.AddRelation(description.RelationArgs{
		Id:  0,
		Key: "remote-mysql:db foo:db",
	})
	relation.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-mysql",
	})
	relation.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
	})

	expected := []service.RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name:            "remote-mysql",
				OfferUUID:       "offer-uuid-1234",
				URL:             "ctrl:admin/model.mysql",
				SourceModelUUID: "source-model-uuid",
				Macaroon:        "macaroon-data",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "db",
						Role:      charm.RoleProvider,
						Interface: "mysql",
					},
				},
				Units: nil,
			},
		},
	}
	s.importService.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		expected,
	).Return(nil)

	// Act - no relations, so no units to extract
	remoteAppUnits := make(map[string][]string)
	err := s.newImportOperation(c).importRemoteApplicationOfferers(c.Context(),
		model.RemoteApplications(), remoteAppUnits)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationOfferersEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	remoteAppUnits := make(map[string][]string)
	err := s.newImportOperation(c).importRemoteApplicationOfferers(c.Context(),
		model.RemoteApplications(), remoteAppUnits)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationOfferersMultiple(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	remoteApp1 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-mysql",
		OfferUUID:       "offer-uuid-1",
		URL:             "ctrl:admin/model.mysql",
		SourceModelUUID: "source-model-uuid-1",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "db",
		Role:      "provider",
		Interface: "mysql",
	})

	relation1 := model.AddRelation(description.RelationArgs{
		Id:  0,
		Key: "remote-mysql:db foo:db",
	})
	relation1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-mysql",
	})
	relation1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
	})

	remoteApp2 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-postgresql",
		OfferUUID:       "offer-uuid-2",
		URL:             "ctrl:admin/model.postgresql",
		SourceModelUUID: "source-model-uuid-2",
	})
	remoteApp2.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "db",
		Role:      "provider",
		Interface: "pgsql",
	})
	remoteApp2.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "admin",
		Role:      "requirer",
		Interface: "admin",
	})

	relation2 := model.AddRelation(description.RelationArgs{
		Id:  0,
		Key: "remote-postgresql:db foo:db",
	})
	relation2.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-postgresql",
	})
	relation2.AddEndpoint(description.EndpointArgs{
		ApplicationName: "foo",
	})

	expected := []service.RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name:            "remote-mysql",
				OfferUUID:       "offer-uuid-1",
				URL:             "ctrl:admin/model.mysql",
				SourceModelUUID: "source-model-uuid-1",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "db",
						Role:      charm.RoleProvider,
						Interface: "mysql",
					},
				},
			},
		},
		{
			RemoteApplicationImport: service.RemoteApplicationImport{
				Name:            "remote-postgresql",
				OfferUUID:       "offer-uuid-2",
				URL:             "ctrl:admin/model.postgresql",
				SourceModelUUID: "source-model-uuid-2",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "db",
						Role:      charm.RoleProvider,
						Interface: "pgsql",
					},
					{
						Name:      "admin",
						Role:      charm.RoleRequirer,
						Interface: "admin",
					},
				},
			},
		},
	}
	s.importService.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		tc.Bind(tc.SameContents, expected),
	).Return(nil)

	remoteAppUnits := make(map[string][]string)
	err := s.newImportOperation(c).importRemoteApplicationOfferers(c.Context(),
		model.RemoteApplications(), remoteAppUnits)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationsWithUnitsFromRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange - create a model with a remote application and a relation
	// that has unit settings for the remote application
	model := description.NewModel(description.ModelArgs{})

	// Add a local application
	model.AddApplication(description.ApplicationArgs{
		Name: "local-app",
	})

	// Add a remote application
	remoteApp := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-mysql",
		OfferUUID:       "offer-uuid-1234",
		URL:             "ctrl:admin/model.mysql",
		SourceModelUUID: "source-model-uuid",
	})
	remoteApp.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "db",
		Role:      "provider",
		Interface: "mysql",
	})

	// Add a relation between local-app and remote-mysql
	rel := model.AddRelation(description.RelationArgs{
		Id:  1,
		Key: "local-app:database remote-mysql:db",
	})
	// Add endpoint for local app
	localEp := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "local-app",
		Name:            "database",
		Role:            "requirer",
		Interface:       "mysql",
		Scope:           "global",
	})
	localEp.SetUnitSettings("local-app/0", map[string]interface{}{"key": "value"})

	// Add endpoint for remote app with unit settings
	remoteEp := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-mysql",
		Name:            "db",
		Role:            "provider",
		Interface:       "mysql",
		Scope:           "global",
	})
	// These unit settings represent the remote units
	remoteEp.SetUnitSettings("remote-mysql/0", map[string]interface{}{"key": "value1"})
	remoteEp.SetUnitSettings("remote-mysql/1", map[string]interface{}{"key": "value2"})

	// The expected import should include the units extracted from relations
	s.importService.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, imports []service.RemoteApplicationOffererImport) error {
		c.Assert(imports, tc.HasLen, 1)
		c.Check(imports[0].Name, tc.Equals, "remote-mysql")
		// Units should be extracted from relation endpoint settings
		c.Check(imports[0].Units, tc.HasLen, 2)
		c.Check(imports[0].Units, tc.SameContents, []string{"remote-mysql/0", "remote-mysql/1"})
		return nil
	})

	// Act - use Execute which extracts units from relations
	op := s.newImportOperation(c)
	remoteAppUnits := op.extractRemoteAppUnits(model)
	err := op.importRemoteApplicationOfferers(c.Context(), model.RemoteApplications(), remoteAppUnits)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationConsumers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	remoteApp := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-13ea27915e7840d888c5e9451444b45d",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		IsConsumerProxy: true,
		ConsumeVersion:  1,
		// URL is left blank as this is not filled in by the 3.6 model
		// description export, so we have to be sure we can handle this case.
		URL: "",
		// OfferUUID is left blank as this remote application is not associated
		// with an offer in the 3.6 model description export, so we need to
		// ensure we can handle this case.
		OfferUUID: "",
	})
	remoteApp.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "source",
		Role:      "provider",
		Interface: "dummy-token",
	})

	model.AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID:       "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		RelationID:      0,
		RelationKey:     "dummy-source:sink remote-13ea27915e7840d888c5e9451444b45d:source",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		UserName:        "admin",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea27915e7840d888c5e9451444b45d",
		Token: "13ea2791-5e78-40d8-88c5-e9451444b45d",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "applicationoffer-cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		Token: "ec7383b4-7ca1-49de-85d3-1ce8d9cf3d6e",
	})

	rel := model.AddRelation(description.RelationArgs{
		Id:        0,
		Key:       "dummy-source:sink remote-13ea27915e7840d888c5e9451444b45d:source",
		Suspended: false,
	})
	ep0 := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-source",
		Name:            "sink",
		Role:            "requirer",
		Scope:           "global",
		Interface:       "dummy-token",
		Limit:           0,
	})
	ep0.SetUnitSettings("dummy-source/0", map[string]any{
		"egress-subnets":  "10.232.51.213/32",
		"ingress-address": "10.232.51.213",
		"private-address": "10.232.51.213",
		"token":           "foo",
	})
	ep1 := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
		Interface:       "dummy-token",
		Name:            "source",
		Role:            "provider",
		Scope:           "",
		Limit:           0,
	})
	ep1.SetUnitSettings("remote-13ea27915e7840d888c5e9451444b45d/0", map[string]any{
		"egress-subnets":  "10.232.51.148/32",
		"ingress-address": "10.232.51.148",
		"private-address": "10.232.51.148",
	})

	expected := []service.RemoteApplicationConsumerImport{{
		RemoteApplicationImport: service.RemoteApplicationImport{
			Name:      "remote-13ea27915e7840d888c5e9451444b45d",
			OfferUUID: "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
			URL:       "",
			Macaroon:  "",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "source",
					Role:      charm.RoleProvider,
					Interface: "dummy-token",
				},
			},
			Units: []string{"remote-13ea27915e7840d888c5e9451444b45d/0"},
		},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
		RelationKey: relation.Key{
			relation.EndpointIdentifier{
				ApplicationName: "dummy-source",
				EndpointName:    "sink",
				Role:            deploymentcharm.RoleRequirer,
			},
			relation.EndpointIdentifier{
				ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
				EndpointName:    "source",
				Role:            deploymentcharm.RoleProvider,
			},
		},
		ConsumerModelUUID:       "4ddd6454-931d-4278-8779-b0b7208994d9",
		ConsumerApplicationUUID: "13ea2791-5e78-40d8-88c5-e9451444b45d",
		UserName:                "admin",
	}}

	s.importService.EXPECT().ImportRemoteApplicationConsumers(
		gomock.Any(),
		expected,
	).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationConsumersMultipleRemoteApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	remoteApp0 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-13ea27915e7840d888c5e9451444b45d",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		IsConsumerProxy: true,
		ConsumeVersion:  1,
		// URL is left blank as this is not filled in by the 3.6 model
		// description export, so we have to be sure we can handle this case.
		URL: "",
		// OfferUUID is left blank as this remote application is not associated
		// with an offer in the 3.6 model description export, so we need to
		// ensure we can handle this case.
		OfferUUID: "",
	})
	remoteApp0.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "source",
		Role:      "provider",
		Interface: "dummy-token",
	})

	remoteApp1 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-a50f295556314aa4803f766a8802e33a",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		IsConsumerProxy: true,
		ConsumeVersion:  1,
		// URL is left blank as this is not filled in by the 3.6 model
		// description export, so we have to be sure we can handle this case.
		URL: "",
		// OfferUUID is left blank as this remote application is not associated
		// with an offer in the 3.6 model description export, so we need to
		// ensure we can handle this case.
		OfferUUID: "",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "source",
		Role:      "provider",
		Interface: "dummy-token",
	})

	model.AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID:       "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		RelationID:      0,
		RelationKey:     "dummy-source:sink remote-13ea27915e7840d888c5e9451444b45d:source",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		UserName:        "admin",
	})
	model.AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID:       "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		RelationID:      0,
		RelationKey:     "dummy-source:sink remote-a50f295556314aa4803f766a8802e33a:source",
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		UserName:        "admin",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea27915e7840d888c5e9451444b45d",
		Token: "13ea2791-5e78-40d8-88c5-e9451444b45d",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-a50f295556314aa4803f766a8802e33a",
		Token: "a50f2955-5631-4aa4-803f-766a8802e33a",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-a50f295556314aa4803f766a8802e33a.source",
		Token: "ed736d84-0007-438c-8c0e-eac6e0d6dadd",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "applicationoffer-cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		Token: "ec7383b4-7ca1-49de-85d3-1ce8d9cf3d6e",
	})

	rel0 := model.AddRelation(description.RelationArgs{
		Id:        0,
		Key:       "dummy-source:sink remote-13ea27915e7840d888c5e9451444b45d:source",
		Suspended: false,
	})
	rel0ep0 := rel0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-source",
		Name:            "sink",
		Role:            "requirer",
		Scope:           "global",
		Interface:       "dummy-token",
		Limit:           0,
	})
	rel0ep0.SetUnitSettings("dummy-source/0", map[string]any{
		"egress-subnets":  "10.232.51.213/32",
		"ingress-address": "10.232.51.213",
		"private-address": "10.232.51.213",
		"token":           "foo",
	})
	rel0ep1 := rel0.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
		Interface:       "dummy-token",
		Name:            "source",
		Role:            "provider",
		Scope:           "",
		Limit:           0,
	})
	rel0ep1.SetUnitSettings("remote-13ea27915e7840d888c5e9451444b45d/0", map[string]any{
		"egress-subnets":  "10.232.51.148/32",
		"ingress-address": "10.232.51.148",
		"private-address": "10.232.51.148",
	})

	rel1 := model.AddRelation(description.RelationArgs{
		Id:        1,
		Key:       "dummy-source:sink remote-a50f295556314aa4803f766a8802e33a:source",
		Suspended: false,
	})
	rel1ep0 := rel1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "dummy-source",
		Name:            "sink",
		Role:            "requirer",
		Scope:           "global",
		Interface:       "dummy-token",
		Limit:           0,
	})
	rel1ep0.SetUnitSettings("dummy-source/0", map[string]any{
		"egress-subnets":  "10.232.51.213/32",
		"ingress-address": "10.232.51.213",
		"private-address": "10.232.51.213",
		"token":           "foo",
	})
	rel1ep1 := rel1.AddEndpoint(description.EndpointArgs{
		ApplicationName: "remote-a50f295556314aa4803f766a8802e33a",
		Interface:       "dummy-token",
		Name:            "source",
		Role:            "provider",
		Scope:           "",
		Limit:           0,
	})
	rel1ep1.SetUnitSettings("remote-a50f295556314aa4803f766a8802e33a/0", map[string]any{
		"egress-subnets":  "10.232.51.5/32",
		"ingress-address": "10.232.51.5",
		"private-address": "10.232.51.5",
	})

	expected := []service.RemoteApplicationConsumerImport{{
		RemoteApplicationImport: service.RemoteApplicationImport{
			Name:      "remote-13ea27915e7840d888c5e9451444b45d",
			OfferUUID: "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
			URL:       "",
			Macaroon:  "",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "source",
					Role:      charm.RoleProvider,
					Interface: "dummy-token",
				},
			},
			Units: []string{"remote-13ea27915e7840d888c5e9451444b45d/0"},
		},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
		RelationKey: relation.Key{
			relation.EndpointIdentifier{
				ApplicationName: "dummy-source",
				EndpointName:    "sink",
				Role:            deploymentcharm.RoleRequirer,
			},
			relation.EndpointIdentifier{
				ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
				EndpointName:    "source",
				Role:            deploymentcharm.RoleProvider,
			},
		},
		ConsumerModelUUID:       "4ddd6454-931d-4278-8779-b0b7208994d9",
		ConsumerApplicationUUID: "13ea2791-5e78-40d8-88c5-e9451444b45d",
		UserName:                "admin",
	}, {
		RemoteApplicationImport: service.RemoteApplicationImport{
			Name:      "remote-a50f295556314aa4803f766a8802e33a",
			OfferUUID: "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
			URL:       "",
			Macaroon:  "",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "source",
					Role:      charm.RoleProvider,
					Interface: "dummy-token",
				},
			},
			Units: []string{"remote-a50f295556314aa4803f766a8802e33a/0"},
		},
		RelationUUID: "ed736d84-0007-438c-8c0e-eac6e0d6dadd",
		RelationKey: relation.Key{
			relation.EndpointIdentifier{
				ApplicationName: "dummy-source",
				EndpointName:    "sink",
				Role:            deploymentcharm.RoleRequirer,
			},
			relation.EndpointIdentifier{
				ApplicationName: "remote-a50f295556314aa4803f766a8802e33a",
				EndpointName:    "source",
				Role:            deploymentcharm.RoleProvider,
			},
		},
		ConsumerModelUUID:       "4ddd6454-931d-4278-8779-b0b7208994d9",
		ConsumerApplicationUUID: "a50f2955-5631-4aa4-803f-766a8802e33a",
		UserName:                "admin",
	}}

	s.importService.EXPECT().ImportRemoteApplicationConsumers(
		gomock.Any(),
		expected,
	).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestExtractRelationUUIDFromRemoteEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := extractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, []relationRemoteEntity{{
		RelationKey: relation.Key{{
			ApplicationName: "dummy-source",
			EndpointName:    "sink",
			Role:            deploymentcharm.RoleRequirer,
		}, {
			ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
			EndpointName:    "source",
			Role:            deploymentcharm.RoleProvider,
		}},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}})
}

func (s *importSuite) TestExtractRelationUUIDFromRemoteEntitiesWithApplicationEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea2791-5e78-40d8-88c5-e9451444b45d",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := extractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, []relationRemoteEntity{{
		RelationKey: relation.Key{{
			ApplicationName: "dummy-source",
			EndpointName:    "sink",
			Role:            deploymentcharm.RoleRequirer,
		}, {
			ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d",
			EndpointName:    "source",
			Role:            deploymentcharm.RoleProvider,
		}},
		RelationUUID: "6049aa01-76c9-462d-8440-964a6e26aac2",
	}})
}

func (s *importSuite) TestExtractRelationUUIDFromRemoteEntitiesNoEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	entities, err := extractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entities, tc.HasLen, 0)
}

func (s *importSuite) TestExtractApplicationUUIDFromRemoteEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea2791-5e78-40d8-88c5-e9451444b45d",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := extractApplicationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, map[string]string{
		"remote-13ea2791-5e78-40d8-88c5-e9451444b45d": "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
}

func (s *importSuite) TestExtractApplicationUUIDFromRemoteEntitiesWithRelationEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "application-remote-13ea2791-5e78-40d8-88c5-e9451444b45d",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := extractApplicationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(entities, tc.DeepEquals, map[string]string{
		"remote-13ea2791-5e78-40d8-88c5-e9451444b45d": "6049aa01-76c9-462d-8440-964a6e26aac2",
	})
}

func (s *importSuite) TestExtractApplicationUUIDFromRemoteEntitiesNoEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	entities, err := extractApplicationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(entities, tc.HasLen, 0)
}

func (s *importSuite) TestFindRelationUUIDForKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-dummy-source.sink#remote-13ea27915e7840d888c5e9451444b45d.source",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := extractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := findRelationUUIDForKey(entities, relation.Key{
		{ApplicationName: "dummy-source", EndpointName: "sink"},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")
}

func (s *importSuite) TestFindRelationUUIDForKeyIgnoresOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	model.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-remote-13ea27915e7840d888c5e9451444b45d.source#dummy-source.sink",
		Token: "6049aa01-76c9-462d-8440-964a6e26aac2",
	})

	entities, err := extractRelationUUIDFromRemoteEntities(model)
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := findRelationUUIDForKey(entities, relation.Key{
		{ApplicationName: "dummy-source", EndpointName: "sink"},
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")

	uuid, err = findRelationUUIDForKey(entities, relation.Key{
		{ApplicationName: "remote-13ea27915e7840d888c5e9451444b45d", EndpointName: "source"},
		{ApplicationName: "dummy-source", EndpointName: "sink"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
	})

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		importService: s.importService,
		clock:         clock.WallClock,
		logger:        loggertesting.WrapCheckLog(c),
	}
}
