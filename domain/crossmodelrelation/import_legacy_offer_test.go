// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	applicationmigration "github.com/juju/juju/domain/application/modelmigration"
	cmrmigration "github.com/juju/juju/domain/crossmodelrelation/modelmigration"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	relationmigration "github.com/juju/juju/domain/relation/modelmigration"
	sequencemigration "github.com/juju/juju/domain/sequence/modelmigration"
	statusmigration "github.com/juju/juju/domain/status/modelmigration"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// legacyOffer holds the identifiers of the Juju 3.6 offering side built by
// legacyOfferModel, so that tests can assert on what the import created.
type legacyOffer struct {
	offerUUID     string
	consumerUUID  string
	relationUUID  string
	remoteApp     string
	remoteUnit    string
	relationKey   string
	appSettings   map[string]string
	unitSettings  map[string]string
	unitsInScope  []string
	relationID    int
	suspendedName string
}

// legacyOfferModel returns the model description of an offering model with one
// established cross model relation, as exported by Juju 3.6: the offered
// application, the remote application standing in for the consumer, and the
// relation between them, carrying the settings of both endpoints and the
// settings of the unit that is in relation scope.
//
// The relation also carries the numeric relation ID, the suspended state and
// the status of the source model. Reallocating the numeric relation ID breaks
// agent checkpoints, and losing the settings or the scope membership of a unit
// changes what existing hooks see once the model has migrated.
func legacyOfferModel() (description.Model, legacyOffer) {
	offer := legacyOffer{
		offerUUID:     "cfa46843-ebf2-4fff-8519-c1fb5a9816f3",
		consumerUUID:  "13ea2791-5e78-40d8-88c5-e9451444b45d",
		relationUUID:  "6049aa01-76c9-462d-8440-964a6e26aac2",
		relationID:    42,
		suspendedName: "waiting for the consumer model",
	}
	offer.remoteApp = "remote-13ea27915e7840d888c5e9451444b45d"
	offer.remoteUnit = offer.remoteApp + "/0"
	offer.relationKey = offer.remoteApp + ":db mysql:db"
	offer.appSettings = map[string]string{
		"mysql:password":              "keep-me",
		offer.remoteApp + ":database": "keep-me-too",
	}
	offer.unitSettings = map[string]string{
		offer.remoteUnit + ":request": "keep-unit-data",
	}
	offer.unitsInScope = []string{offer.remoteUnit}

	m := description.NewModel(description.ModelArgs{Type: model.IAAS.String()})
	a := m.AddApplication(description.ApplicationArgs{Name: "mysql", CharmURL: "ch:mysql-1"})
	a.SetCharmOrigin(description.CharmOriginArgs{
		Source: "charm-hub", ID: "deadbeef", Hash: "deadbeef2", Revision: 1,
		Channel: "latest/stable", Platform: "amd64/ubuntu/20.04",
	})
	a.SetCharmMetadata(description.CharmMetadataArgs{
		Name: "mysql", Provides: map[string]description.CharmMetadataRelation{
			"db": migrationtesting.Relation{Name_: "db", Role_: "provider", InterfaceName_: "db", Scope_: "global"},
		},
	})
	a.SetCharmManifest(description.CharmManifestArgs{Bases: []description.CharmManifestBase{
		migrationtesting.ManifestBase{Name_: "ubuntu", Channel_: "stable", Architectures_: []string{"amd64"}},
	}})
	a.AddOffer(description.ApplicationOfferArgs{
		OfferUUID: offer.offerUUID, OfferName: "mysql",
		ApplicationName: "mysql", Endpoints: map[string]string{"db": "db"},
	})

	rapp := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: offer.remoteApp, IsConsumerProxy: true,
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
	})
	rapp.AddEndpoint(description.RemoteEndpointArgs{
		Name: "db", Role: "requirer", Interface: "db",
	})

	rel := m.AddRelation(description.RelationArgs{
		Id: offer.relationID, Key: offer.relationKey,
		Suspended: true, SuspendedReason: offer.suspendedName,
	})
	ep := rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: "mysql", Name: "db", Role: "provider", Interface: "db",
	})
	ep.SetApplicationSettings(map[string]any{"password": "keep-me"})
	ep = rel.AddEndpoint(description.EndpointArgs{
		ApplicationName: offer.remoteApp, Name: "db",
		Role: "requirer", Interface: "db",
	})
	ep.SetApplicationSettings(map[string]any{"database": "keep-me-too"})
	ep.SetUnitSettings(offer.remoteUnit, map[string]any{
		"request": "keep-unit-data",
	})
	rel.SetStatus(description.StatusArgs{Value: "joined", Updated: time.Now().UTC()})

	// Legacy models exchange tokens rather than UUIDs. The relation token is
	// the identity that both sides of the relation agreed on, so the imported
	// relation keeps using it as its relation UUID.
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID: "application-" + offer.remoteApp, Token: offer.consumerUUID,
	})
	m.AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "relation-" + offer.remoteApp + ".db#mysql.db",
		Token: offer.relationUUID,
	})
	m.AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID: offer.offerUUID, RelationID: offer.relationID,
		RelationKey:     offer.relationKey,
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
		UserName:        "admin",
	})

	// Keep the production order, including conversion of the 3.6 sequence
	// (next ID) to the 4.0 sequence (last allocated ID). The relation must be
	// imported with the ID of the source model, not with the next value of the
	// sequence.
	m.SetSequence("relation", offer.relationID+1)
	return m, offer
}

