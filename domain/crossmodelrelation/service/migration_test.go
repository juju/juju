// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	modelMigrationState *MockModelMigrationState
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestImportOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            uuid.MustNewUUID(),
			Name:            "test",
			ApplicationName: "test",
			Endpoints:       []string{"db-admin"},
		}, {
			UUID:            uuid.MustNewUUID(),
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.modelMigrationState.EXPECT().ImportOffers(gomock.Any(), input).Return(nil)

	// Act
	err := s.service(c).ImportOffers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportOffersFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            uuid.MustNewUUID(),
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.modelMigrationState.EXPECT().ImportOffers(gomock.Any(), input).Return(applicationerrors.ApplicationNotFound)

	// Act
	err := s.service(c).ImportOffers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationSuite) TestImportRemoteApplicationOfferers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
				Name:            "remote-app1",
				OfferUUID:       uuid.MustNewUUID().String(),
				URL:             "ctrl:admin/model.app1",
				SourceModelUUID: uuid.MustNewUUID().String(),
				Macaroon:        "macaroon-data",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "db",
						Role:      "provider",
						Interface: "mysql",
					},
				},
			},
		},
		{
			RemoteApplicationImport: RemoteApplicationImport{
				Name:            "remote-app2",
				OfferUUID:       uuid.MustNewUUID().String(),
				URL:             "ctrl:admin/model.app2",
				SourceModelUUID: uuid.MustNewUUID().String(),
				Macaroon:        "macaroon-data-2",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "endpoint",
						Role:      "requirer",
						Interface: "http",
					},
				},
			},
		},
	}
	// Verify the service builds synthetic charms correctly
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: transform.Slice(input, func(v RemoteApplicationOffererImport) RemoteApplicationImport {
				return v.RemoteApplicationImport
			}),
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplicationOfferers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationOfferersEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []RemoteApplicationOffererImport{}
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: transform.Slice(input, func(v RemoteApplicationOffererImport) RemoteApplicationImport {
				return v.RemoteApplicationImport
			}),
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplicationOfferers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationOfferersFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
				Name:            "remote-app",
				OfferUUID:       uuid.MustNewUUID().String(),
				URL:             "ctrl:admin/model.app",
				SourceModelUUID: uuid.MustNewUUID().String(),
				Macaroon:        "macaroon-data",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "db",
						Role:      "provider",
						Interface: "mysql",
					},
				},
			},
		},
	}
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: transform.Slice(input, func(v RemoteApplicationOffererImport) RemoteApplicationImport {
				return v.RemoteApplicationImport
			}),
		},
	).Return(applicationerrors.ApplicationNotFound)

	// Act
	err := s.service(c).ImportRemoteApplicationOfferers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationSuite) TestImportRemoteApplicationOfferersPeerIgnored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange - import with a peer endpoint to verify it's ignored in synthetic
	// charm
	input := []RemoteApplicationOffererImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
				Name:            "remote-app",
				OfferUUID:       uuid.MustNewUUID().String(),
				URL:             "ctrl:admin/model.app",
				SourceModelUUID: uuid.MustNewUUID().String(),
				Macaroon:        "macaroon-data",
				Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
					{
						Name:      "provider-ep",
						Role:      "provider",
						Interface: "http",
					},
					{
						Name:      "peer-ep",
						Role:      "peer",
						Interface: "cluster",
					},
					{
						Name:      "requirer-ep",
						Role:      "requirer",
						Interface: "db",
					},
				},
			},
		},
	}
	// Verify the synthetic charm excludes peer endpoints
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: transform.Slice(input, func(v RemoteApplicationOffererImport) RemoteApplicationImport {
				return v.RemoteApplicationImport
			}),
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplicationOfferers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationConsumers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
		},
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
		},
	}

	offererAppUUID := tc.Must0(c, coreapplication.NewUUID).String()
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), "dummy-source").
		Return(offererAppUUID, nil).Times(2)

	var got []crossmodelrelation.RemoteApplicationConsumerImport
	s.modelMigrationState.EXPECT().ImportRemoteApplicationConsumers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: transform.Slice(input, func(v RemoteApplicationConsumerImport) RemoteApplicationImport {
				return v.RemoteApplicationImport
			}),
		},
	).DoAndReturn(func(ctx context.Context, raci []crossmodelrelation.RemoteApplicationConsumerImport) error {
		got = raci
		return nil
	})

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 2)

	c.Check(got[0].RelationUUID, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac2")
	c.Check(got[0].ConsumerApplicationUUID, tc.Equals, "13ea2791-5e78-40d8-88c5-e9451444b45d")
	c.Check(got[0].OffererApplicationUUID, tc.Equals, offererAppUUID)
	c.Check(got[0].ConsumerApplicationEndpoint, tc.Equals, "source")
	c.Check(got[0].OffererApplicationEndpoint, tc.Equals, "sink")
	c.Check(got[0].UserName, tc.Equals, "admin")

	c.Check(got[1].RelationUUID, tc.Equals, "ed736d84-0007-438c-8c0e-eac6e0d6dadd")
	c.Check(got[1].ConsumerApplicationUUID, tc.Equals, "a50f2955-5631-4aa4-803f-766a8802e33a")
	c.Check(got[1].OffererApplicationUUID, tc.Equals, offererAppUUID)
	c.Check(got[1].ConsumerApplicationEndpoint, tc.Equals, "source")
	c.Check(got[1].OffererApplicationEndpoint, tc.Equals, "sink")
	c.Check(got[1].UserName, tc.Equals, "admin")
}

