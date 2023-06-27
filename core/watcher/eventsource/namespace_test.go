// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	"database/sql"
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/testing"
)

var _ watcher.StringsWatcher = &NamespaceWatcher{}

type keysSuite struct {
	baseSuite
}

var _ = gc.Suite(&keysSuite{})

func (s *keysSuite) TestInitialStateSent(c *gc.C) {
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

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"random_namespace",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	// The EventQueue is mocked, but we use a real Sqlite DB from which the
	// initial state is read. Insert some data to verify.
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "CREATE TABLE random_namespace (key_name TEXT PRIMARY KEY)"); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, "INSERT INTO random_namespace(key_name) VALUES ('some-key')")
		return err
	})

	c.Assert(err, jc.ErrorIsNil)
	w := NewNamespaceWatcher(
		s.newBaseWatcher(), changestream.All, "random_namespace", "SELECT key_name FROM random_namespace")
	defer workertest.CleanKill(c, w)

	select {
	case changes := <-w.Changes():
		c.Assert(changes, gc.HasLen, 1)
		c.Check(changes[0], gc.Equals, "some-key")
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	workertest.CleanKill(c, w)
}

func (s *keysSuite) TestDeltasSent(c *gc.C) {
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
	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"external_controller",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	w := NewNamespaceWatcher(
		s.newBaseWatcher(), changestream.All, "external_controller", "SELECT uuid FROM external_controller")
	defer workertest.CleanKill(c, w)

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
		changed:    "some-ec-uuid",
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

func (s *keysSuite) TestSubscriptionDoneKillsWorker(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()

	done := make(chan struct{})
	close(done)
	subExp.Done().Return(done)

	subExp.Unsubscribe()

	// The specific table doesn't matter here. Only that exists to read from.
	// We don't need any initial data.
	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace(
			"external_controller",
			changestream.Create|changestream.Update|changestream.Delete,
		)},
	).Return(s.sub, nil)

	w := NewNamespaceWatcher(
		s.newBaseWatcher(), changestream.All, "external_controller", "SELECT uuid FROM external_controller")
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Check(errors.Is(err, ErrSubscriptionClosed), jc.IsTrue)
}

func (s *keysSuite) TestInvalidChangeMask(c *gc.C) {
	w := NewNamespaceWatcher(s.newBaseWatcher(), 0, "external_controller", "SELECT uuid FROM external_controller")
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "changeMask value: 0 not valid")
}

func (s *keysSuite) TestEnsureCloseOnCleanKill(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()
	done := make(chan struct{})
	subExp.Done().Return(done)
	subExp.Unsubscribe()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w := NewValueWatcher(s.newBaseWatcher(), "random_namespace", "key_value")

	workertest.CleanKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, jc.IsFalse)
}

func (s *keysSuite) TestEnsureCloseOnDirtyKill(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()
	done := make(chan struct{})
	subExp.Done().Return(done)
	subExp.Unsubscribe()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w := NewValueWatcher(s.newBaseWatcher(), "random_namespace", "key_value")

	workertest.DirtyKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, jc.IsFalse)
}