// registerLegacyOfferImports registers the import operations that the offering
// side of a legacy cross model relation depends on, in the production order:
// the cross model relation import creates the relation of the remote
// application consumer, and the relation import, which must therefore come
// after it, imports the data of that relation.
func registerLegacyOfferImports(
	c *tc.C,
	coordinator *coremodelmigration.Coordinator,
	includeRelationNetworks bool,
) {
	logger := loggertesting.WrapCheckLog(c)
	sequencemigration.RegisterImport(coordinator)
	applicationmigration.RegisterImport(coordinator, clock.WallClock, logger)
	cmrmigration.RegisterImport(coordinator, clock.WallClock, logger)
	relationmigration.RegisterImport(coordinator, clock.WallClock, logger)
	if includeRelationNetworks {
		cmrmigration.RegisterImportRelationNetworks(coordinator, clock.WallClock, logger)
	}
	statusmigration.RegisterImport(coordinator, clock.WallClock, logger)
}

// TestImportLegacyOfferRelationPreservesIdentityAndData checks the offering
// side of an established 3.6 cross model relation before workers can republish
// any data: the numeric relation ID, the suspended state, the status, the
// application settings of both endpoints, and the settings and scope membership
// of the remote unit.
func (s *importSuite) TestImportLegacyOfferRelationPreservesIdentityAndData(c *tc.C) {
	m, offer := legacyOfferModel()

	_, scope, _ := s.setupCoordinatorScopeAndService(c)
	coordinator := coremodelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
	)
	registerLegacyOfferImports(c, coordinator, false)
	c.Assert(coordinator.Perform(c.Context(), scope, m), tc.ErrorIsNil)

	runner, err := scope.ModelDB()(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var (
		relationID      int
		suspended       bool
		suspendedReason string
		relationStatus  string
		appSettings     map[string]string
		unitSettings    map[string]string
		units           []string
	)
	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		appSettings = make(map[string]string)
		unitSettings = make(map[string]string)
		units = nil

		if err := tx.QueryRowContext(ctx, `
SELECT relation_id, suspended, COALESCE(suspended_reason, '')
FROM relation WHERE uuid = ?`, offer.relationUUID).
			Scan(&relationID, &suspended, &suspendedReason); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
SELECT rst.name
FROM relation_status AS rs
JOIN relation_status_type AS rst ON rs.relation_status_type_id = rst.id
WHERE rs.relation_uuid = ?`, offer.relationUUID).Scan(&relationStatus); err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT a.name, s.key, s.value
FROM relation_application_setting AS s
JOIN relation_endpoint AS re ON re.uuid = s.relation_endpoint_uuid
JOIN application_endpoint AS ae ON ae.uuid = re.endpoint_uuid
JOIN application AS a ON a.uuid = ae.application_uuid
WHERE re.relation_uuid = ?`, offer.relationUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var app, key, value string
			if err := rows.Scan(&app, &key, &value); err != nil {
				return err
			}
			appSettings[app+":"+key] = value
		}
		if err := rows.Err(); err != nil {
			return rows.Err()
		}

		rows, err = tx.QueryContext(ctx, `
SELECT u.name, s.key, s.value
FROM relation_unit_setting AS s
JOIN relation_unit AS ru ON ru.uuid = s.relation_unit_uuid
JOIN relation_endpoint AS re ON re.uuid = ru.relation_endpoint_uuid
JOIN unit AS u ON u.uuid = ru.unit_uuid
WHERE re.relation_uuid = ?`, offer.relationUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var unit, key, value string
			if err := rows.Scan(&unit, &key, &value); err != nil {
				return err
			}
			unitSettings[unit+":"+key] = value
		}
		if err := rows.Err(); err != nil {
			return rows.Err()
		}

		rows, err = tx.QueryContext(ctx, `
SELECT u.name
FROM relation_unit AS ru
JOIN relation_endpoint AS re ON re.uuid = ru.relation_endpoint_uuid
JOIN unit AS u ON u.uuid = ru.unit_uuid
WHERE re.relation_uuid = ?`, offer.relationUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			units = append(units, name)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(relationID, tc.Equals, offer.relationID)
	c.Check(suspended, tc.IsTrue)
	c.Check(suspendedReason, tc.Equals, offer.suspendedName)
	c.Check(relationStatus, tc.Equals, "joined")
	c.Check(appSettings, tc.DeepEquals, offer.appSettings)
	c.Check(unitSettings, tc.DeepEquals, offer.unitSettings)
	c.Check(units, tc.SameContents, offer.unitsInScope)
}

