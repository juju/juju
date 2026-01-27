// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	model "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstorageservice "github.com/juju/juju/domain/application/service/storage"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	testing.ModelSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "prod", "iaas", "fluffy", "ec2")
		`, s.ModelUUID(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchObsoleteForAppsAndUnitsOwned(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	uri4 := coresecrets.NewURI()

	// Create an initial secret to ensure it is not picked up
	// when the watcher is created.
	sp := secret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchObsoleteSecrets(ctx,
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mysql/0",
		},

		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mediawiki",
		},
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mediawiki/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmApplicationSecret(ctx, st, 1, uri3, "mediawiki", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri4, "mediawiki/0", sp)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// We create a new revision 2, then the old revision 1 of each secret should become obsolete.
	checkObsoleteSecretEvent(c, st, harness, uri1, ptr(1))
	checkObsoleteSecretEvent(c, st, harness, uri2, ptr(1))
	checkObsoleteSecretEvent(c, st, harness, uri3, ptr(1))
	checkObsoleteSecretEvent(c, st, harness, uri4, ptr(1))

	//  We create a new revision 3, then the old revision 2 of each secret should become obsolete.
	checkObsoleteSecretEvent(c, st, harness, uri1, ptr(2))
	checkObsoleteSecretEvent(c, st, harness, uri2, ptr(2))
	checkObsoleteSecretEvent(c, st, harness, uri3, ptr(2))

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchObsoleteForAppsOwned(c *tc.C) {
	s.setupUnits(c, "mysql")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchObsoleteSecrets(ctx,
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
		}
		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// We create a new revision 2, then the old revision 1 of each secret should become obsolete.
	// We watch for the application owned secrets, so the unit owned secret uri2 should not be included.
	checkObsoleteSecretEvent(c, st, harness, uri1, ptr(1))
	checkObsoleteSecretEvent(c, st, harness, uri2, nil)

	// We create a new revision 3, then the old revision 2 of each secret should become obsolete.
	// We watch for the application owned secrets, so the unit owned secret uri2 should not be included.
	checkObsoleteSecretEvent(c, st, harness, uri1, ptr(2))
	checkObsoleteSecretEvent(c, st, harness, uri2, nil)

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchObsoleteForUnitsOwned(c *tc.C) {
	s.setupUnits(c, "mysql")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchObsoleteSecrets(ctx,
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mysql/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
		}
		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// We create a new revision 2, then the old revision 1 of each secret should become obsolete.
	// We watch for the unit owned secrets, so the application owned secret uri1 should not be included.
	checkObsoleteSecretEvent(c, st, harness, uri1, nil)
	checkObsoleteSecretEvent(c, st, harness, uri2, ptr(1))

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchObsoleteUserSecretsToPrune(c *tc.C) {
	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	data := coresecrets.SecretData{"foo": "bar", "hello": "world"}
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	c.Logf("uri1: %v, uri2: %v", uri1, uri2)

	w, err := svc.WatchObsoleteUserSecretsToPrune(ctx)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		err := createUserSecret(ctx, st, 1, uri1, secret.UpsertSecretParams{
			Data:       data,
			RevisionID: ptr(uuid.MustNewUUID().String()),
		})
		c.Assert(err, tc.ErrorIsNil)
		err = createUserSecret(ctx, st, 1, uri2, secret.UpsertSecretParams{
			Data:       data,
			AutoPrune:  ptr(true),
			RevisionID: ptr(uuid.MustNewUUID().String()),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// We create a new revision 2, then the old revision 1 of uri1 should become obsolete.
	// There is no event has been fired because the auto prune is not turned on for uri1.
	checkObsoleteUserSecretToPruneEvent(c, st, harness, uri1, 0)

	// We create a new revision 2, then the old revision 1 of uri2 should become obsolete.
	// An event is fired because the auto prune is turned on for uri2.
	checkObsoleteUserSecretToPruneEvent(c, st, harness, uri2, 2)

	harness.AddTest(c, func(c *tc.C) {
		err = st.UpdateSecret(c.Context(), uri1, secret.UpsertSecretParams{
			AutoPrune: ptr(true),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})

	// Pretend that the agent restarted and the watcher is re-created.
	w1, err := svc.WatchObsoleteUserSecretsToPrune(ctx)
	c.Assert(err, tc.IsNil)
	c.Assert(w1, tc.NotNil)
	defer watchertest.CleanKill(c, w1)

	harness1 := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w1))
	harness1.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness1.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchDeletedForAppOwnedSecret(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()

	// Create an initial secret to ensure it is not picked up
	// when the watcher is created.
	sp := secret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	sp.RevisionID = ptr(uuid.MustNewUUID().String())
	err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
	c.Assert(err, tc.ErrorIsNil)

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchDeletedSecrets(ctx,
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		// Create another app owned secret with an extra revision.
		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)
		createNewRevision(c, st, uri2)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri3, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete the application owned secret.
		err := st.DeleteSecret(ctx, uri1, nil)
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri1.ID,
			),
		)
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete an application owned revision.
		err := st.DeleteSecret(ctx, uri2, []int{1})
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri2.ID + "/1",
			),
		)
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete the unit owned secret.
		err := st.DeleteSecret(ctx, uri3, nil)
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchDeletedSecretRemovesRevisionFromChangeSet(c *tc.C) {
	s.setupUnits(c, "mysql")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchDeletedSecrets(ctx,
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
		}
		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		// Create another app owned secret with a few extra revisions.
		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)
		createNewRevision(c, st, uri2)
		createNewRevision(c, st, uri2)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete the application owned secret.
		err := st.DeleteSecret(ctx, uri1, nil)
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri1.ID,
			),
		)
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete few application owned revisions.
		err := st.DeleteSecret(ctx, uri2, []int{1, 3})
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri2.ID+"/1",
				uri2.ID+"/3",
			),
		)
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete the extra revision of the above secret
		err := st.DeleteSecret(ctx, uri2, []int{2})
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				// the latest revision has been removed, so the whole secret is
				// reported in event
				uri2.ID,
			),
		)
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchDeletedForUnitsOwnedSecret(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchDeletedSecrets(ctx,
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mysql/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
		}
		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri2, "mysql/0", sp)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete the application owned secret.
		err := st.DeleteSecret(ctx, uri1, nil)
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Delete the unit owned secret.
		err := st.DeleteSecret(ctx, uri2, nil)
		tc.Assert(c, err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri2.ID,
			),
		)
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	saveConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		unitName := unittesting.GenNewName(c, consumerID)
		err := st.SaveSecretConsumer(ctx, uri, unitName, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchConsumedSecretsChanges(ctx, "mediawiki/0")
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
		}

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmApplicationSecret(ctx, st, 1, uri2, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		// The consumed revision 1 is the initial revision - will be ignored.
		saveConsumer(uri1, 1, "mediawiki/0")
		// The consumed revision 1 is the initial revision - will be ignored.
		saveConsumer(uri2, 1, "mediawiki/0")
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// We create a new revision 2, then the old revision 1 of each secret should become obsolete.
	// A consumed secret change event of uri1 should be fired.
	harness.AddTest(c, func(c *tc.C) {
		// create revision 2.
		createNewRevision(c, st, uri1)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri1.String(),
			),
		)
	})
	harness.Run(c, []string(nil))

	// Pretend that the agent restarted and the watcher is re-created.
	w1, err := svc.WatchConsumedSecretsChanges(ctx, "mediawiki/0")
	c.Assert(err, tc.IsNil)
	c.Assert(w1, tc.NotNil)
	defer watchertest.CleanKill(c, w1)

	harness1 := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w1))
	harness1.AddTest(c, func(c *tc.C) {
		// The consumed revision 2 is the updated current_revision.
		saveConsumer(uri1, 2, "mediawiki/0")
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness1.Run(c, []string{uri1.String()})
}

func (s *watcherSuite) updateRemoteSecretRevisionInConsumingModel(c *tc.C, uri *coresecrets.URI, latestRevision int, appUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO secret (id) VALUES (?) ON CONFLICT(id) DO NOTHING`, uri.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO secret_reference (secret_id, latest_revision, owner_application_uuid) VALUES (?, ?, ?)
ON CONFLICT(secret_id) DO UPDATE SET
    latest_revision=excluded.latest_revision
