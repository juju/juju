// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"context"
	"database/sql"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
)

type uuidSuite struct {
	baseSuite
}

var _ = gc.Suite(&uuidSuite{})

func (s *uuidSuite) TestInitialStateSent(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	// We go through the worker loop twice; once to dispatch initial events,
	// then to pick up tomb.Dying(). Done() is read each time.
	done := make(chan struct{})
	s.sub.EXPECT().Done().Return(done).Times(2)

	changes := make(chan []changestream.ChangeEvent)
	s.sub.EXPECT().Changes().Return(changes)

	s.queue.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"random_namespace",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	err := s.TrackedDB().TxnNoRetry(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE TABLE random_namespace (uuid TEXT PRIMARY KEY)"); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, "INSERT INTO random_namespace(uuid) VALUES ('some-uuid')")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	w := NewUUIDWatcher(s.makeBaseWatcher(), "random_namespace")
	defer workertest.DirtyKill(c, w)

	select {
	case changes := <-w.Changes():
		c.Assert(changes, gc.HasLen, 1)
		c.Check(changes[0], gc.Equals, "some-uuid")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	workertest.CleanKill(c, w)
}
