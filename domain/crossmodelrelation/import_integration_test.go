// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/application/charm"
	applicationmodelmigration "github.com/juju/juju/domain/application/modelmigration"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/crossmodelrelation/modelmigration"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	controllerstate "github.com/juju/juju/domain/crossmodelrelation/state/controller"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/life"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	schematesting.ControllerModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

// TestImport is a comprehensive happy-path test that verifies the complete
// import process for both offers and remote applications with all fields populated.
// This ensures no data is lost during migration.
func (s *importSuite) TestImport(c *tc.C) {
	// Arrange: set up the import data
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Set up application with offers.
	appName := "foo"
	app := desc.AddApplication(description.ApplicationArgs{
		Name:     appName,
		CharmURL: "ch:foo-1",
	})
	app.SetCharmOrigin(description.CharmOriginArgs{
		Source:   "charm-hub",
		ID:       "deadbeef",
		Hash:     "deadbeef2",
		Revision: 1,
		Channel:  "latest/stable",
		Platform: "amd64/ubuntu/20.04",
	})
	app.SetCharmMetadata(description.CharmMetadataArgs{
		Name: appName,
		Provides: map[string]description.CharmMetadataRelation{
			"db": migrationtesting.Relation{
				Name_:          "db",
				Role_:          "provider",
				InterfaceName_: "db",
				Optional_:      true,
				Limit_:         1,
				Scope_:         "global",
			},
			"db-admin": migrationtesting.Relation{
				Name_:          "db-admin",
				Role_:          "provider",
				InterfaceName_: "db-admin",
				Optional_:      true,
				Limit_:         1,
				Scope_:         "global",
			},
			"cos-agent": migrationtesting.Relation{
				Name_:          "cos-agent",
				Role_:          "provider",
				InterfaceName_: "agent",
				Optional_:      true,
				Limit_:         1,
				Scope_:         "global",
			},
		},
		Requires: map[string]description.CharmMetadataRelation{
			"cache": migrationtesting.Relation{
				Name_:          "cache",
				Role_:          "requirer",
				InterfaceName_: "cache",
				Optional_:      true,
				Limit_:         3,
				Scope_:         "container",
			},
		},
	})
	app.SetCharmManifest(description.CharmManifestArgs{
		Bases: []description.CharmManifestBase{
			migrationtesting.ManifestBase{
				Name_:          "ubuntu",
				Channel_:       "stable",
				Architectures_: []string{"amd64"},
			},
		},
	})

	// Add offers for the application.
	offerOneUUID := tc.Must(c, uuid.NewUUID).String()
	offerOneName := "foo"
	app.AddOffer(description.ApplicationOfferArgs{
		OfferUUID:       offerOneUUID,
		OfferName:       offerOneName,
		Endpoints:       map[string]string{"db": "db", "db-admin": "db-admin"},
		ApplicationName: appName,
	})
	offerTwoUUID := tc.Must(c, uuid.NewUUID).String()
	offerTwoName := "agent"
	app.AddOffer(description.ApplicationOfferArgs{
		OfferUUID:       offerTwoUUID,
		OfferName:       offerTwoName,
		Endpoints:       map[string]string{"cos-agent": "cos-agent"},
		ApplicationName: appName,
	})

	// Set up remote applications.
	remoteOfferUUID := tc.Must(c, uuid.NewUUID).String()
	remoteSourceModelUUID := tc.Must(c, uuid.NewUUID).String()
	remoteMacaroon1 := newMacaroon(c, "kafka-macaroon")
	remoteMacaroon1JSON := string(tc.Must(c, remoteMacaroon1.MarshalJSON))

	remoteApp1 := desc.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-kafka",
		OfferUUID:       remoteOfferUUID,
		URL:             "ctrl:admin/production.kafka",
		SourceModelUUID: remoteSourceModelUUID,
		IsConsumerProxy: false,
		ConsumeVersion:  2, // Note: ConsumeVersion is not currently imported
		Macaroon:        remoteMacaroon1JSON,
		Bindings:        map[string]string{"client": "alpha", "zookeeper": "beta"}, // Note: Bindings are not currently imported
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "client",
		Role:      "provider",
		Interface: "kafka",
	})
	remoteApp1.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "zookeeper",
		Role:      "requirer",
		Interface: "zookeeper",
	})

	// Add a second remote application to test multiple remote apps.
	remoteOfferUUID2 := tc.Must(c, uuid.NewUUID).String()
	remoteSourceModelUUID2 := tc.Must(c, uuid.NewUUID).String()
	remoteMacaroon2 := newMacaroon(c, "redis-macaroon")
	remoteMacaroon2JSON := string(tc.Must(c, remoteMacaroon2.MarshalJSON))

	remoteApp2 := desc.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-redis",
		OfferUUID:       remoteOfferUUID2,
		URL:             "ctrl:admin/staging.redis",
		SourceModelUUID: remoteSourceModelUUID2,
		IsConsumerProxy: false,
		Macaroon:        remoteMacaroon2JSON,
	})
	remoteApp2.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "cache",
		Role:      "provider",
		Interface: "redis",
	})

	// Add a consumer proxy remote application (should be skipped during import).
	consumerProxyMacaroon := newMacaroon(c, "proxy-macaroon")
	consumerProxyMacaroonJSON := string(tc.Must(c, consumerProxyMacaroon.MarshalJSON))
	consumerProxyApp := desc.AddRemoteApplication(description.RemoteApplicationArgs{
		Name:            "remote-consumer-proxy",
		OfferUUID:       tc.Must(c, uuid.NewUUID).String(),
		URL:             "ctrl:admin/consumer.app",
		SourceModelUUID: tc.Must(c, uuid.NewUUID).String(),
		IsConsumerProxy: true, // This should be skipped during import
		Macaroon:        consumerProxyMacaroonJSON,
	})
	consumerProxyApp.AddEndpoint(description.RemoteEndpointArgs{
		Name:      "endpoint",
		Role:      "provider",
		Interface: "http",
	})

	// Arrange: setup the db and import
	coordinator, scope, svc := s.setupCoordinatorScopeAndService(c)

	// Act
	err := coordinator.Perform(c.Context(), scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// Verify offers were imported correctly.
	obtainedOffers, err := svc.GetOffers(c.Context(), []service.OfferFilter{{}})
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Endpoints", tc.Ignore)
	c.Check(obtainedOffers, tc.UnorderedMatch[[]*crossmodelrelation.OfferDetail](mc), []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:       offerOneUUID,
			OfferName:       offerOneName,
			ApplicationName: appName,
			CharmLocator:    charm.CharmLocator{Name: appName, Revision: 1, Source: "charmhub"},
		}, {
			OfferUUID:       offerTwoUUID,
			OfferName:       offerTwoName,
			ApplicationName: appName,
			CharmLocator:    charm.CharmLocator{Name: appName, Revision: 1, Source: "charmhub"},
		},
	}, tc.Commentf("%+v\n%+v", obtainedOffers[0], obtainedOffers[1]))

	// Verify offer endpoints.
	obtainedEndpoints := transform.SliceToMap(obtainedOffers, func(in *crossmodelrelation.OfferDetail) (string, []crossmodelrelation.OfferEndpoint) {
		return in.OfferName, in.Endpoints
	})
	obtainedOfferOneEndpoints, ok := obtainedEndpoints[offerOneName]
	if c.Check(ok, tc.IsTrue, tc.Commentf("missing %q endpoints", offerOneName)) {
		c.Check(obtainedOfferOneEndpoints, tc.SameContents, []crossmodelrelation.OfferEndpoint{
			{
				Name:      "db",
				Role:      "provider",
				Interface: "db",
				Limit:     1,
			}, {
				Name:      "db-admin",
				Role:      "provider",
				Interface: "db-admin",
				Limit:     1,
			},
		})
	}
	obtainedOfferTwoEndpoints, ok := obtainedEndpoints[offerTwoName]
	if c.Check(ok, tc.IsTrue, tc.Commentf("missing %q endpoints", offerTwoName)) {
		c.Check(obtainedOfferTwoEndpoints, tc.SameContents, []crossmodelrelation.OfferEndpoint{
			{
				Name:      "cos-agent",
				Role:      "provider",
				Interface: "agent",
				Limit:     1,
			},
		})
	}

	// Verify remote applications were imported correctly.
	// Consumer proxy should be skipped, so only 2 remote apps are expected.
	obtainedRemoteApps, err := svc.GetRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(obtainedRemoteApps), tc.Equals, 2, tc.Commentf("Expected 2 remote apps (consumer proxy should be skipped)"))

	// Build a map for easier lookup.
	remoteAppsByName := transform.SliceToMap(obtainedRemoteApps, func(in crossmodelrelation.RemoteApplicationOfferer) (string, crossmodelrelation.RemoteApplicationOfferer) {
		return in.ApplicationName, in
	})

	// Verify remote-kafka.
	kafkaApp, ok := remoteAppsByName["remote-kafka"]
	if c.Check(ok, tc.IsTrue, tc.Commentf("missing remote-kafka")) {
		c.Check(kafkaApp.ApplicationName, tc.Equals, "remote-kafka")
		c.Check(kafkaApp.OfferUUID, tc.Equals, remoteOfferUUID)
		c.Check(kafkaApp.OfferURL, tc.Equals, "ctrl:admin/production.kafka")
		c.Check(kafkaApp.OffererModelUUID, tc.Equals, remoteSourceModelUUID)
		c.Check(kafkaApp.Life, tc.Equals, life.Alive)
		// Verify macaroon was stored correctly.
		c.Check(kafkaApp.Macaroon, tc.DeepEquals, remoteMacaroon1)
	}

	// Verify remote-redis.
	redisApp, ok := remoteAppsByName["remote-redis"]
	if c.Check(ok, tc.IsTrue, tc.Commentf("missing remote-redis")) {
		c.Check(redisApp.ApplicationName, tc.Equals, "remote-redis")
		c.Check(redisApp.OfferUUID, tc.Equals, remoteOfferUUID2)
		c.Check(redisApp.OfferURL, tc.Equals, "ctrl:admin/staging.redis")
		c.Check(redisApp.OffererModelUUID, tc.Equals, remoteSourceModelUUID2)
		c.Check(redisApp.Life, tc.Equals, life.Alive)
		// Verify macaroon was stored correctly.
		c.Check(redisApp.Macaroon, tc.DeepEquals, remoteMacaroon2)
	}

	// Verify consumer proxy was NOT imported.
	_, ok = remoteAppsByName["remote-consumer-proxy"]
	c.Check(ok, tc.IsFalse, tc.Commentf("consumer proxy should not be imported"))
}

// setupCoordinatorScopeAndService returns the coordinator, scope and service
// to use in testing import. The scope and service must share the same
// modelRunner instance for these tests to be successful.
func (s *importSuite) setupCoordinatorScopeAndService(c *tc.C) (*coremodelmigration.Coordinator, coremodelmigration.Scope, *service.Service) {
	coordinator := coremodelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	applicationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	modelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	modelUUID := tc.Must(c, model.NewUUID)

	controllerFactory := func(context.Context) (database.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (database.TxnRunner, error) {
		return modelRunner, nil
	}

	scope := coremodelmigration.NewScope(controllerFactory, modelFactory, nil, "deadbeef", modelUUID)
	srv := service.NewService(
		controllerstate.NewState(controllerFactory, loggertesting.WrapCheckLog(c)),
		modelstate.NewState(modelFactory, modelUUID, clock.WallClock, loggertesting.WrapCheckLog(c)),
		nil,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	return coordinator, scope, srv
}