`,
			uri.ID, latestRevision, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchConsumedRemoteSecretsChanges(c *tc.C) {
	appUUID := s.setupUnits(c, "mediawiki")
	var unitUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", "mediawiki/0").Scan(&unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	svc, _ := s.setupServiceAndState(c)

	saveConsumer := func(uri *coresecrets.URI, revision int, consumerUUID string) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO secret (id) VALUES (?) ON CONFLICT(id) DO NOTHING`, uri.ID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
INSERT INTO secret_reference (secret_id, latest_revision, owner_application_uuid) VALUES (?, ?, ?)
ON CONFLICT(secret_id) DO UPDATE SET
    latest_revision=excluded.latest_revision
`, uri.ID, revision, appUUID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
INSERT INTO secret_unit_consumer(secret_id, unit_uuid, source_model_uuid, current_revision)
VALUES (?, ?, ?, ?)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET
    label=excluded.label,
    current_revision=excluded.current_revision`,
				uri.ID, consumerUUID, uri.SourceUUID, revision)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	sourceModelUUID := uuid.MustNewUUID()
	uri1 := coresecrets.NewURI()
	uri1.SourceUUID = sourceModelUUID.String()

	uri2 := coresecrets.NewURI()
	uri2.SourceUUID = sourceModelUUID.String()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchConsumedSecretsChanges(ctx, "mediawiki/0")
	c.Assert(err, tc.ErrorIsNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		// The consumed revision 1 is the initial revision - will be ignored.
		saveConsumer(uri1, 1, unitUUID)
		// The consumed revision 1 is the initial revision - will be ignored.
		saveConsumer(uri2, 1, unitUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// We update the remote secret revision to 2.
	// A remote consumed secret change event of uri1 should be fired.
	harness.AddTest(c, func(c *tc.C) {
		s.updateRemoteSecretRevisionInConsumingModel(c, uri1, 2, appUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				uri1.String(),
			),
		)
	})

	harness.Run(c, []string(nil))

	// Pretend that the agent restarted and the watcher is re-created.
	w1, err := svc.WatchConsumedSecretsChanges(ctx, "mediawiki/0")
	c.Assert(err, tc.ErrorIsNil)
	defer watchertest.CleanKill(c, w1)

	harness1 := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w1))
	harness1.AddTest(c, func(c *tc.C) {
		// The consumed revision 2 is the updated current_revision.
		saveConsumer(uri1, 2, unitUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness1.Run(c, []string{uri1.String()})
}

func (s *watcherSuite) TestWatchSecretsRotationChanges(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchSecretsRotationChanges(c.Context(),
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mediawiki/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
		}

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err := createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", sp)
		c.Assert(err, tc.ErrorIsNil)

		sp.RevisionID = ptr(uuid.MustNewUUID().String())
		err = createCharmUnitSecret(ctx, st, 1, uri2, "mediawiki/0", sp)
		c.Assert(err, tc.ErrorIsNil)
		createNewRevision(c, st, uri2)
	}, func(w watchertest.WatcherC[[]corewatcher.SecretTriggerChange]) {
		w.AssertNoChange()
	})

	now := time.Now()
	harness.AddTest(c, func(c *tc.C) {
		err = st.SecretRotated(ctx, uri1, now.Add(1*time.Hour))
		c.Assert(err, tc.ErrorIsNil)
		err = st.SecretRotated(ctx, uri2, now.Add(2*time.Hour))
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]corewatcher.SecretTriggerChange]) {
		w.Check(
			watchertest.SecretTriggerSliceAssert(
				corewatcher.SecretTriggerChange{
					URI:             uri1,
					Revision:        1,
					NextTriggerTime: now.Add(1 * time.Hour),
				},
				corewatcher.SecretTriggerChange{
					URI:             uri2,
					Revision:        2,
					NextTriggerTime: now.Add(2 * time.Hour),
				},
			),
		)
	})

	harness.Run(c, []corewatcher.SecretTriggerChange(nil))

	// Pretend that the agent restarted and the watcher is re-created.
	w1, err := svc.WatchSecretsRotationChanges(c.Context(),
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mediawiki/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w1, tc.NotNil)
	defer watchertest.CleanKill(c, w1)

	harness1 := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w1))

	harness1.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]corewatcher.SecretTriggerChange]) {
		w.Check(
			watchertest.SecretTriggerSliceAssert(
				corewatcher.SecretTriggerChange{
					URI:             uri1,
					Revision:        1,
					NextTriggerTime: now.Add(1 * time.Hour),
				},
				corewatcher.SecretTriggerChange{
					URI:             uri2,
					Revision:        2,
					NextTriggerTime: now.Add(2 * time.Hour),
				},
			),
		)
	})

	harness1.Run(c, []corewatcher.SecretTriggerChange(nil))
}

