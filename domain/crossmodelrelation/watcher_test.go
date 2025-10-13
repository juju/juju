// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation_test

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/offer"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	controllerstate "github.com/juju/juju/domain/crossmodelrelation/state/controller"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/life"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
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

	svc, _ := s.setupService(c, factory)
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
	svc, _ := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	offerUUID := tc.Must(c, offer.NewUUID)
	relationUUID := uuid.MustNewUUID().String()
	s.createLocalOfferForConsumer(c, db, offerUUID)

	watcher, err := svc.WatchRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddRemoteApplicationConsumer(c.Context(), service.AddRemoteApplicationConsumerArgs{
			RemoteApplicationUUID: uuid.MustNewUUID().String(),
			OfferUUID:             offerUUID,
			RelationUUID:          relationUUID,
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

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) (*service.WatchableService, *modelstate.State) {
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
	), modelState
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

func (s *watcherSuite) initModel(c *tc.C, db database.TxnRunner) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "prod", "iaas", "fluffy", "ec2")
		`, s.modelUUID, coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) createSecret(c *tc.C, db database.TxnRunner, uri *coresecrets.URI, content map[string]string) {
	c.Assert(content, gc.Not(gc.HasLen), 0)

	now := time.Now().UTC()
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO secret (id) VALUES (?)`, uri.ID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO secret_metadata (secret_id, version, rotate_policy_id, auto_prune, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?)`,
			uri.ID, 1, 0, false, now, now,
		)
		if err != nil {
			return err
		}
		revisionUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO secret_revision (uuid, secret_id, revision, create_time) VALUES (?, ?, ?, ?)`,
			revisionUUID, uri.ID, 1, now,
		)
		if err != nil {
			return err
		}
		for k, v := range content {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO secret_content (revision_uuid, name, content) VALUES (?, ?, ?)`,
				revisionUUID, k, v,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) addRevision(c *tc.C, db database.TxnRunner, uri *coresecrets.URI, content map[string]string) {
	c.Assert(content, tc.Not(tc.HasLen), 0)

	now := time.Now().UTC()
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		revisionUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx,
			`
INSERT INTO secret_revision (uuid, secret_id, revision, create_time) 
VALUES (?, ?, (SELECT MAX(revision)+1 FROM secret_revision WHERE secret_id=?), ?)`,
			revisionUUID, uri.ID, uri.ID, now,
		)
		if err != nil {
			return err
		}
		for k, v := range content {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO secret_content (revision_uuid, name, content) VALUES (?, ?, ?)`,
				revisionUUID, k, v,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) setupRemoteApp(c *tc.C, db database.TxnRunner, appName string) application.UUID {
	appUUID := uuid.MustNewUUID().String()
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		charmUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id)
VALUES (?, ?, 0);
`, charmUUID, appName)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?);
		`, charmUUID, appName)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, appUUID, charmUUID, appName, life.Alive, network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return application.UUID(appUUID)
}

func (s *watcherSuite) TestWatchRemoteConsumedSecretsChanges(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)

	svc, st := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	s.initModel(c, db)

	ctx := c.Context()

	saveRemoteConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		err := st.SaveSecretRemoteConsumer(ctx, uri, consumerID, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	uri1 := coresecrets.NewURI()
	uri1.SourceUUID = s.modelUUID
	uri2 := coresecrets.NewURI()
	uri2.SourceUUID = s.modelUUID
	appUUID := s.setupRemoteApp(c, db, "mediawiki")

	w, err := svc.WatchRemoteConsumedSecretsChanges(ctx, appUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		s.createSecret(c, db, uri1, map[string]string{"foo": "bar"})
		s.createSecret(c, db, uri2, map[string]string{"foo": "bar"})

		// The consumed revision 1 is the initial revision - will be ignored.
		saveRemoteConsumer(uri1, 1, "mediawiki/0")
		// The consumed revision 1 is the initial revision - will be ignored.
		saveRemoteConsumer(uri2, 1, "mediawiki/0")

	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// We create a new revision 2 and update the remote secret revision to 2.
	// A remote consumed secret change event of uri1 should be fired.
	harness.AddTest(c, func(c *tc.C) {
		s.addRevision(c, db, uri1, map[string]string{"foo": "bar2"})
		err = st.UpdateRemoteSecretRevision(ctx, uri1, 2)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri1.String(),
			),
		)
	})

	harness.Run(c, []string(nil))

	// Pretend that the agent restarted and the watcher is re-created.
	w1, err := svc.WatchRemoteConsumedSecretsChanges(ctx, appUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(w1, tc.NotNil)
	defer watchertest.CleanKill(c, w1)

	harness1 := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w1))
	harness1.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri1.String(),
			),
		)
	})

	harness1.AddTest(c, func(c *tc.C) {
		// The consumed revision 2 is the updated current_revision.
		saveRemoteConsumer(uri1, 2, "mediawiki/0")
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness1.Run(c, []string(nil))

	// Pretend that the agent restarted and the watcher is re-created again.
	// Since we consume the latest revision already, so there should be no change.
	w2, err := svc.WatchRemoteConsumedSecretsChanges(ctx, appUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(w2, tc.NotNil)
	defer watchertest.CleanKill(c, w2)

	harness2 := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w2))
	harness2.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness2.Run(c, []string(nil))
}