func (s *migrationSuite) TestImportRemoteApplicationConsumersApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
		},
	}

	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), "dummy-source").
		Return(tc.Must0(c, coreapplication.NewUUID).String(), errors.Errorf("boom"))

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *migrationSuite) TestImportRemoteApplicationConsumerInvalidRelationKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
			},
			ConsumerModelUUID:       "4ddd6454-931d-4278-8779-b0b7208994d9",
			ConsumerApplicationUUID: "13ea2791-5e78-40d8-88c5-e9451444b45d",
			UserName:                "admin",
		},
	}

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationSuite) TestImportRemoteApplicationConsumerInvalidRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
			RelationUUID: "!!6049aa01-76c9-462d-8440-964a6e26aac2",
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
		},
	}

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationSuite) TestImportRemoteApplicationConsumerInvalidOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
				Name:      "remote-13ea27915e7840d888c5e9451444b45d",
				OfferUUID: "!!cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
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
		},
	}

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationSuite) TestImportRemoteApplicationConsumerInvalidConsumerModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
			ConsumerModelUUID:       "!!4ddd6454-931d-4278-8779-b0b7208994d9",
			ConsumerApplicationUUID: "13ea2791-5e78-40d8-88c5-e9451444b45d",
			UserName:                "admin",
		},
	}

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationSuite) TestImportRemoteApplicationConsumerInvalidConsumerApplicationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		{
			RemoteApplicationImport: RemoteApplicationImport{
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
			ConsumerApplicationUUID: "!!13ea2791-5e78-40d8-88c5-e9451444b45d",
			UserName:                "admin",
		},
	}

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// syntheticCharmMatcher is a custom gomock matcher that verifies
// RemoteApplicationImport slices have correctly built synthetic charms.
type syntheticCharmMatcher struct {
	expectedApps []RemoteApplicationImport
}

func (m syntheticCharmMatcher) Matches(x interface{}) bool {
	var actual []crossmodelrelation.RemoteApplicationImport
	switch v := x.(type) {
	case []crossmodelrelation.RemoteApplicationOffererImport:
		actual = transform.Slice(v, func(v crossmodelrelation.RemoteApplicationOffererImport) crossmodelrelation.RemoteApplicationImport {
			return v.RemoteApplicationImport
		})
	case []crossmodelrelation.RemoteApplicationConsumerImport:
		actual = transform.Slice(v, func(v crossmodelrelation.RemoteApplicationConsumerImport) crossmodelrelation.RemoteApplicationImport {
			return v.RemoteApplicationImport
		})
	default:
		return false
	}

	if len(actual) != len(m.expectedApps) {
		return false
	}

	for i, app := range actual {
		expected := m.expectedApps[i]

		// Verify basic fields match
		if app.Name != expected.Name ||
			app.OfferUUID != expected.OfferUUID ||
			app.URL != expected.URL ||
			app.SourceModelUUID != expected.SourceModelUUID ||
			app.Macaroon != expected.Macaroon {
			return false
		}

		// Verify synthetic charm was built correctly
		if app.SyntheticCharm.Metadata.Name != app.Name {
			return false
		}
		if app.SyntheticCharm.Source != charm.CMRSource {
			return false
		}
		if app.SyntheticCharm.ReferenceName != app.Name {
			return false
		}

		// Verify charm endpoints match input endpoints
		for _, ep := range expected.Endpoints {
			switch ep.Role {
			case "provider":
				rel, ok := app.SyntheticCharm.Metadata.Provides[ep.Name]
				if !ok || rel.Interface != ep.Interface {
					return false
				}
			case "requirer":
				rel, ok := app.SyntheticCharm.Metadata.Requires[ep.Name]
				if !ok || rel.Interface != ep.Interface {
					return false
				}
			case "peer":
				// Peer relations should not be in synthetic charm
				if _, inProvides := app.SyntheticCharm.Metadata.Provides[ep.Name]; inProvides {
					return false
				}
				if _, inRequires := app.SyntheticCharm.Metadata.Requires[ep.Name]; inRequires {
					return false
				}
			}
		}
	}

	return true
}

func (m syntheticCharmMatcher) String() string {
	return "matches RemoteApplicationImport with correctly built synthetic charms"
}

func (s *migrationSuite) service(c *tc.C) *MigrationService {
	return &MigrationService{
		modelState: s.modelMigrationState,
		logger:     loggertesting.WrapCheckLog(c),
	}
}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelMigrationState = NewMockModelMigrationState(ctrl)

	c.Cleanup(func() {
		s.modelMigrationState = nil
	})
	return ctrl
}