func ptr[T any](v T) *T {
	return &v
}

func (s *watcherSuite) TestWatchSecretsRevisionExpiryChanges(c *tc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := c.Context()
	svc, st := s.setupServiceAndState(c)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	c.Logf("uri1: %v, uri2: %v", uri1, uri2)

	s.AssertChangeStreamIdle(c)

	w, err := svc.WatchSecretRevisionsExpiryChanges(c.Context(),
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mediawiki/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
	defer watchertest.CleanKill(c, w)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		err := createCharmUnitSecret(ctx, st, 1, uri2, "mediawiki/0", secret.UpsertSecretParams{
			Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
			RevisionID: ptr(uuid.MustNewUUID().String()),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]corewatcher.SecretTriggerChange]) {
		w.AssertNoChange()
	})

	now := time.Now()
	harness.AddTest(c, func(c *tc.C) {
		err = createCharmApplicationSecret(ctx, st, 1, uri1, "mysql", secret.UpsertSecretParams{
			Data:       coresecrets.SecretData{"foo": "bar", "hello": "world"},
			ExpireTime: ptr(now.Add(1 * time.Hour)),
			RevisionID: ptr(uuid.MustNewUUID().String()),
		})
		c.Assert(err, tc.ErrorIsNil)

		err = st.UpdateSecret(c.Context(), uri2, secret.UpsertSecretParams{
			Data:       coresecrets.SecretData{"foo-new": "bar-new"},
			ExpireTime: ptr(now.Add(2 * time.Hour)),
			RevisionID: ptr(uuid.MustNewUUID().String()),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]corewatcher.SecretTriggerChange]) {
		w.Check(
			watchertest.SecretTriggerSliceAssert(
				corewatcher.SecretTriggerChange{
					URI:             uri1,
					Revision:        1,
					NextTriggerTime: now.Add(1 * time.Hour),
				},
				corewatcher.SecretTriggerChange{
					URI:             uri2,
					Revision:        2,
					NextTriggerTime: now.Add(2 * time.Hour),
				},
			),
		)
	})

	harness.Run(c, []corewatcher.SecretTriggerChange(nil))

	// Pretend that the agent restarted and the watcher is re-created.
	w1, err := svc.WatchSecretRevisionsExpiryChanges(c.Context(),
		secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
		secret.CharmSecretOwner{
			Kind: secret.UnitCharmSecretOwner,
			ID:   "mediawiki/0",
		},
	)
	c.Assert(err, tc.IsNil)
	c.Assert(w1, tc.NotNil)
	defer watchertest.CleanKill(c, w1)

	harness1 := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w1))
	harness1.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]corewatcher.SecretTriggerChange]) {
		w.Check(
			watchertest.SecretTriggerSliceAssert(
				corewatcher.SecretTriggerChange{
					URI:             uri1,
					Revision:        1,
					NextTriggerTime: now.Add(1 * time.Hour),
				},
				corewatcher.SecretTriggerChange{
					URI:             uri2,
					Revision:        2,
					NextTriggerTime: now.Add(2 * time.Hour),
				},
			),
		)
	})

	harness1.Run(c, []corewatcher.SecretTriggerChange(nil))
}

