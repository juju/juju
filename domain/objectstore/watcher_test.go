// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore_test

import (
	"context"
	"database/sql"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/domain/objectstore/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchWithAdd(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, clock.WallClock),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Add a new object.
	metadata := objectstore.Metadata{
		Path:   "foo",
		SHA256: "hash256",
		SHA384: "hash384",
		Size:   666,
	}
	_, err = svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, tc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}
}

func (s *watcherSuite) TestWatchWithDelete(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, clock.WallClock),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Add a new object.
	metadata := objectstore.Metadata{
		Path:   "foo",
		SHA256: "hash256",
		SHA384: "hash384",
		Size:   666,
	}
	_, err = svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, tc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Remove the object.
	err = svc.RemoveMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, tc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	_, err = svc.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

func (s *watcherSuite) TestWatchDraining(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableDrainingService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) }, clock.WallClock),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.WatchDraining(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `UPDATE object_store_backend SET life_id = 1`); err != nil {
				return err
			}

			_, err := tx.ExecContext(ctx, `
INSERT INTO object_store_backend (uuid, life_id, type_id, updated_at) 
VALUES ('foo', 0, 1, CURRENT_TIMESTAMP)
`)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)

		err = svc.SetDrainingPhase(c.Context(), objectstore.PhaseDraining)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetDrainingPhase(c.Context(), objectstore.PhaseCompleted)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchObjectStoreBackend(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableDrainingService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) {
			return factory(ctx)
		}, clock.WallClock),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)

	s.AssertChangeStreamIdle(c)

	watcher, err := svc.WatchObjectStoreBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	currentBackend := getActiveBackend(c, factory)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var nextBackend string
	harness.AddTest(c, func(c *tc.C) {
		err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
			Endpoint:  "https://s3.example.invalid",
			AccessKey: "access-key",
			SecretKey: "secret-key",
		})
		c.Assert(err, tc.ErrorIsNil)

		nextBackend = getActiveBackend(c, factory)
		c.Assert(nextBackend, tc.Not(tc.Equals), currentBackend)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(currentBackend, nextBackend))
	})

	harness.Run(c, []string{currentBackend})
}

// TestWatchDrainingFullLifecycle tests the complete draining lifecycle through
// service calls: transition to S3 (which atomically initiates draining),
// observe the draining phase, complete the drain, and mark the backend as
// drained.
func (s *watcherSuite) TestWatchDrainingFullLifecycle(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableDrainingService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) {
			return factory(ctx)
		}, clock.WallClock),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)

	// Verify the initial state: no draining in progress.
	phase, err := svc.GetDrainingPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(phase, tc.Equals, objectstore.PhaseUnknown)

	// Record the initial active backend.
	initialBackend := getActiveBackend(c, factory)
	c.Assert(initialBackend, tc.Not(tc.Equals), "")

	// Start watching the draining table.
	watcher, err := svc.WatchDraining(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Stage 1: TransitionBackendToS3 atomically starts draining.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
			Endpoint:  "https://s3.example.invalid",
			AccessKey: "access-key",
			SecretKey: "secret-key",
		})
		c.Assert(err, tc.ErrorIsNil)

		// Verify the draining phase is now active.
		phase, err := svc.GetDrainingPhase(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(phase, tc.Equals, objectstore.PhaseDraining)

		// Verify the active backend has changed.
		newBackend := getActiveBackend(c, factory)
		c.Assert(newBackend, tc.Not(tc.Equals), initialBackend)

		// Verify the draining phase info includes the from backend UUID.
		phaseInfo, err := svc.GetDrainingPhaseInfo(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(phaseInfo.Phase, tc.Equals, objectstore.PhaseDraining)
		c.Assert(phaseInfo.FromBackendUUID, tc.Not(tc.IsNil))
		c.Assert(string(*phaseInfo.FromBackendUUID), tc.Equals, initialBackend)
		c.Assert(string(phaseInfo.ActiveBackendUUID), tc.Equals, newBackend)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Stage 2: Complete the drain phase. This atomically marks the from-backend
	// as dead and transitions the phase to completed.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetDrainingPhase(c.Context(), objectstore.PhaseCompleted)
		c.Assert(err, tc.ErrorIsNil)

		// After completing, the drain is no longer "active", so
		// GetDrainingPhase returns PhaseUnknown.
		phase, err := svc.GetDrainingPhase(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(phase, tc.Equals, objectstore.PhaseUnknown)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

// TestWatchDrainingTransitionToError tests the draining lifecycle when
// transitioning to an error state instead of completed.
func (s *watcherSuite) TestWatchDrainingTransitionToError(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableDrainingService(
		state.NewState(func(ctx context.Context) (database.TxnRunner, error) {
			return factory(ctx)
		}, clock.WallClock),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)

	watcher, err := svc.WatchDraining(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Stage 1: TransitionBackendToS3 starts draining.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.TransitionBackendToS3(c.Context(), domainobjectstore.S3Credentials{
			Endpoint:  "https://s3.example.invalid",
			AccessKey: "access-key",
			SecretKey: "secret-key",
		})
		c.Assert(err, tc.ErrorIsNil)

		phase, err := svc.GetDrainingPhase(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(phase, tc.Equals, objectstore.PhaseDraining)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Stage 2: Set the phase to error. Error is terminal, so there is no
	// longer an active drain and GetDrainingPhase returns PhaseUnknown.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.SetDrainingPhase(c.Context(), objectstore.PhaseError)
		c.Assert(err, tc.ErrorIsNil)

		phase, err := svc.GetDrainingPhase(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(phase, tc.Equals, objectstore.PhaseUnknown)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func getActiveBackend(c *tc.C, factory changestream.WatchableDBFactory) string {
	db, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var backend string
	err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT uuid FROM object_store_backend WHERE life_id = 0`).Scan(&backend)
	})
	c.Assert(err, tc.ErrorIsNil)

	return backend
}
