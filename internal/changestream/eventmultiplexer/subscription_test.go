// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"sync/atomic"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
)

type subscriptionSuite struct {
	baseSuite
}

var _ = gc.Suite(&subscriptionSuite{})

func (s *subscriptionSuite) TestSubscriptionIsDone(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0, func() {})
	defer workertest.CleanKill(c, sub)

	workertest.CleanKill(c, sub)

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("failed to wait for subscription done")
	}
}

func (s *subscriptionSuite) TestSubscriptionUnsubscriptionIsCalled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var called bool
	sub := newSubscription(0, func() { called = true })
	defer workertest.CleanKill(c, sub)

	sub.Unsubscribe()
	c.Assert(called, jc.IsTrue)

	workertest.CleanKill(c, sub)
}

func (s *subscriptionSuite) TestSubscriptionWitnessChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0, func() {
		c.Fatalf("failed if called")
	})
	defer workertest.CleanKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestream.Create,
		ns:      "foo",
		changed: "1",
	}}

	go func() {
		err := sub.dispatch(context.Background(), changes)
		c.Assert(err, jc.ErrorIsNil)
	}()

	var witnessed ChangeSet
	select {
	case got := <-sub.Changes():
		witnessed = got
	case <-time.After(testing.ShortWait):
	}

	c.Assert(witnessed, gc.HasLen, len(changes))
	c.Check(witnessed, jc.SameContents, changes)

	workertest.CleanKill(c, sub)
}

func (s *subscriptionSuite) TestSubscriptionDoesNoteWitnessChangesWithCancelledContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0, func() {
		c.Fatalf("failed if called")
	})
	defer workertest.CleanKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestream.Create,
		ns:      "foo",
		changed: "1",
	}}

	syncPoint := make(chan struct{})
	go func() {
		defer close(syncPoint)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := sub.dispatch(ctx, changes)
		c.Assert(err, jc.ErrorIsNil)
	}()

	select {
	case <-syncPoint:
	case <-time.After(testing.ShortWait):
		c.Fatalf("failed waiting for sync point")
	}

	select {
	case <-sub.Changes():
		c.Fatalf("unexpected changes witnessed")
	case <-time.After(testing.ShortWait):
	}

	workertest.CleanKill(c, sub)
}

func (s *subscriptionSuite) TestSubscriptionDoesNotWitnessChangesWithUnsub(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var witnessed int64
	sub := newSubscription(0, func() {
		atomic.AddInt64(&witnessed, 1)
	})
	defer workertest.CleanKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestream.Create,
		ns:      "foo",
		changed: "1",
	}}

	syncPoint := make(chan struct{})
	go func() {
		defer close(syncPoint)

		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()

		time.Sleep(time.Millisecond)

		err := sub.dispatch(ctx, changes)
		c.Assert(err, jc.ErrorIsNil)
	}()

	select {
	case <-syncPoint:
	case <-time.After(testing.ShortWait):
		c.Fatalf("failed waiting for sync point")
	}

	select {
	case <-sub.Changes():
		c.Fatalf("unexpected changes witnessed")
	case <-time.After(testing.ShortWait):
	}

	// We should have witnessed the unsubscribe
	c.Check(atomic.LoadInt64(&witnessed), gc.Equals, int64(1))

	workertest.CleanKill(c, sub)
}

func (s *subscriptionSuite) TestSubscriptionDoesNotWitnessChangesWithDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0, func() {
		c.Fatalf("failed if called")
	})
	defer workertest.CleanKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestream.Create,
		ns:      "foo",
		changed: "1",
	}}

	syncPoint := make(chan struct{})
	go func() {
		defer close(syncPoint)

		err := sub.close()
		c.Assert(err, jc.ErrorIsNil)

		err = sub.dispatch(context.Background(), changes)
		c.Assert(err, gc.ErrorMatches, "tomb: dying")
	}()

	select {
	case <-syncPoint:
	case <-time.After(testing.ShortWait):
		c.Fatalf("failed waiting for sync point")
	}

	select {
	case <-sub.Changes():
		c.Fatalf("unexpected changes witnessed")
	case <-time.After(testing.ShortWait):
	}

	workertest.CleanKill(c, sub)
}