func (s *watcherSuite) setupUnits(c *tc.C, appName string) string {
	logger := loggertesting.WrapCheckLog(c)
	st := applicationstate.NewState(s.TxnRunnerFactory(), model.UUID(s.ModelUUID()), clock.WallClock, logger)
	storageProviderRegistryGetter := corestorage.ConstModelStorageRegistry(
		func() internalstorage.ProviderRegistry {
			return internalstorage.NotImplementedProviderRegistry{}
		},
	)
	storageSvc := applicationstorageservice.NewService(
		st,
		applicationstorageservice.NewStoragePoolProvider(
			storageProviderRegistryGetter, st,
		),
		loggertesting.WrapCheckLog(c),
	)

	svc := applicationservice.NewProviderService(
		st,
		storageSvc,
		domaintesting.NoopLeaderEnsurer(),
		nil,
		func(ctx context.Context) (applicationservice.Provider, error) {
			return serviceProvider{}, nil
		},
		func(ctx context.Context) (applicationservice.CAASProvider, error) {
			return serviceProvider{}, nil
		},
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		model.UUID(s.ModelUUID()),
		clock.WallClock,
		logger,
	)

	appUUID, err := svc.CreateIAASApplication(c.Context(),
		appName,
		&stubCharm{},
		corecharm.Origin{
			Source: corecharm.CharmHub,
			Platform: corecharm.Platform{
				Channel:      "24.04",
				OS:           "ubuntu",
				Architecture: "amd64",
			},
		},
		applicationservice.AddApplicationArgs{
			ReferenceName: appName,
			DownloadInfo: &applicationcharm.DownloadInfo{
				Provenance:         applicationcharm.ProvenanceDownload,
				CharmhubIdentifier: "wordpress-1",
				DownloadURL:        "https://example.com/wordpress-1",
				DownloadSize:       1000,
			},
		},
		applicationservice.AddIAASUnitArg{},
	)
	c.Assert(err, tc.ErrorIsNil)
	return appUUID.String()
}

