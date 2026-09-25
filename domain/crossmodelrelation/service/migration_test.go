// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	relationerrors "github.com/juju/juju/domain/relation/errors"
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
	appUUID1 := tc.Must(c, coreapplication.NewUUID)
	appUUID2 := tc.Must(c, coreapplication.NewUUID)
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
			OffererApplicationUUID: appUUID1,
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
			OffererApplicationUUID: appUUID2,
		},
	}
	// Verify the service builds synthetic charms correctly

	var appUUIDs []coreapplication.UUID
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: transform.Slice(input, func(v RemoteApplicationOffererImport) RemoteApplicationImport {
				appUUIDs = append(appUUIDs, v.OffererApplicationUUID)
				return v.RemoteApplicationImport
			}),
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplicationOfferers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(appUUIDs, tc.HasLen, 2)
	c.Check(appUUIDs, tc.SameContents, []coreapplication.UUID{appUUID1, appUUID2})
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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
			RelationUUID:  "ed736d84-0007-438c-8c0e-eac6e0d6dadd",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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
	c.Check(got[0].SyntheticApplicationUUID, tc.Equals, "13ea2791-5e78-40d8-88c5-e9451444b45d")
	c.Check(got[0].OffererApplicationUUID, tc.Equals, offererAppUUID)
	c.Check(got[0].ConsumerApplicationEndpoint, tc.Equals, "source")
	c.Check(got[0].OffererApplicationEndpoint, tc.Equals, "sink")
	c.Check(got[0].UserName, tc.Equals, "admin")

	c.Check(got[1].RelationUUID, tc.Equals, "ed736d84-0007-438c-8c0e-eac6e0d6dadd")
	c.Check(got[1].ConsumerApplicationUUID, tc.Equals, "a50f2955-5631-4aa4-803f-766a8802e33a")
	c.Check(got[1].SyntheticApplicationUUID, tc.Equals, "a50f2955-5631-4aa4-803f-766a8802e33a")
	c.Check(got[1].OffererApplicationUUID, tc.Equals, offererAppUUID)
	c.Check(got[1].ConsumerApplicationEndpoint, tc.Equals, "source")
	c.Check(got[1].OffererApplicationEndpoint, tc.Equals, "sink")
	c.Check(got[1].UserName, tc.Equals, "admin")
}

const (
	multiOfferConnProxyName = "remote-13ea27915e7840d888c5e9451444b45d"
	// multiOfferConnConsumerAppUUID is the consuming application UUID
	// shared by every offer connection of the proxy.
	multiOfferConnConsumerAppUUID = "13ea2791-5e78-40d8-88c5-e9451444b45d"
)

// newMultiOfferConnectionImport returns an import entry for one offer
// connection of the legacy consumer proxy shared by the multiple offer
// connection tests.
func newMultiOfferConnectionImport(c *tc.C, offererName, offerUUID, relationUUID string) RemoteApplicationConsumerImport {
	key, err := relation.NewKeyFromString(offererName + ":db " + multiOfferConnProxyName + ":db")
	c.Assert(err, tc.ErrorIsNil)
	return RemoteApplicationConsumerImport{
		RemoteApplicationImport: RemoteApplicationImport{
			Name:      multiOfferConnProxyName,
			OfferUUID: offerUUID,
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "db",
					Role:      charm.RoleRequirer,
					Interface: "db",
				},
			},
			Units: []string{multiOfferConnProxyName + "/0"},
		},
		RelationUUID:            relationUUID,
		RelationID:              41,
		RelationScope:           charm.ScopeGlobal,
		RelationKey:             key,
		ConsumerModelUUID:       "4ddd6454-931d-4278-8779-b0b7208994d9",
		ConsumerApplicationUUID: multiOfferConnConsumerAppUUID,
		UserName:                "admin",
	}
}

