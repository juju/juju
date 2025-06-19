// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	"github.com/juju/juju/core/testing"
)

type subscriptionSuite struct {
	baseSuite
}

func TestSubscriptionSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &subscriptionSuite{})
}

func (s *subscriptionSuite) TestSubscriptionIsDone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0)
	defer workertest.DirtyKill(c, sub)

	workertest.CleanKill(c, sub)

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("failed to wait for subscription done")
	}
}

func (s *subscriptionSuite) TestSubscriptionWitnessChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0)
	defer workertest.DirtyKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "foo",
		changed: "1",
	}}

	go func() {
		err := sub.dispatch(c.Context(), changes)
		c.Assert(err, tc.ErrorIsNil)
	}()

	var witnessed ChangeSet
	select {
	case got := <-sub.Changes():
		witnessed = got
	case <-time.After(testing.ShortWait):
	}

	c.Assert(witnessed, tc.HasLen, len(changes))
	c.Check(witnessed, tc.SameContents, changes)

	workertest.CleanKill(c, sub)
}

func (s *subscriptionSuite) TestSubscriptionDoesNotWitnessChangesWithCancelledContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0)
	defer workertest.DirtyKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "foo",
		changed: "1",
	}}

	syncPoint := make(chan struct{})
	go func() {
		defer close(syncPoint)

		ctx, cancel := context.WithCancel(c.Context())
		cancel()

		err := sub.dispatch(ctx, changes)
		c.Assert(err, tc.ErrorIs, context.Canceled)
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

func (s *subscriptionSuite) TestDispatchTimeoutKillsSubscription(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0)
	defer workertest.DirtyKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "foo",
		changed: "1",
	}}

	syncPoint := make(chan struct{})
	go func() {
		defer close(syncPoint)

		ctx, cancel := context.WithTimeout(c.Context(), time.Nanosecond)
		defer cancel()

		time.Sleep(time.Millisecond)

		err := sub.dispatch(ctx, changes)
		c.Assert(err, tc.ErrorIs, context.DeadlineExceeded)
	}()

	select {
	case <-syncPoint:
	case <-time.After(testing.ShortWait):
		c.Fatalf("failed waiting for sync point")
	}

	select {
	case <-sub.Done():
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for subscription to be killed")
	}

	workertest.CleanKill(c, sub)
}

func (s *subscriptionSuite) TestSubscriptionDoesNotWitnessChangesWithDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sub := newSubscription(0)
	defer workertest.DirtyKill(c, sub)

	changes := ChangeSet{changeEvent{
		ctype:   changestreamtesting.Create,
		ns:      "foo",
		changed: "1",
	}}

	syncPoint := make(chan struct{})
	go func() {
		defer close(syncPoint)

		sub.Kill()

		err := sub.dispatch(c.Context(), changes)
		c.Assert(err, tc.ErrorMatches, "tomb: dying")
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
