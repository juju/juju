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

// TestImportLegacyOfferRelationIdentityAndState checks the identity and state
// of the offering side of a 3.6 cross model relation that is imported by the
// cross model relation domain: the numeric relation id of the source model,
// the suspended state of the relation and the relation status. Reallocating
// the numeric relation ID breaks agent checkpoints, and resetting the
// suspended state or status changes what existing hooks see after migration.
//
// The endpoint settings and unit scope membership of the relation are
// imported by the relation domain, and are covered by a follow-up PR along
// with the test from the issue that also asserts them.
func (s *importSuite) TestImportLegacyOfferRelationIdentityAndState(c *tc.C) {
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
	const offerUUID = "cfa46843-ebf2-4fff-8519-c1fb5a9816f3"
	const relationUUID = "6049aa01-76c9-462d-8440-964a6e26aac2"
	const remote = "remote-13ea27915e7840d888c5e9451444b45d"
	a.AddOffer(description.ApplicationOfferArgs{
		OfferUUID: offerUUID, OfferName: "mysql", ApplicationName: "mysql", Endpoints: map[string]string{"db": "db"},
	})
	rapp := m.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: remote, IsConsumerProxy: true, SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9",
	})
	rapp.AddEndpoint(description.RemoteEndpointArgs{Name: "db", Role: "requirer", Interface: "db"})
	r := m.AddRelation(description.RelationArgs{
		Id: 42, Key: remote + ":db mysql:db",
		Suspended: true, SuspendedReason: "waiting for the consumer model",
	})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: "mysql", Name: "db", Role: "provider", Interface: "db"})
	r.AddEndpoint(description.EndpointArgs{ApplicationName: remote, Name: "db", Role: "requirer", Interface: "db"})
	r.SetStatus(description.StatusArgs{Value: "joined", Updated: time.Now().UTC()})
	m.AddRemoteEntity(description.RemoteEntityArgs{ID: "application-" + remote, Token: "13ea2791-5e78-40d8-88c5-e9451444b45d"})
	m.AddRemoteEntity(description.RemoteEntityArgs{ID: "relation-" + remote + ".db#mysql.db", Token: relationUUID})
	m.AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID: offerUUID, RelationID: 42, RelationKey: r.Key(),
		SourceModelUUID: "4ddd6454-931d-4278-8779-b0b7208994d9", UserName: "admin",
	})

	_, scope, _ := s.setupCoordinatorScopeAndService(c)
	logger := loggertesting.WrapCheckLog(c)
	coordinator := coremodelmigration.NewCoordinator(logger)
	// Keep the production order, including conversion of the 3.6 sequence
	// (next ID) to the 4.0 sequence (last allocated ID). The relation must be
	// imported with the id of the source model, not with the next value of
	// the sequence.
	m.SetSequence("relation", 43)
	sequencemigration.RegisterImport(coordinator)
	applicationmigration.RegisterImport(coordinator, clock.WallClock, logger)
	cmrmigration.RegisterImport(coordinator, clock.WallClock, logger)
	relationmigration.RegisterImport(coordinator, clock.WallClock, logger)
	statusmigration.RegisterImport(coordinator, clock.WallClock, logger)
	c.Assert(coordinator.Perform(c.Context(), scope, m), tc.ErrorIsNil)

	runner, err := scope.ModelDB()(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	var (
		relationID      int
		suspended       bool
		suspendedReason string
		relationStatus  string
	)
	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT relation_id, suspended, COALESCE(suspended_reason, '')
FROM relation WHERE uuid = ?`, relationUUID).Scan(&relationID, &suspended, &suspendedReason); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `
SELECT rst.name
FROM relation_status AS rs
JOIN relation_status_type AS rst ON rs.relation_status_type_id = rst.id
WHERE rs.relation_uuid = ?`, relationUUID).Scan(&relationStatus)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relationID, tc.Equals, 42)
	c.Check(suspended, tc.IsTrue)
	c.Check(suspendedReason, tc.Equals, "waiting for the consumer model")
	c.Check(relationStatus, tc.Equals, "joined")
}