// TestImportRemoteApplicationConsumersMultipleOfferConnections imports the
// offer connections of a single legacy consumer proxy that relates to two
// offered applications. The first connection keeps the identity of the
// legacy proxy, and every additional connection is represented by a fresh
// synthetic application, as the model holds one application per offer
// connection.
func (s *migrationSuite) TestImportRemoteApplicationConsumersMultipleOfferConnections(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		newMultiOfferConnectionImport(c, "mysql", "cfa46843-ebf2-4fff-8519-c1fb5a9816f0", "6049aa01-76c9-462d-8440-964a6e26aac0"),
		newMultiOfferConnectionImport(c, "postgres", "cfa46843-ebf2-4fff-8519-c1fb5a9816f1", "6049aa01-76c9-462d-8440-964a6e26aac1"),
	}

	offererAppUUID := tc.Must0(c, coreapplication.NewUUID).String()
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), "mysql").
		Return(offererAppUUID, nil)
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), "postgres").
		Return(offererAppUUID, nil)

	var got []crossmodelrelation.RemoteApplicationConsumerImport
	s.modelMigrationState.EXPECT().ImportRemoteApplicationConsumers(
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, raci []crossmodelrelation.RemoteApplicationConsumerImport) error {
		got = raci
		return nil
	})

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(got, tc.HasLen, 2)

	// The first connection keeps the identity of the legacy consumer
	// proxy, along with its synthetic units.
	first, second := got[0], got[1]
	c.Check(first.Name, tc.Equals, multiOfferConnProxyName)
	c.Check(first.SyntheticApplicationUUID, tc.Equals, multiOfferConnConsumerAppUUID)
	c.Check(first.SyntheticCharm.Metadata.Name, tc.Equals, multiOfferConnProxyName)
	c.Check(first.Units, tc.DeepEquals, []string{multiOfferConnProxyName + "/0"})
	c.Check(first.ConsumerApplicationUUID, tc.Equals, multiOfferConnConsumerAppUUID)
	c.Check(first.ConsumerApplicationEndpoint, tc.Equals, "db")
	c.Check(first.OffererApplicationEndpoint, tc.Equals, "db")
	c.Check(first.OfferUUID, tc.Equals, "cfa46843-ebf2-4fff-8519-c1fb5a9816f0")
	c.Check(first.RelationUUID, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac0")

	// The additional connection is represented by a fresh synthetic
	// application, with no synthetic units of its own.
	c.Check(second.SyntheticApplicationUUID != multiOfferConnConsumerAppUUID, tc.IsTrue)
	c.Check(coreapplication.UUID(second.SyntheticApplicationUUID).Validate(), tc.ErrorIsNil)
	c.Check(second.Name, tc.Equals,
		coreapplication.RemoteApplicationNameFromUUID(coreapplication.UUID(second.SyntheticApplicationUUID)))
	c.Check(second.SyntheticCharm.Metadata.Name, tc.Equals, second.Name)
	c.Check(second.Units, tc.HasLen, 0)
	c.Check(second.ConsumerApplicationUUID, tc.Equals, multiOfferConnConsumerAppUUID)
	c.Check(second.ConsumerApplicationEndpoint, tc.Equals, "db")
	c.Check(second.OffererApplicationEndpoint, tc.Equals, "db")
	c.Check(second.OfferUUID, tc.Equals, "cfa46843-ebf2-4fff-8519-c1fb5a9816f1")
	c.Check(second.RelationUUID, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac1")
}

// TestImportRemoteApplicationConsumersMultipleOfferConnectionsReverseOrder
// pins that the connection keeping the identity of the legacy proxy is the
// first entry of the imports, regardless of which offer it belongs to.
func (s *migrationSuite) TestImportRemoteApplicationConsumersMultipleOfferConnectionsReverseOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()

	input := []RemoteApplicationConsumerImport{
		newMultiOfferConnectionImport(c, "postgres", "cfa46843-ebf2-4fff-8519-c1fb5a9816f1", "6049aa01-76c9-462d-8440-964a6e26aac1"),
		newMultiOfferConnectionImport(c, "mysql", "cfa46843-ebf2-4fff-8519-c1fb5a9816f0", "6049aa01-76c9-462d-8440-964a6e26aac0"),
	}

	offererAppUUID := tc.Must0(c, coreapplication.NewUUID).String()
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), "postgres").
		Return(offererAppUUID, nil)
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), "mysql").
		Return(offererAppUUID, nil)

	var got []crossmodelrelation.RemoteApplicationConsumerImport
	s.modelMigrationState.EXPECT().ImportRemoteApplicationConsumers(
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, raci []crossmodelrelation.RemoteApplicationConsumerImport) error {
		got = raci
		return nil
	})

	err := s.service(c).ImportRemoteApplicationConsumers(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(got, tc.HasLen, 2)

	// The first connection keeps the identity of the legacy consumer
	// proxy, along with its synthetic units.
	first, second := got[0], got[1]
	c.Check(first.Name, tc.Equals, multiOfferConnProxyName)
	c.Check(first.SyntheticApplicationUUID, tc.Equals, multiOfferConnConsumerAppUUID)
	c.Check(first.Units, tc.DeepEquals, []string{multiOfferConnProxyName + "/0"})
	c.Check(first.OfferUUID, tc.Equals, "cfa46843-ebf2-4fff-8519-c1fb5a9816f1")
	c.Check(first.RelationUUID, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac1")

	// The additional connection is represented by a fresh synthetic
	// application, with no synthetic units of its own.
	c.Check(second.SyntheticApplicationUUID != multiOfferConnConsumerAppUUID, tc.IsTrue)
	c.Check(second.Name, tc.Equals,
		coreapplication.RemoteApplicationNameFromUUID(coreapplication.UUID(second.SyntheticApplicationUUID)))
	c.Check(second.Units, tc.HasLen, 0)
	c.Check(second.OfferUUID, tc.Equals, "cfa46843-ebf2-4fff-8519-c1fb5a9816f0")
	c.Check(second.RelationUUID, tc.Equals, "6049aa01-76c9-462d-8440-964a6e26aac0")
}

func (s *migrationSuite) TestImportRelationNetworks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	key, err := relation.NewKeyFromString("mysql:db remote-13ea:db")
	c.Assert(err, tc.ErrorIsNil)

	input := []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkIngress,
			CIDRs:       []string{"10.0.0.0/24"},
		}, {
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkEgress,
			CIDRs:       []string{"192.168.0.0/16", "192.168.1.0/24"},
		},
	}

	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), key).
		Return("6049aa01-76c9-462d-8440-964a6e26aac2", nil).Times(2)
	s.modelMigrationState.EXPECT().AddRelationNetworkIngress(
		gomock.Any(), "6049aa01-76c9-462d-8440-964a6e26aac2", []string{"10.0.0.0/24"}).Return(nil)
	s.modelMigrationState.EXPECT().AddRelationNetworkEgress(
		gomock.Any(), "6049aa01-76c9-462d-8440-964a6e26aac2", []string{"192.168.0.0/16", "192.168.1.0/24"}).Return(nil)

	// Act
	err = s.service(c).ImportRelationNetworks(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRelationNetworksInvalidCIDR(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	key, err := relation.NewKeyFromString("mysql:db remote-13ea:db")
	c.Assert(err, tc.ErrorIsNil)

	input := []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkIngress,
			CIDRs:       []string{"not-a-cidr"},
		},
	}

	// Act
	err = s.service(c).ImportRelationNetworks(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*validating CIDR "not-a-cidr".*`)
}

func (s *migrationSuite) TestImportRelationNetworksUnknownDirection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	key, err := relation.NewKeyFromString("mysql:db remote-13ea:db")
	c.Assert(err, tc.ErrorIsNil)

	input := []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkDirection("invalid"),
			CIDRs:       []string{"10.0.0.0/24"},
		},
	}

	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), key).
		Return("6049aa01-76c9-462d-8440-964a6e26aac2", nil)

	// Act
	err = s.service(c).ImportRelationNetworks(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*unknown relation network direction "invalid".*`)
}

