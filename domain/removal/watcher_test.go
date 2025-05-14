// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/removal/service"
	"github.com/juju/juju/domain/removal/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

var _ = tc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchRemovals(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "some-model-uuid")

	log := loggertesting.WrapCheckLog(c)

	svc := service.NewWatchableService(
		state.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, log),
		domain.NewWatcherFactory(factory, log),
		clock.WallClock,
		log,
	)

	w, err := svc.WatchRemovals()
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	// Insert 2 new jobs and check that the watcher emits their UUIDs.
	harness.AddTest(func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			q := `INSERT INTO removal (uuid, removal_type_id, entity_uuid) VALUES (?, ?, ?)`

			if _, err := tx.ExecContext(ctx, q, "job-uuid-1", 1, "rel-uuid-1"); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, q, "job-uuid-2", 1, "rel-uuid-2")
			return err
		})
		c.Assert(err, tc.ErrorIsNil)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("job-uuid-1", "job-uuid-2"))
	})
}
