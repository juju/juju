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
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
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
	relationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()

	s.createLocalOfferForConsumer(c, db, offerUUID)

	watcher, err := svc.WatchRemoteApplicationConsumers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddConsumedRelation(c.Context(), service.AddConsumedRelationArgs{
			ConsumerApplicationUUID: uuid.MustNewUUID().String(),
			OfferUUID:               offerUUID,
			OfferingEndpointName:    "db",
			RelationUUID:            relationUUID,
			ConsumerModelUUID:       consumerModelUUID,
			ConsumerApplicationEndpoint: charm.Relation{
				Name:  "db",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeGlobal,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var offerConnUUID string
			if err := tx.QueryRowContext(ctx, `
SELECT arc.offer_connection_uuid
FROM application_remote_consumer arc
JOIN offer_connection oc ON oc.uuid = arc.offer_connection_uuid
WHERE oc.offer_uuid = ?`, offerUUID).Scan(&offerConnUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM application_remote_consumer WHERE offer_connection_uuid=?`, offerConnUUID); err != nil {
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
	modelState := modelstate.NewState(modelDB, coremodel.UUID(s.modelUUID), clock.WallClock, loggertesting.WrapCheckLog(c))

	return service.NewWatchableService(
		controllerState,
		modelState,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
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
	c.Assert(content, tc.Not(tc.HasLen), 0)

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
		err = st.UpdateRemoteSecretRevision(ctx, uri1, 2, appUUID.String())
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

func (s *watcherSuite) TestWatchConsumerRelations(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Set up a remote offerer app and a relation (initial changes).
	remoteRelationUUID := s.setupRemoteOffererLocalAndRelation(c, db, svc)

	// Start the watcher.
	s.modelIdler.AssertChangeStreamIdle(c)
	w, err := svc.WatchConsumerRelations(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w))

	// Seting the relation to dying should trigger an event.
	harness.AddTest(c, func(c *tc.C) {
		s.setRelationDying(c, db, remoteRelationUUID.String())
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(remoteRelationUUID.String()))
	})

	// Now create a relation between two local applications; this should be ignored by the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.createLocalToLocalRelation(c, db)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{remoteRelationUUID.String()})
}

func (s *watcherSuite) TestWatchOffererRelations(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Set up a local offer and a remote consumer with a relation (initial
	// changes).
	remoteRelationUUID := s.setupLocalOfferRemoteConsumerAndRelation(c, db, svc)

	// Start the watcher.
	s.modelIdler.AssertChangeStreamIdle(c)
	w, err := svc.WatchOffererRelations(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w))

	// Setting the relation to dying should trigger an event.
	harness.AddTest(c, func(c *tc.C) {
		s.setRelationDying(c, db, remoteRelationUUID.String())
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(remoteRelationUUID.String()))
	})

	// Now create a relation between two local applications; this should be
	// ignored by the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.createLocalToLocalRelation(c, db)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{remoteRelationUUID.String()})
}

func (s *watcherSuite) TestWatchOffererRelationsCaching(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Set up a local offer with an endpoint "db".
	localOfferUUID := tc.Must(c, offer.NewUUID)
	s.createLocalOfferForConsumer(c, db, localOfferUUID)

	// Start the watcher before adding any remote consumers.
	s.modelIdler.AssertChangeStreamIdle(c)
	w, err := svc.WatchOffererRelations(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w))

	// Add an app remote consumer - this should create a relation and trigger
	// the watcher.
	var consumerRelationUUID1 string
	var remoteApplicationUUID1 string
	harness.AddTest(c, func(c *tc.C) {
		remoteApplicationUUID1 = tc.Must(c, uuid.NewUUID).String()
		consumerModelUUID := tc.Must(c, uuid.NewUUID).String()
		consumerRelationUUID1 = tc.Must(c, uuid.NewUUID).String()

		err := svc.AddConsumedRelation(c.Context(), service.AddConsumedRelationArgs{
			ConsumerApplicationUUID: remoteApplicationUUID1,
			OfferUUID:               localOfferUUID,
			OfferingEndpointName:    "db",
			RelationUUID:            consumerRelationUUID1,
			ConsumerModelUUID:       consumerModelUUID,
			ConsumerApplicationEndpoint: charm.Relation{
				Name:      "db",
				Role:      charm.RoleRequirer,
				Interface: "db",
				Scope:     charm.ScopeGlobal,
			},
		})
		c.Assert(err, tc.ErrorIsNil)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(consumerRelationUUID1))
	})

	// Add a second app remote consumer - the watcher should now track this
	// relation too.
	var consumerRelationUUID2 string
	harness.AddTest(c, func(c *tc.C) {
		remoteApplicationUUID := tc.Must(c, uuid.NewUUID).String()
		consumerModelUUID := tc.Must(c, uuid.NewUUID).String()
		consumerRelationUUID2 = tc.Must(c, uuid.NewUUID).String()

		err := svc.AddConsumedRelation(c.Context(), service.AddConsumedRelationArgs{
			ConsumerApplicationUUID: remoteApplicationUUID,
			OfferUUID:               localOfferUUID,
			OfferingEndpointName:    "db",
			RelationUUID:            consumerRelationUUID2,
			ConsumerModelUUID:       consumerModelUUID,
			ConsumerApplicationEndpoint: charm.Relation{
				Name:      "db",
				Role:      charm.RoleRequirer,
				Interface: "db",
				Scope:     charm.ScopeGlobal,
			},
		})
		c.Assert(err, tc.ErrorIsNil)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(consumerRelationUUID2))
	})

	// Remove the first app remote consumer - the watcher should now stop tracking
	// this relation.
	harness.AddTest(c, func(c *tc.C) {
		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var offerConnUUID string
			if err := tx.QueryRowContext(ctx, `
SELECT oc.uuid
FROM application_remote_consumer arc
JOIN offer_connection oc ON oc.uuid = arc.offer_connection_uuid
WHERE oc.remote_relation_uuid = ?`, consumerRelationUUID1).Scan(&offerConnUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM application_remote_consumer WHERE offer_connection_uuid=?`, offerConnUUID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM offer_connection WHERE uuid=?`, offerConnUUID); err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Modify the first remote relation - should NOT trigger an event because
	// the consumer was removed and the cache should have been updated.
	harness.AddTest(c, func(c *tc.C) {
		s.setRelationDying(c, db, consumerRelationUUID1)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Modify the second remote relation - should trigger an event because
	// its consumer is still active and the cache should still track it.
	harness.AddTest(c, func(c *tc.C) {
		s.setRelationDying(c, db, consumerRelationUUID2)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(consumerRelationUUID2))
	})

	// Create a local-to-local relation - should be ignored by the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.createLocalToLocalRelation(c, db)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) setupLocalOfferRemoteConsumerAndRelation(c *tc.C, db database.TxnRunner, svc *service.WatchableService) uuid.UUID {
	// Create a local application "local-app" with an endpoint "db".
	localOfferUUID := tc.Must(c, offer.NewUUID)
	s.createLocalOfferForConsumer(c, db, localOfferUUID)

	// Add a remote consumer for the local offer.
	remoteApplicationUUID := tc.Must(c, uuid.NewUUID).String()
	consumerModelUUID := tc.Must(c, uuid.NewUUID).String()
	consumerRelationUUID := tc.Must(c, uuid.NewUUID)

	err := svc.AddConsumedRelation(c.Context(), service.AddConsumedRelationArgs{
		ConsumerApplicationUUID: remoteApplicationUUID,
		OfferUUID:               localOfferUUID,
		OfferingEndpointName:    "db",
		RelationUUID:            consumerRelationUUID.String(),
		ConsumerModelUUID:       consumerModelUUID,
		ConsumerApplicationEndpoint: charm.Relation{
			Name:      "db",
			Role:      charm.RoleRequirer,
			Interface: "db",
			Scope:     charm.ScopeGlobal,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	return consumerRelationUUID
}

func (s *watcherSuite) setupRemoteOffererLocalAndRelation(c *tc.C, db database.TxnRunner, svc *service.WatchableService) corerelation.UUID {
	err := svc.AddRemoteApplicationOfferer(c.Context(), "foo", service.AddRemoteApplicationOffererArgs{
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

	localOfferUUID := tc.Must(c, offer.NewUUID)
	s.createLocalOfferForConsumer(c, db, localOfferUUID)

	var relUUID corerelation.UUID
	var localAppUUID, localEP string
	err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var remoteAppUUID string
		if err := tx.QueryRowContext(ctx, `SELECT uuid FROM application WHERE name = ?`, "foo").Scan(&remoteAppUUID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT uuid FROM application WHERE name = ?`, "local-app").Scan(&localAppUUID); err != nil {
			return err
		}

		qEP := `
SELECT ae.uuid
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
WHERE  ae.application_uuid = ? AND cr.name = ?`
		var remoteEP string
		if err := tx.QueryRowContext(ctx, qEP, remoteAppUUID, "db").Scan(&remoteEP); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, qEP, localAppUUID, "db").Scan(&localEP); err != nil {
			return err
		}

		var globalScopeID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_scope WHERE name='global'`).Scan(&globalScopeID); err != nil {
			return err
		}

		relUUID = tc.Must(c, corerelation.NewUUID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, 1, ?)`, relUUID.String(), globalScopeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, tc.Must(c, uuid.NewUUID).String(), relUUID.String(), remoteEP); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, tc.Must(c, uuid.NewUUID).String(), relUUID.String(), localEP); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	s.createUnitWithAddressForRelation(c, db, localAppUUID, relUUID.String(), localEP)

	return relUUID
}