func (s *migrationSuite) TestImportRelationNetworksRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	key, err := relation.NewKeyFromString("mysql:db remote-13ea:db")
	c.Assert(err, tc.ErrorIsNil)
	otherKey, err := relation.NewKeyFromString("wordpress:db remote-13ea:db")
	c.Assert(err, tc.ErrorIsNil)

	input := []crossmodelrelation.RelationNetworkImport{
		{
			RelationKey: key,
			Direction:   crossmodelrelation.RelationNetworkIngress,
			CIDRs:       []string{"10.0.0.0/24"},
		}, {
			RelationKey: otherKey,
			Direction:   crossmodelrelation.RelationNetworkIngress,
			CIDRs:       []string{"192.0.2.0/24"},
		},
	}

	// The first relation was not migrated, so its networks are skipped and
	// the remaining networks are still imported.
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), key).
		Return("", relationerrors.RelationNotFound)
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), otherKey).
		Return("ed736d84-0007-438c-8c0e-eac6e0d6dadd", nil)
	s.modelMigrationState.EXPECT().AddRelationNetworkIngress(
		gomock.Any(), "ed736d84-0007-438c-8c0e-eac6e0d6dadd", []string{"192.0.2.0/24"}).Return(nil)

	// Act
	err = s.service(c).ImportRelationNetworks(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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
			RelationUUID:  "!!6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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

func (s *migrationSuite) TestImportRemoteApplicationConsumerNegativeRelationID(c *tc.C) {
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    -1,
			RelationScope: charm.ScopeGlobal,
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
	c.Assert(err, tc.ErrorMatches, ".*validating relation ID -1.*")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationSuite) TestImportRemoteApplicationConsumerInvalidRelationScope(c *tc.C) {
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.RelationScope("bogus"),
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
	c.Assert(err, tc.ErrorMatches, `.*validating relation scope "bogus".*`)
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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
			RelationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
			RelationID:    0,
			RelationScope: charm.ScopeGlobal,
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

func (s *migrationSuite) TestImportGrantedSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	secretID := "secret-id"
	appName := "app"
	appUUID := uuid.MustNewUUID().String()
	relKey := relation.Key{
		{
			ApplicationName: appName,
			EndpointName:    "endpoint",
			Role:            deploymentcharm.RoleProvider,
		},
	}
	relUUID := uuid.MustNewUUID().String()
	unitName := unit.Name("app/0")

	input := []GrantedSecretImport{
		{
			SecretID: secretID,
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					RelationKey:     relKey,
					Role:            secrets.RoleView,
				},
			},
			Consumers: []GrantedSecretConsumerImport{
				{
					Unit:            unitName,
					CurrentRevision: 1,
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), relKey).Return(relUUID, nil)
	s.modelMigrationState.EXPECT().ImportRemoteApplicationSecretGrants(gomock.Any(), []internal.RemoteApplicationSecretGrant{
		{
			SecretID:        secretID,
			ApplicationName: appName,
			ApplicationUUID: appUUID,
			RelationKey:     relKey.String(),
			RelationUUID:    relUUID,
		},
	}).Return(nil)
	s.modelMigrationState.EXPECT().ImportRemoteSecretConsumers(gomock.Any(), []internal.RemoteUnitConsumer{
		{
			SecretID:        secretID,
			Unit:            unitName.String(),
			CurrentRevision: 1,
		},
	}).Return(nil)

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportGrantedSecretsUnsupportedRole(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []GrantedSecretImport{
		{
			SecretID: "secret-id",
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: "app",
					Role:            secrets.RoleManage, // Unsupported
				},
			},
		},
	}

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*unsupported role "manage" for remote secret "secret-id"`)
}

func (s *migrationSuite) TestImportGrantedSecretsConsumerGrantNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appName := "app"
	otherAppName := "other"
	input := []GrantedSecretImport{
		{
			SecretID: "secret-id",
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					Role:            secrets.RoleView,
				},
			},
			Consumers: []GrantedSecretConsumerImport{
				{
					Unit: unit.Name(otherAppName + "/0"),
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(uuid.MustNewUUID().String(), nil)
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), gomock.Any()).Return(uuid.MustNewUUID().String(), nil)

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*grant for application "other" not found for remote secret "secret-id"`)
}