// TestImportLegacyOfferRelationNetworks checks the relation networks of the
// offering side of a legacy cross model relation. They are imported by the
// cross model relation domain, and located by relation key, so they need the
// relation of the remote application consumer to exist first. The admin
// override egress networks of the source model take precedence over the
// default ones.
func (s *importSuite) TestImportLegacyOfferRelationNetworks(c *tc.C) {
	m, offer := legacyOfferModel()
	m.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          offer.relationKey + ":ingress:default",
		RelationKey: offer.relationKey,
		CIDRS:       []string{"10.0.0.0/24"},
	})
	m.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          offer.relationKey + ":egress:default",
		RelationKey: offer.relationKey,
		CIDRS:       []string{"192.168.0.0/16"},
	})
	m.AddRelationNetwork(description.RelationNetworkArgs{
		ID:          offer.relationKey + ":egress:override",
		RelationKey: offer.relationKey,
		CIDRS:       []string{"192.168.1.0/24"},
	})

	_, scope, _ := s.setupCoordinatorScopeAndService(c)
	coordinator := coremodelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
	)
	registerLegacyOfferImports(c, coordinator, true)
	c.Assert(coordinator.Perform(c.Context(), scope, m), tc.ErrorIsNil)

	runner, err := scope.ModelDB()(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var ingressCIDRs, egressCIDRs []string
	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		ingressCIDRs, egressCIDRs = nil, nil

		var cidrs []string
		readCIDRs := func(query string) error {
			rows, err := tx.QueryContext(ctx, query, offer.relationUUID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var cidr string
				if err := rows.Scan(&cidr); err != nil {
					return err
				}
				cidrs = append(cidrs, cidr)
			}
			return rows.Err()
		}

		if err := readCIDRs(`
SELECT cidr FROM relation_network_ingress WHERE relation_uuid = ?`); err != nil {
			return err
		}
		ingressCIDRs = cidrs

		cidrs = nil
		if err := readCIDRs(`
SELECT cidr FROM relation_network_egress WHERE relation_uuid = ?`); err != nil {
			return err
		}
		egressCIDRs = cidrs
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ingressCIDRs, tc.SameContents, []string{"10.0.0.0/24"})
	c.Check(egressCIDRs, tc.SameContents, []string{"192.168.1.0/24"})
}