func (s *watcherSuite) createUnitWithAddressForRelation(c *tc.C, db database.TxnRunner, appUUID, relationUUID, endpointUUID string) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var charmUUID string
		if err := tx.QueryRowContext(ctx, `SELECT charm_uuid FROM application WHERE uuid = ?`, appUUID).Scan(&charmUUID); err != nil {
			return err
		}

		netNodeUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID); err != nil {
			return err
		}

		unitUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO unit (uuid, net_node_uuid, name, life_id, application_uuid, charm_uuid)
VALUES (?, ?, 'local-app/0', 0, ?, ?)`, unitUUID, netNodeUUID, appUUID, charmUUID); err != nil {
			return err
		}

		var relationEndpointUUID string
		err := tx.QueryRowContext(ctx, `
SELECT uuid FROM relation_endpoint WHERE relation_uuid = ? AND endpoint_uuid = ?`, relationUUID, endpointUUID).Scan(&relationEndpointUUID)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
VALUES (?, ?, ?)`, tc.Must(c, uuid.NewUUID).String(), relationEndpointUUID, unitUUID); err != nil {
			return err
		}

		deviceUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, device_type_id, virtual_port_type_id)
VALUES (?, ?, 'eth0', 2, 0)`, deviceUUID, netNodeUUID); err != nil {
			return err
		}

		ipUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, config_type_id, origin_id, scope_id)
VALUES (?, ?, ?, '198.51.100.1', 0, 4, 1, 1)`, ipUUID, netNodeUUID, deviceUUID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) createLocalToLocalRelation(c *tc.C, db database.TxnRunner) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		charmUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id, revision)
VALUES (?, ?, 0, 1)`, charmUUID, charmUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate, description)
VALUES (?, ?, false, 'test app')`, charmUUID, "local-app-2"); err != nil {
			return err
		}
		var providerRoleID, globalScopeID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_role WHERE name='provider'`).Scan(&providerRoleID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_scope WHERE name='global'`).Scan(&globalScopeID); err != nil {
			return err
		}
		crUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, capacity, scope_id)
