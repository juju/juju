// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hook_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5/hooks"

	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/hook/hooktesting"
)

type PeekerSuite struct {
}

var _ = gc.Suite(&PeekerSuite{})

func (s *PeekerSuite) TestPeeks(c *gc.C) {
	expect := hooktesting.HookList(hooks.Install, hooks.ConfigChanged, hooks.Start)
	source := hook.NewListSource(expect)
	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	timeout := time.After(coretesting.LongWait)
	for _, expectInfo := range expect {
		select {
		case peek, ok := <-peeker.Peeks():
			c.Assert(ok, jc.IsTrue)
			c.Assert(peek.HookInfo(), gc.Equals, expectInfo)
			peek.Reject()
		case <-timeout:
			c.Fatalf("ran out of time")
		}
		select {
		case peek, ok := <-peeker.Peeks():
			c.Assert(ok, jc.IsTrue)
			c.Assert(peek.HookInfo(), gc.Equals, expectInfo)
			peek.Consume()
		case <-timeout:
			c.Fatalf("ran out of time")
		}
	}
	select {
	case <-time.After(coretesting.ShortWait):
	case peek, ok := <-peeker.Peeks():
		c.Fatalf("unexpected peek from empty queue: %#v, %#v", peek, ok)
	}
}

func (s *PeekerSuite) TestStopWhileRunning(c *gc.C) {
	source := hooktesting.NewFullUnbufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Grab and reject a peek to check we're running.
	select {
	case peek, ok := <-peeker.Peeks():
		c.Assert(ok, jc.IsTrue)
		c.Assert(peek.HookInfo(), gc.Equals, hook.Info{Kind: hooks.Install})
		peek.Reject()
		assertStopPeeker(c, peeker, source)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (s *PeekerSuite) TestStopWhilePeeking(c *gc.C) {
	source := hooktesting.NewFullUnbufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	select {
	case peek, ok := <-peeker.Peeks():
		c.Assert(ok, jc.IsTrue)
		c.Assert(peek.HookInfo(), gc.Equals, hook.Info{Kind: hooks.Install})
		assertStopPeeker(c, peeker, source)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (s *PeekerSuite) TestStopWhileUpdating(c *gc.C) {
	source := hooktesting.NewFullBufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Deliver a change; do not accept updates.
	select {
	case source.ChangesC <- source.NewChange(nil):
		assertStopPeeker(c, peeker, source)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (s *PeekerSuite) TestUpdatesFullQueue(c *gc.C) {
	source := hooktesting.NewFullUnbufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Check we're being sent peeks but not updates.
	timeout := time.After(coretesting.LongWait)
	assertActive := func() {
		select {
		case peek, ok := <-peeker.Peeks():
			c.Assert(ok, jc.IsTrue)
			defer peek.Reject()
			c.Assert(peek.HookInfo(), gc.Equals, hook.Info{Kind: hooks.Install})
		case update, ok := <-source.UpdatesC:
			c.Fatalf("got unexpected update: %#v %#v", update, ok)
		case <-timeout:
			c.Fatalf("timed out")
		}
	}
	assertActive()

	// Send an event on the Changes() chan.
	select {
	case source.ChangesC <- source.NewChange("sent"):
	case <-timeout:
		c.Fatalf("could not send change")
	}

	// Now that a change has been delivered, nothing should be sent on the out
	// chan, or read from the changes chan, until the Update method has completed.
	select {
	case source.ChangesC <- source.NewChange("notSent"):
		c.Fatalf("sent extra change while updating queue")
	case peek, ok := <-peeker.Peeks():
		c.Fatalf("got unexpected peek while updating queue: %#v %#v", peek, ok)
	case got, ok := <-source.UpdatesC:
		c.Assert(ok, jc.IsTrue)
		c.Assert(got, gc.Equals, "sent")
	case <-timeout:
		c.Fatalf("timed out")
	}

	// Check we're still being sent hooks and not updates.
	assertActive()
}

func (s *PeekerSuite) TestUpdatesFullQueueSpam(c *gc.C) {
	source := hooktesting.NewFullUnbufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Spam all channels continuously for a bit.
	timeout := time.After(coretesting.LongWait)
	peekCount := 0
	changeCount := 0
	updateCount := 0
	for i := 0; i < 100; i++ {
		select {
		case peek, ok := <-peeker.Peeks():
			c.Assert(ok, jc.IsTrue)
			c.Assert(peek.HookInfo(), gc.DeepEquals, hook.Info{Kind: hooks.Install})
			peek.Consume()
			peekCount++
		case source.ChangesC <- source.NewChange("!"):
			changeCount++
		case update, ok := <-source.UpdatesC:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.Equals, "!")
			updateCount++
		case <-timeout:
			c.Fatalf("not enough things happened in time")
		}
	}

	// Once we've finished sending, exhaust the updates...
	for i := updateCount; i < changeCount && updateCount < changeCount; i++ {
		select {
		case update, ok := <-source.UpdatesC:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.DeepEquals, "!")
			updateCount++
		case <-timeout:
			c.Fatalf("expected %d updates, got %d", changeCount, updateCount)
		}
	}

	// ...and check sane end state to validate the foregoing.
	c.Check(peekCount, gc.Not(gc.Equals), 0)
	c.Check(changeCount, gc.Not(gc.Equals), 0)
}

func (s *PeekerSuite) TestUpdatesEmptyQueue(c *gc.C) {
	source := hooktesting.NewEmptySource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Check no hooks are sent and no updates delivered.
	assertIdle := func() {
		select {
		case peek, ok := <-peeker.Peeks():
			c.Fatalf("got unexpected peek: %#v %#v", peek, ok)
		case update, ok := <-source.UpdatesC:
			c.Fatalf("got unexpected update: %#v %#v", update, ok)
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertIdle()

	// Send an event on the Changes() chan.
	timeout := time.After(coretesting.LongWait)
	select {
	case source.ChangesC <- source.NewChange("sent"):
	case <-timeout:
		c.Fatalf("timed out")
	}

	// Now that a change has been delivered, nothing should be sent on the out
	// chan, or read from the changes chan, until the Update method has completed.
	select {
	case source.ChangesC <- source.NewChange("notSent"):
		c.Fatalf("sent extra change while updating queue")
	case peek, ok := <-peeker.Peeks():
		c.Fatalf("got unexpected peek while updating queue: %#v %#v", peek, ok)
	case got, ok := <-source.UpdatesC:
		c.Assert(ok, jc.IsTrue)
		c.Assert(got, gc.Equals, "sent")
	case <-timeout:
		c.Fatalf("timed out")
	}

	// Now the change has been delivered, nothing should be happening.
	assertIdle()
}

func (s *PeekerSuite) TestUpdatesEmptyQueueSpam(c *gc.C) {
	source := hooktesting.NewEmptySource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Spam all channels continuously for a bit.
	timeout := time.After(coretesting.LongWait)
	changeCount := 0
	updateCount := 0
	for i := 0; i < 100; i++ {
		select {
		case peek, ok := <-peeker.Peeks():
			c.Fatalf("got unexpected peek: %#v %#v", peek, ok)
		case source.ChangesC <- source.NewChange("!"):
			changeCount++
		case update, ok := <-source.UpdatesC:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.Equals, "!")
			updateCount++
		case <-timeout:
			c.Fatalf("not enough things happened in time")
		}
	}

	// Check sane end state.
	c.Check(changeCount, gc.Equals, 50)
	c.Check(updateCount, gc.Equals, 50)
}

func (s *PeekerSuite) TestPeeksBlockUntilRejected(c *gc.C) {
	source := hooktesting.NewFullBufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Collect a peek...
	timeout := time.After(coretesting.LongWait)
	select {
	case <-timeout:
		c.Fatalf("failed to receive peek")
	case peek, ok := <-peeker.Peeks():
		c.Assert(ok, jc.IsTrue)
		c.Assert(peek.HookInfo(), gc.Equals, hook.Info{Kind: hooks.Install})

		// ...and check that changes can't be delivered...
		select {
		case source.ChangesC <- source.NewChange(nil):
			c.Fatalf("delivered change while supposedly peeking")
		default:
		}

		// ...before the peek is rejected, at which point changes are unblocked.
		peek.Reject()
		select {
		case source.ChangesC <- source.NewChange(nil):
		case <-timeout:
			c.Fatalf("failed to unblock changes")
		}
	}
}

func (s *PeekerSuite) TestPeeksBlockUntilConsumed(c *gc.C) {
	source := hooktesting.NewFullBufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Collect a peek...
	timeout := time.After(coretesting.LongWait)
	select {
	case <-timeout:
		c.Fatalf("failed to receive peek")
	case peek, ok := <-peeker.Peeks():
		c.Assert(ok, jc.IsTrue)
		c.Assert(peek.HookInfo(), gc.Equals, hook.Info{Kind: hooks.Install})

		// ...and check that changes can't be delivered...
		select {
		case source.ChangesC <- source.NewChange(nil):
			c.Fatalf("delivered change while supposedly peeking")
		default:
		}

		// ...before the peek is consumed, at which point changes are unblocked.
		peek.Consume()
		select {
		case source.ChangesC <- source.NewChange(nil):
		case <-timeout:
			c.Fatalf("failed to unblock changes")
		}
	}
}

func (s *PeekerSuite) TestBlocksWhenUpdating(c *gc.C) {
	source := hooktesting.NewFullUnbufferedSource()
	defer statetesting.AssertStop(c, source)

	peeker := hook.NewPeeker(source)
	defer statetesting.AssertStop(c, peeker)

	// Deliver a change.
	timeout := time.After(coretesting.LongWait)
	select {
	case source.ChangesC <- source.NewChange("sent"):
	case <-timeout:
		c.Fatalf("failed to send change")
	}

	// Check peeks are not delivered...
	select {
	case peek, ok := <-peeker.Peeks():
		c.Fatalf("got unexpected peek: %#v, %#v", peek, ok)
	default:
	}

	// ...before the update is handled...
	select {
	case update, ok := <-source.UpdatesC:
		c.Assert(ok, jc.IsTrue)
		c.Assert(update, gc.Equals, "sent")
	case <-timeout:
		c.Fatalf("failed to collect update")
	}

	// ...at which point peeks are expected again.
	select {
	case peek, ok := <-peeker.Peeks():
		c.Assert(ok, jc.IsTrue)
		c.Assert(peek.HookInfo(), gc.Equals, hook.Info{Kind: hooks.Install})
		peek.Reject()
	case <-timeout:
		c.Fatalf("failed to send peek")
	}
}

func assertStopPeeker(c *gc.C, peeker hook.Peeker, source *hooktesting.UpdateSource) {
	// Stop the peeker...
	err := peeker.Stop()
	c.Assert(err, gc.IsNil)

	// ...and check it closed the channel...
	select {
	case peek, ok := <-peeker.Peeks():
		c.Assert(peek, gc.IsNil)
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("peeks channel not closed")
	}

	// ...and stopped the source.
	select {
	case <-source.Tomb.Dead():
		c.Assert(source.Tomb.Err(), jc.ErrorIsNil)
	default:
		c.Fatalf("source not stopped")
	}
}