func (s *watcherSuite) setupServiceAndState(c *tc.C) (*service.WatchableService, *state.State) {
	logger := loggertesting.WrapCheckLog(c)
	st := state.NewState(s.TxnRunnerFactory(), logger)
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secret_revision"),
		logger,
	)
	return service.NewWatchableService(st, nil, nil, factory, logger), st
}

func revID(uri *coresecrets.URI, rev int) string {
	return fmt.Sprintf("%s/%d", uri.ID, rev)
}

func createNewRevision(c *tc.C, st *state.State, uri *coresecrets.URI) {
	sp := secret.UpsertSecretParams{
		Data:       coresecrets.SecretData{"foo-new": "bar-new"},
		RevisionID: ptr(uuid.MustNewUUID().String()),
	}
	err := st.UpdateSecret(c.Context(), uri, sp)
	c.Assert(err, tc.ErrorIsNil)
}

func checkObsoleteSecretEvent(c *tc.C, st *state.State, harness *watchertest.Harness[[]string], uri *coresecrets.URI, rev *int) {
	harness.AddTest(c, func(c *tc.C) {
		createNewRevision(c, st, uri)
	}, func(w watchertest.WatcherC[[]string]) {
		if rev != nil {
			w.Check(
				watchertest.StringSliceAssert(
					revID(uri, *rev),
				),
			)
		} else {
			w.AssertNoChange()
		}
	})
}

func checkObsoleteUserSecretToPruneEvent(c *tc.C, st *state.State, harness *watchertest.Harness[struct{}],
	uri *coresecrets.URI, changeCount int) {
	harness.AddTest(c, func(c *tc.C) {
		sp := secret.UpsertSecretParams{
			Data:       coresecrets.SecretData{"foo-new": "bar-new"},
			RevisionID: ptr(uuid.MustNewUUID().String()),
		}
		err := st.UpdateSecret(c.Context(), uri, sp)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		if changeCount > 0 {
			w.AssertNChanges(changeCount)
		} else {
			w.AssertNoChange()
		}
	})
}

func createUserSecret(ctx context.Context, st *state.State, version int, uri *coresecrets.URI, secret secret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.CreateUserSecret(ctx, version, uri, secret)
	})
}

func createCharmApplicationSecret(ctx context.Context, st *state.State, version int, uri *coresecrets.URI, appName string, secret secret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		appUUID, err := st.GetApplicationUUID(ctx, appName)
		if err != nil {
			return err
		}
		return st.CreateCharmApplicationSecret(ctx, version, uri, appUUID, secret)
	})
}

func createCharmUnitSecret(ctx context.Context, st *state.State, version int, uri *coresecrets.URI, unitName unit.Name, secret secret.UpsertSecretParams) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		unitUUID, err := st.GetUnitUUID(ctx, unitName)
		if err != nil {
			return err
		}
		return st.CreateCharmUnitSecret(ctx, version, uri, unitUUID, secret)
	})
}

type stubCharm struct{}

var _ charm.Charm = (*stubCharm)(nil)

func (m *stubCharm) Meta() *charm.Meta {
	return &charm.Meta{
		Name: "foo",
	}
}

func (m *stubCharm) Manifest() *charm.Manifest {
	return &charm.Manifest{
		Bases: []charm.Base{{
			Name:          "ubuntu",
			Channel:       charm.Channel{Risk: charm.Stable},
			Architectures: []string{"amd64"},
		}},
	}
}

func (m *stubCharm) Config() *charm.ConfigSpec {
	return &charm.ConfigSpec{}
}

func (m *stubCharm) Actions() *charm.Actions {
	return &charm.Actions{}
}

func (m *stubCharm) Revision() int {
	return 1
}

func (m *stubCharm) Version() string {
	return ""
}