VALUES (?, ?, 'db', ?, 'db', 0, ?)`, crUUID, charmUUID, providerRoleID, globalScopeID); err != nil {
			return err
		}
		appUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, 'local-app-2', 0, ?, ?)`, appUUID, charmUUID, network.AlphaSpaceId); err != nil {
			return err
		}
		epUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid)
VALUES (?, ?, ?, ?)`, epUUID, appUUID, crUUID, network.AlphaSpaceId); err != nil {
			return err
		}

		// Get endpoint for first local app "local-app".
		var local1AppUUID, ep1 string
		if err := tx.QueryRowContext(ctx, `SELECT uuid FROM application WHERE name = ?`, "local-app").Scan(&local1AppUUID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
SELECT ae.uuid
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
WHERE  ae.application_uuid = ? AND cr.name = ?`, local1AppUUID, "db").Scan(&ep1); err != nil {
			return err
		}

		rUUID := tc.Must(c, uuid.NewUUID).String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, 2, ?)`, rUUID, globalScopeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, tc.Must(c, uuid.NewUUID).String(), rUUID, ep1); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, tc.Must(c, uuid.NewUUID).String(), rUUID, epUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) setRelationDying(c *tc.C, db database.TxnRunner, relationUUID string) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE relation SET life_id = ? WHERE uuid = ?`, life.Dying, relationUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchRelationIngressNetworks(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	remoteRelationUUID := s.setupRemoteOffererLocalAndRelation(c, db, svc)
	saasIngressAllow := []string{"0.0.0.0/0", "::/0"}
	err = svc.AddRelationNetworkIngress(c.Context(), remoteRelationUUID, saasIngressAllow, []string{"203.0.113.0/24"})
	c.Assert(err, tc.ErrorIsNil)

	s.modelIdler.AssertChangeStreamIdle(c)
	w, err := svc.WatchRelationIngressNetworks(c.Context(), remoteRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w))

	// Adding an ingress network should trigger an event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddRelationNetworkIngress(c.Context(), remoteRelationUUID, saasIngressAllow, []string{"192.0.2.0/24"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Adding another ingress network to the same relation should trigger
	// another event.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddRelationNetworkIngress(c.Context(), remoteRelationUUID, saasIngressAllow, []string{"198.51.100.0/24"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Deleting an ingress network should trigger an event.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteIngressNetwork(c, db, remoteRelationUUID, "192.0.2.0/24")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Deleting the last ingress network should trigger an event.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteIngressNetwork(c, db, remoteRelationUUID, "198.51.100.0/24")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Now create a different relation and add ingress networks to it; this
	// should be ignored.
	harness.AddTest(c, func(c *tc.C) {
		otherRelationUUID := s.createRelation(c, db, svc)
		err := svc.AddRelationNetworkIngress(c.Context(), otherRelationUUID, saasIngressAllow, []string{"203.0.113.0/24"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchRelationIngressNetworksEmptyRelationUUID(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	_, err := svc.WatchRelationIngressNetworks(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, "relation uuid cannot be empty")
}

func (s *watcherSuite) TestWatchRelationIngressNetworksInvalidUUID(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	_, err := svc.WatchRelationIngressNetworks(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "relation uuid \"foo\": not valid")
}

func (s *watcherSuite) deleteIngressNetwork(c *tc.C, db database.TxnRunner, relationUUID corerelation.UUID, cidr string) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
DELETE FROM relation_network_ingress
WHERE relation_uuid = ? AND cidr = ?`, relationUUID.String(), cidr)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchRelationEgressNetworks(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	remoteRelationUUID := s.setupRemoteOffererLocalAndRelation(c, db, svc)

	s.modelIdler.AssertChangeStreamIdle(c)
	w, err := svc.WatchRelationEgressNetworks(c.Context(), remoteRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, w))

	// Adding an egress network should trigger an event with the CIDR.
	harness.AddTest(c, func(c *tc.C) {
		s.addEgressNetwork(c, db, remoteRelationUUID, "192.0.2.0/24")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("192.0.2.0/24"))
	})

	// Deleting an egress network falls back to unit addresses (converted to
	// CIDRs).
	harness.AddTest(c, func(c *tc.C) {
		s.deleteEgressNetwork(c, db, remoteRelationUUID, "192.0.2.0/24")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("198.51.100.1/32"))
	})

	// Adding an egress network overrides the unit address fallback.
	harness.AddTest(c, func(c *tc.C) {
		s.addEgressNetwork(c, db, remoteRelationUUID, "198.51.100.0/24")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("198.51.100.0/24"))
	})

	// Adding multiple egress networks should return all CIDRs.
	harness.AddTest(c, func(c *tc.C) {
		s.addEgressNetwork(c, db, remoteRelationUUID, "203.0.113.0/24")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("198.51.100.0/24", "203.0.113.0/24"))
	})

	// Changes to a different relation's egress networks should be ignored.
	harness.AddTest(c, func(c *tc.C) {
		otherRelationUUID := s.createRelation(c, db, svc)
		s.addEgressNetwork(c, db, otherRelationUUID, "10.0.0.0/8")
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Deleting the last egress network falls back to unit addresses.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteEgressNetwork(c, db, remoteRelationUUID, "198.51.100.0/24")
		s.deleteEgressNetwork(c, db, remoteRelationUUID, "203.0.113.0/24")
	}, func(w watchertest.WatcherC[[]string]) {
		// Falls back to unit address CIDR.
		w.Check(watchertest.StringSliceAssert("198.51.100.1/32"))
	})

	// Changes to unrelated model config keys should be ignored.
	harness.AddTest(c, func(c *tc.C) {
		s.setModelConfig(c, db, "some-other-key", "some-value")
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Initial event should contain the unit address converted to CIDR.
	// The setupRemoteOffererLocalAndRelation creates a unit with address
	// 198.51.100.1 (public scope), which should be returned as 198.51.100.1/32.
	harness.Run(c, []string{"198.51.100.1/32"})
}

func (s *watcherSuite) TestWatchRelationEgressNetworksEmptyRelationUUID(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	_, err := svc.WatchRelationEgressNetworks(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, "relation uuid cannot be empty")
}

func (s *watcherSuite) TestWatchRelationEgressNetworksInvalidUUID(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)
	svc, _ := s.setupService(c, factory)

	_, err := svc.WatchRelationEgressNetworks(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "relation uuid \"foo\": not valid")
}

func (s *watcherSuite) addEgressNetwork(c *tc.C, db database.TxnRunner, relationUUID corerelation.UUID, cidr string) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO relation_network_egress (relation_uuid, cidr)
VALUES (?, ?)
ON CONFLICT (relation_uuid, cidr) DO NOTHING`, relationUUID.String(), cidr)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) deleteEgressNetwork(c *tc.C, db database.TxnRunner, relationUUID corerelation.UUID, cidr string) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
DELETE FROM relation_network_egress
WHERE relation_uuid = ? AND cidr = ?`, relationUUID.String(), cidr)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) setModelConfig(c *tc.C, db database.TxnRunner, key, value string) {
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_config (key, value)
VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) createRelation(c *tc.C, db database.TxnRunner, svc *service.WatchableService) corerelation.UUID {
	var relUUID corerelation.UUID
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var globalScopeID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_scope WHERE name='global'`).Scan(&globalScopeID); err != nil {
			return err
		}

		relUUID = tc.Must(c, corerelation.NewUUID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, 2, ?)`, relUUID.String(), globalScopeID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return relUUID
}