func (s *migrationSuite) TestImportGrantedSecretsGetApplicationUUIDByNameFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appName := "app"
	input := []GrantedSecretImport{
		{
			SecretID: "secret-id",
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					Role:            secrets.RoleView,
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), gomock.Any()).
		Return(uuid.MustNewUUID().String(), nil)
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return("", errors.Errorf("boom"))

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*getting application UUID by name \"app\": boom")
}

func (s *migrationSuite) TestImportGrantedSecretsImportGrantsFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appName := "app"
	input := []GrantedSecretImport{
		{
			SecretID: "secret-id",
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					Role:            secrets.RoleView,
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return("uuid", nil)
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), gomock.Any()).Return("rel-uuid", nil)
	s.modelMigrationState.EXPECT().ImportRemoteApplicationSecretGrants(gomock.Any(), gomock.Any()).Return(errors.Errorf("boom"))

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

// TestImportGrantedSecretsSkipsUnresolvedRelation checks that a grant
// scoped by a relation that was not migrated under its legacy key (for
// example a relation of a legacy consumer proxy that was re-keyed on
// import) is skipped with a warning, while the remaining grants and the
// consumers of resolved grants are still imported.
func (s *migrationSuite) TestImportGrantedSecretsSkipsUnresolvedRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	secretID := "secret-id"
	appName := "app"
	appUUID := uuid.MustNewUUID().String()
	resolvedKey := relation.Key{
		{
			ApplicationName: appName,
			EndpointName:    "endpoint",
			Role:            deploymentcharm.RoleProvider,
		},
	}
	unresolvedKey := relation.Key{
		{
			ApplicationName: appName,
			EndpointName:    "other-endpoint",
			Role:            deploymentcharm.RoleProvider,
		},
	}
	resolvedRelUUID := uuid.MustNewUUID().String()
	unitName := unit.Name(appName + "/0")

	input := []GrantedSecretImport{
		{
			SecretID: secretID,
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					RelationKey:     resolvedKey,
					Role:            secrets.RoleView,
				},
				{
					ApplicationName: appName,
					RelationKey:     unresolvedKey,
					Role:            secrets.RoleView,
				},
			},
			Consumers: []GrantedSecretConsumerImport{
				{
					Unit:            unitName,
					CurrentRevision: 1,
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), resolvedKey).
		Return(resolvedRelUUID, nil)
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), unresolvedKey).
		Return("", relationerrors.RelationNotFound)
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.modelMigrationState.EXPECT().ImportRemoteApplicationSecretGrants(gomock.Any(), []internal.RemoteApplicationSecretGrant{
		{
			SecretID:        secretID,
			ApplicationName: appName,
			ApplicationUUID: appUUID,
			RelationKey:     resolvedKey.String(),
			RelationUUID:    resolvedRelUUID,
		},
	}).Return(nil)
	s.modelMigrationState.EXPECT().ImportRemoteSecretConsumers(gomock.Any(), []internal.RemoteUnitConsumer{
		{
			SecretID:        secretID,
			Unit:            unitName.String(),
			CurrentRevision: 1,
		},
	}).Return(nil)

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportGrantedSecretsSkipsConsumersWhenGrantSkipped checks that the
// consumers of an application whose only grant was skipped are skipped as
// well, instead of failing the migration.
func (s *migrationSuite) TestImportGrantedSecretsSkipsConsumersWhenGrantSkipped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appName := "app"
	input := []GrantedSecretImport{
		{
			SecretID: "secret-id",
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					RelationKey: relation.Key{
						{
							ApplicationName: appName,
							EndpointName:    "endpoint",
							Role:            deploymentcharm.RoleProvider,
						},
					},
					Role: secrets.RoleView,
				},
			},
			Consumers: []GrantedSecretConsumerImport{
				{
					Unit:            unit.Name(appName + "/0"),
					CurrentRevision: 1,
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), gomock.Any()).
		Return("", relationerrors.RelationNotFound)

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportGrantedSecretsKeepsAllGrantsForSameApplication checks that
// grants of the same application scoped by different relations are all
// imported, instead of the last one overwriting the others.
func (s *migrationSuite) TestImportGrantedSecretsKeepsAllGrantsForSameApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	secretID := "secret-id"
	appName := "app"
	appUUID := uuid.MustNewUUID().String()
	firstKey := relation.Key{
		{
			ApplicationName: appName,
			EndpointName:    "endpoint",
			Role:            deploymentcharm.RoleProvider,
		},
	}
	secondKey := relation.Key{
		{
			ApplicationName: appName,
			EndpointName:    "other-endpoint",
			Role:            deploymentcharm.RoleProvider,
		},
	}
	firstRelUUID := uuid.MustNewUUID().String()
	secondRelUUID := uuid.MustNewUUID().String()

	input := []GrantedSecretImport{
		{
			SecretID: secretID,
			ACLs: []GrantedSecretACLImport{
				{
					ApplicationName: appName,
					RelationKey:     firstKey,
					Role:            secrets.RoleView,
				},
				{
					ApplicationName: appName,
					RelationKey:     secondKey,
					Role:            secrets.RoleView,
				},
			},
		},
	}

	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), firstKey).
		Return(firstRelUUID, nil)
	s.modelMigrationState.EXPECT().GetRelationUUIDByRelationKey(gomock.Any(), secondKey).
		Return(secondRelUUID, nil)
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).
		Return(appUUID, nil)
	s.modelMigrationState.EXPECT().GetApplicationUUIDByName(gomock.Any(), appName).
		Return(appUUID, nil)
	s.modelMigrationState.EXPECT().ImportRemoteApplicationSecretGrants(gomock.Any(), []internal.RemoteApplicationSecretGrant{
		{
			SecretID:        secretID,
			ApplicationName: appName,
			ApplicationUUID: appUUID,
			RelationKey:     firstKey.String(),
			RelationUUID:    firstRelUUID,
		},
		{
			SecretID:        secretID,
			ApplicationName: appName,
			ApplicationUUID: appUUID,
			RelationKey:     secondKey.String(),
			RelationUUID:    secondRelUUID,
		},
	}).Return(nil)

	// Act
	err := s.service(c).ImportGrantedSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	secretID := "secret-id"
	sourceUUID := uuid.MustNewUUID().String()
	unitName := unit.Name("app/0")
	unitUUID := uuid.MustNewUUID().String()

	input := []RemoteSecretImport{
		{
			SecretID:        secretID,
			SourceUUID:      sourceUUID,
			Label:           "label",
			ConsumerUnit:    unitName,
			CurrentRevision: 1,
			LatestRevision:  2,
		},
	}

	s.modelMigrationState.EXPECT().GetUnitUUID(gomock.Any(), unitName.String()).Return(unitUUID, nil)
	s.modelMigrationState.EXPECT().ImportRemoteSecret(gomock.Any(), internal.RemoteSecret{
		SecretID:        secretID,
		SourceModelUUID: sourceUUID,
		UnitUUID:        unitUUID,
		Label:           "label",
		CurrentRevision: 1,
		LatestRevision:  2,
	}).Return(nil)

	// Act
	err := s.service(c).ImportRemoteSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteSecretsFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("app/0")
	input := []RemoteSecretImport{
		{
			SecretID:     "secret-id",
			ConsumerUnit: unitName,
		},
	}

	s.modelMigrationState.EXPECT().GetUnitUUID(gomock.Any(), unitName.String()).Return("", errors.Errorf("not found"))

	// Act
	err := s.service(c).ImportRemoteSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*not found")
}

func (s *migrationSuite) TestImportRemoteSecretsImportFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := unit.Name("app/0")
	input := []RemoteSecretImport{
		{
			SecretID:     "secret-id",
			ConsumerUnit: unitName,
		},
	}

	s.modelMigrationState.EXPECT().GetUnitUUID(gomock.Any(), unitName.String()).Return("unit-uuid", nil)
	s.modelMigrationState.EXPECT().ImportRemoteSecret(gomock.Any(), gomock.Any()).Return(errors.Errorf("boom"))

	// Act
	err := s.service(c).ImportRemoteSecrets(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

// syntheticCharmMatcher is a custom gomock matcher that verifies
// RemoteApplicationImport slices have correctly built synthetic charms.
type syntheticCharmMatcher struct {
	expectedApps []RemoteApplicationImport
}

func (m syntheticCharmMatcher) Matches(x any) bool {
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
