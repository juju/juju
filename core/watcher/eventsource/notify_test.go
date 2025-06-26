// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testing"
)

type notifySuite struct {
	baseSuite
}

var _ watcher.NotifyWatcher = &NotifyWatcher{}

func TestNotifySuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &notifySuite{})
}

func (s *notifySuite) TestNotificationsByNamespaceFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop minimum 4 times:
	// - Read initial delta (additional subsequent events aren't guaranteed).
	// - Dispatch initial notification.
	// - Read deltas.
	// - Dispatch notification.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).MinTimes(4)

	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)

	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyMapperWatcher(s.newBaseWatcher(c), func(ctx context.Context, e []changestream.ChangeEvent) ([]string, error) {
		if len(e) != 1 {
			c.Fatalf("expected 1 event, got %d", len(e))
		}
		if s := e[0].Changed(); s == "some-key-value" {
			return singleton(s), nil
		}
		return nil, nil
	}, NamespaceFilter("random_namespace", changestream.All))
	defer workertest.DirtyKill(c, w)
	c.Assert(err, tc.ErrorIsNil)

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

	// Simulate an incoming change from the stream.
	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 1,
		namespace:  "random_namespace",
		changed:    "should-not-match",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	// Notification for change.
	select {
	case <-w.Changes():
		c.Fatal("unexpected changes")
	case <-time.After(time.Second):
	}

	workertest.CleanKill(c, w)
}

func (s *notifySuite) TestNotificationsByPredicateFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop minimum 4 times:
	// - Read initial delta (additional subsequent events aren't guaranteed).
	// - Dispatch initial notification.
	// - Read deltas.
	// - Dispatch notification.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).MinTimes(4)

	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)

	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyMapperWatcher(s.newBaseWatcher(c), func(ctx context.Context, e []changestream.ChangeEvent) ([]string, error) {
		if len(e) != 1 {
			c.Fatalf("expected 1 event, got %d", len(e))
		}
		if s := e[0].Changed(); s == "some-key-value" {
			return singleton(s), nil
		}
		return nil, nil
	}, PredicateFilter("random_namespace", changestream.All, EqualsPredicate("some-key-value")))
	defer workertest.DirtyKill(c, w)
	c.Assert(err, tc.ErrorIsNil)

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

	// Simulate an incoming change from the stream.
	select {
	case deltas <- []changestream.ChangeEvent{changeEvent{
		changeType: 1,
		namespace:  "random_namespace",
		changed:    "should-not-match",
	}}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out dispatching change event")
	}

	// Notification for change.
	select {
	case <-w.Changes():
		c.Fatal("unexpected changes")
	case <-time.After(time.Second):
	}

	workertest.CleanKill(c, w)
}

func (s *notifySuite) TestNotificationsByMapperError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	done := make(chan struct{})
	subExp.Done().Return(done).MinTimes(1)

	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)

	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyMapperWatcher(s.newBaseWatcher(c), func(_ context.Context, _ []changestream.ChangeEvent) ([]string, error) {
		return nil, errors.Errorf("boom")
	}, PredicateFilter("random_namespace", changestream.All, EqualsPredicate("value")))
	defer workertest.DirtyKill(c, w)
	c.Assert(err, tc.ErrorIsNil)

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
	case _, ok := <-w.Changes():
		// Ensure the channel is closed, when the predicate dies.
		c.Assert(ok, tc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for initial watcher changes")
	}

	err = workertest.CheckKill(c, w)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *notifySuite) TestNotificationsSent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()

	// We go through the worker loop minimum 4 times:
	// - Read initial delta (additional subsequent events aren't guaranteed).
	// - Dispatch initial notification.
	// - Read deltas.
	// - Dispatch notification.
	// - Pick up tomb.Dying()
	done := make(chan struct{})
	subExp.Done().Return(done).MinTimes(4)

	deltas := make(chan []changestream.ChangeEvent)
	subExp.Changes().Return(deltas)

	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyWatcher(s.newBaseWatcher(c), PredicateFilter("random_namespace", changestream.All, EqualsPredicate("value")))
	defer workertest.DirtyKill(c, w)
	c.Assert(err, tc.ErrorIsNil)

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

func (s *notifySuite) TestSubscriptionDoneKillsWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()
	subExp.Changes().Return(make(chan []changestream.ChangeEvent)).AnyTimes()

	done := make(chan struct{})
	close(done)
	subExp.Done().Return(done)

	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyWatcher(s.newBaseWatcher(c), PredicateFilter("random_namespace", changestream.All, EqualsPredicate("value")))
	defer workertest.DirtyKill(c, w)
	c.Assert(err, tc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorIs, ErrSubscriptionClosed)
}

func (s *notifySuite) TestEnsureCloseOnCleanKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()
	done := make(chan struct{})
	subExp.Changes().Return(make(chan []changestream.ChangeEvent)).AnyTimes()
	subExp.Done().Return(done)
	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyWatcher(s.newBaseWatcher(c), PredicateFilter("random_namespace", changestream.All, EqualsPredicate("value")))
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher to close")
	}
}

func (s *notifySuite) TestEnsureCloseOnDirtyKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subExp := s.sub.EXPECT()
	done := make(chan struct{})
	subExp.Changes().Return(make(chan []changestream.ChangeEvent))
	subExp.Done().Return(done)
	subExp.Kill()

	s.eventsource.EXPECT().Subscribe(
		subscriptionOptionMatcher{opt: changestream.Namespace("random_namespace", changestream.All)},
	).Return(s.sub, nil)

	w, err := NewNotifyWatcher(s.newBaseWatcher(c), PredicateFilter("random_namespace", changestream.All, EqualsPredicate("value")))
	c.Assert(err, tc.ErrorIsNil)

	workertest.DirtyKill(c, w)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for watcher to close")
	}
}

func (s *notifySuite) TestNilOption(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewNotifyWatcher(s.newBaseWatcher(c), nil)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *notifySuite) TestNilPredicate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewNotifyWatcher(s.newBaseWatcher(c), PredicateFilter("random_namespace", changestream.All, nil))
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}
