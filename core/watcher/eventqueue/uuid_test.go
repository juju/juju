// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"context"
	"database/sql"
	"errors"
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

	subExp := s.sub.EXPECT()

	// We go through the worker loop twice; once to dispatch initial events,
	// then to pick up tomb.Dying(). Done() is read each time.
	done := make(chan struct{})
	subExp.Done().Return(done).Times(2)

	// When we tick over from the initial dispatch mode to
	// read mode, we will have the in channel assigned.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)

	subExp.Unsubscribe()

	s.queue.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"random_namespace",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	// The EventQueue is mocked, but we use a real Sqlite DB from which the
	// initial state is read. Insert some data to verify.
	err := s.TrackedDB().TxnNoRetry(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE TABLE random_namespace (uuid TEXT PRIMARY KEY)"); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, "INSERT INTO random_namespace(uuid) VALUES ('some-uuid')")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	w := NewUUIDWatcher(s.newBaseWatcher(), "random_namespace")
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

func (s *uuidSuite) TestDeltasSent(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop 4 times:
	// - Dispatch initial events.
	// - Read deltas.
	// - Dispatch deltas.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).Times(4)

	// Tick-tock-tick-tock. 2 assignments of the in channel.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas).Times(2)

	subExp.Unsubscribe()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.queue.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"external_controller",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	w := NewUUIDWatcher(s.newBaseWatcher(), "external_controller")
	defer workertest.DirtyKill(c, w)

	// No initial data.
	select {
	case changes := <-w.Changes():
		c.Assert(changes, gc.HasLen, 0)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 0,
		namespace:  "external_controller",
		uuid:       "some-ec-uuid",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	select {
	case changes := <-w.Changes():
		c.Assert(changes, gc.HasLen, 1)
		c.Check(changes[0], gc.Equals, "some-ec-uuid")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher delta")
	}

	workertest.CleanKill(c, w)
}

func (s *uuidSuite) TestSubscriptionDoneKillsWorker(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()

	done := make(chan struct{})
	close(done)
	subExp.Done().Return(done)

	subExp.Unsubscribe()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.queue.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"external_controller",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	w := NewUUIDWatcher(s.newBaseWatcher(), "external_controller")
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Check(errors.Is(err, ErrSubscriptionClosed), jc.IsTrue)
}

type changeEvent struct {
	changeType changestream.ChangeType
	namespace  string
	uuid       string
}

func (e changeEvent) Type() changestream.ChangeType {
	return e.changeType
}

func (e changeEvent) Namespace() string {
	return e.namespace
}

func (e changeEvent) ChangedUUID() string {
	return e.uuid
}
