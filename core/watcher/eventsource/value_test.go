// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/testing"
)

var _ watcher.NotifyWatcher = &ValueWatcher{}

type valueSuite struct {
	baseSuite
}

var _ = gc.Suite(&valueSuite{})

func (s *keysSuite) TestNotificationsSent(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop 4 times:
	// - Dispatch initial notification.
	// - Read deltas.
	// - Dispatch notification.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).Times(4)

	// Tick-tock-tick-tock. 2 assignments of the in channel.
	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas).Times(2)

	subExp.Unsubscribe()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w := NewValueWatcher(s.newBaseWatcher(), "random_namespace", "value")
	defer workertest.CleanKill(c, w)

	// Initial notification.
	select {
	case <-w.Changes():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	// Simulate an incoming change from the stream.
	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 0,
		namespace:  "random_namespace",
		changed:    "some-key-value",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	// Notification for change.
	select {
	case <-w.Changes():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	workertest.CleanKill(c, w)
}

func (s *valueSuite) TestSubscriptionDoneKillsWorker(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()

	done := make(chan struct{})
	close(done)
	subExp.Done().Return(done)

	subExp.Unsubscribe()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w := NewValueWatcher(s.newBaseWatcher(), "random_namespace", "value")
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Check(errors.Is(err, ErrSubscriptionClosed), jc.IsTrue)
}

func (s *valueSuite) TestEnsureCloseOnCleanKill(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()
	done := make(chan struct{})
	subExp.Done().Return(done)
	subExp.Unsubscribe()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w := NewValueWatcher(s.newBaseWatcher(), "random_namespace", "value")

	workertest.CleanKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, jc.IsFalse)
}

func (s *valueSuite) TestEnsureCloseOnDirtyKill(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	subExp := s.sub.EXPECT()
	done := make(chan struct{})
	subExp.Done().Return(done)
	subExp.Unsubscribe()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w := NewValueWatcher(s.newBaseWatcher(), "random_namespace", "value")

	workertest.DirtyKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, jc.IsFalse)
}
