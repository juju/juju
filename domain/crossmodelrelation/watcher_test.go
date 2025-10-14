// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	controllerstate "github.com/juju/juju/domain/crossmodelrelation/state/controller"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ControllerModelSuite

	modelUUID  string
	modelIdler *changestreamtesting.Idler
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	s.modelUUID = uuid.MustNewUUID().String()
	_, s.modelIdler = s.InitWatchableDB(c, s.modelUUID)
}

func (s *watcherSuite) TestWatchRemoteApplicationOfferers(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		err = svc.AddRemoteApplicationOfferer(c.Context(), "foo", service.AddRemoteApplicationOffererArgs{
			OfferUUID:        tc.Must(c, offer.NewUUID),
			OffererModelUUID: tc.Must(c, uuid.NewUUID).String(),
			Endpoints: []charm.Relation{{
				Name:  "db",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeGlobal,
			}},
			Macaroon: newMacaroon(c, "offer-macaroon"),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var applicationUUID string
			err := tx.QueryRowContext(ctx, `SELECT uuid FROM application WHERE name = "foo"`).Scan(&applicationUUID)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
DELETE FROM application_remote_offerer_status
WHERE application_remote_offerer_uuid = (
SELECT uuid FROM application_remote_offerer WHERE application_uuid = ?)`, applicationUUID)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `DELETE FROM application_remote_offerer WHERE application_uuid = ?`, applicationUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchRemoteApplicationConsumers(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	s.createLocalOfferForConsumer(c, db, offerUUID)

	watcher, err := svc.WatchRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddRemoteApplicationConsumer(c.Context(), service.AddRemoteApplicationConsumerArgs{
			RemoteApplicationUUID: uuid.MustNewUUID().String(),
			OfferUUID:             offerUUID,
			RelationUUID:          relationUUID,
			ConsumerModelUUID:     consumerModelUUID,
			Endpoints: []charm.Relation{{
				Name:  "db",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeGlobal,
			}},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var consumerUUID, offerConnUUID string
			if err := tx.QueryRowContext(ctx, `
	SELECT arc.uuid, arc.offer_connection_uuid
	FROM application_remote_consumer arc
	JOIN offer_connection oc ON oc.uuid = arc.offer_connection_uuid
	WHERE oc.offer_uuid = ?`, offerUUID).Scan(&consumerUUID, &offerConnUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM application_remote_consumer WHERE uuid=?`, consumerUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM offer_connection WHERE uuid=?`, offerConnUUID); err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	controllerDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.GetWatchableDB(ctx, s.modelUUID)
	}

	controllerState := controllerstate.NewState(controllerDB, loggertesting.WrapCheckLog(c))
	modelState := modelstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))

	return service.NewWatchableService(
		controllerState,
		modelState,
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}

func (s *watcherSuite) createLocalOfferForConsumer(c *tc.C, db database.TxnRunner, offerUUID offer.UUID) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create charm.
		charmUUID := uuid.MustNewUUID().String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id, revision)
VALUES (?, ?, 0, 1)`, charmUUID, charmUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate, description)
VALUES (?, ?, false, 'test app')`, charmUUID, "local-app"); err != nil {
			return err
		}

		// Get provider role and global scope IDs.
		var providerRoleID, globalScopeID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_role WHERE name='provider'`).Scan(&providerRoleID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_scope WHERE name='global'`).Scan(&globalScopeID); err != nil {
			return err
		}

		// Create charm relation referenced by endpoint/offer.
		charmRelationUUID := uuid.MustNewUUID().String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, capacity, scope_id)
VALUES (?, ?, 'db', ?, 'db', 0, ?)`, charmRelationUUID, charmUUID, providerRoleID, globalScopeID); err != nil {
			return err
		}

		// Create application.
		appUUID := uuid.MustNewUUID().String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, 'local-app', 0, ?, ?)`, appUUID, charmUUID, network.AlphaSpaceId); err != nil {
			return err
		}

		// Application endpoint.
		appEndpointUUID := uuid.MustNewUUID().String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid)
VALUES (?, ?, ?, ?)`, appEndpointUUID, appUUID, charmRelationUUID, network.AlphaSpaceId); err != nil {
			return err
		}

		// Offer + endpoint mapping.
		if _, err := tx.ExecContext(ctx, `
INSERT INTO offer (uuid, name)
VALUES (?, 'local-offer')`, offerUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid)
VALUES (?, ?)`, offerUUID, appEndpointUUID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}
