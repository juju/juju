// Copyright 2012-2015 Canonical Ltd.
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

type HookSenderSuite struct{}

var _ = gc.Suite(&HookSenderSuite{})

func assertNext(c *gc.C, out chan hook.Info, expect hook.Info) {
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %#v", expect)
	case actual, ok := <-out:
		c.Assert(ok, jc.IsTrue)
		c.Assert(actual, gc.Equals, expect)
	}
}

func assertEmpty(c *gc.C, out chan hook.Info) {
	select {
	case <-time.After(coretesting.ShortWait):
	case actual, ok := <-out:
		c.Fatalf("got unexpected %#v %#v", actual, ok)
	}
}

func (s *HookSenderSuite) TestSendsHooks(c *gc.C) {
	expect := hooktesting.HookList(hooks.Install, hooks.ConfigChanged, hooks.Start)
	source := hook.NewListSource(expect)
	out := make(chan hook.Info)
	sender := hook.NewSender(out, source)
	defer statetesting.AssertStop(c, sender)

	for i := range expect {
		assertNext(c, out, expect[i])
	}
	assertEmpty(c, out)
	statetesting.AssertStop(c, sender)
	c.Assert(source.Empty(), jc.IsTrue)
}

func (s *HookSenderSuite) TestStopsHooks(c *gc.C) {
	expect := hooktesting.HookList(hooks.Install, hooks.ConfigChanged, hooks.Start)
	source := hook.NewListSource(expect)
	out := make(chan hook.Info)
	sender := hook.NewSender(out, source)
	defer statetesting.AssertStop(c, sender)

	assertNext(c, out, expect[0])
	assertNext(c, out, expect[1])
	statetesting.AssertStop(c, sender)
	assertEmpty(c, out)
	c.Assert(source.Next(), gc.Equals, expect[2])
}

func (s *HookSenderSuite) TestHandlesUpdatesFullQueue(c *gc.C) {
	source := hooktesting.NewFullUnbufferedSource()
	defer statetesting.AssertStop(c, source)

	out := make(chan hook.Info)
	sender := hook.NewSender(out, source)
	defer statetesting.AssertStop(c, sender)

	// Check we're being sent hooks but not updates.
	assertActive := func() {
		assertNext(c, out, hook.Info{Kind: hooks.Install})
		select {
		case update, ok := <-source.UpdatesC:
			c.Fatalf("got unexpected update: %#v %#v", update, ok)
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertActive()

	// Send an event on the Changes() chan.
	select {
	case source.ChangesC <- source.NewChange("sent"):
	case <-time.After(coretesting.LongWait):
		c.Fatalf("could not send change")
	}

	// Now that a change has been delivered, nothing should be sent on the out
	// chan, or read from the changes chan, until the Update method has completed.
	select {
	case source.ChangesC <- source.NewChange("notSent"):
		c.Fatalf("sent extra change while updating queue")
	case hi, ok := <-out:
		c.Fatalf("got unexpected hook while updating queue: %#v %#v", hi, ok)
	case got, ok := <-source.UpdatesC:
		c.Assert(ok, jc.IsTrue)
		c.Assert(got, gc.Equals, "sent")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}

	// Check we're still being sent hooks and not updates.
	assertActive()
}

func (s *HookSenderSuite) TestHandlesUpdatesFullQueueSpam(c *gc.C) {
	source := hooktesting.NewFullBufferedSource()
	defer statetesting.AssertStop(c, source)

	out := make(chan hook.Info)
	sender := hook.NewSender(out, source)
	defer statetesting.AssertStop(c, sender)

	// Spam all channels continuously for a bit.
	timeout := time.After(coretesting.LongWait)
	hookCount := 0
	changeCount := 0
	updateCount := 0
	for i := 0; i < 100; i++ {
		select {
		case hi, ok := <-out:
			c.Assert(ok, jc.IsTrue)
			c.Assert(hi, gc.DeepEquals, hook.Info{Kind: hooks.Install})
			hookCount++
		case source.ChangesC <- source.NewChange("sent"):
			changeCount++
		case update, ok := <-source.UpdatesC:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.Equals, "sent")
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
			c.Assert(update, gc.Equals, "sent")
			updateCount++
		case <-timeout:
			c.Fatalf("expected %d updates, got %d", changeCount, updateCount)
		}
	}

	// ...and check sane end state to validate the foregoing.
	c.Check(hookCount, gc.Not(gc.Equals), 0)
	c.Check(changeCount, gc.Not(gc.Equals), 0)
}

func (s *HookSenderSuite) TestHandlesUpdatesEmptyQueue(c *gc.C) {
	source := hooktesting.NewEmptySource()
	defer statetesting.AssertStop(c, source)

	out := make(chan hook.Info)
	sender := hook.NewSender(out, source)
	defer statetesting.AssertStop(c, sender)

	// Check no hooks are sent and no updates delivered.
	assertIdle := func() {
		select {
		case hi, ok := <-out:
			c.Fatalf("got unexpected hook: %#v %#v", hi, ok)
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
		c.Fatalf("sent extra update while updating queue")
	case hi, ok := <-out:
		c.Fatalf("got unexpected hook while updating queue: %#v %#v", hi, ok)
	case got, ok := <-source.UpdatesC:
		c.Assert(ok, jc.IsTrue)
		c.Assert(got, gc.Equals, "sent")
	case <-timeout:
		c.Fatalf("timed out")
	}

	// Now the change has been delivered, nothing should be happening.
	assertIdle()
}

func (s *HookSenderSuite) TestHandlesUpdatesEmptyQueueSpam(c *gc.C) {
	source := hooktesting.NewEmptySource()
	defer statetesting.AssertStop(c, source)

	out := make(chan hook.Info)
	sender := hook.NewSender(out, source)
	defer statetesting.AssertStop(c, sender)

	// Spam all channels continuously for a bit.
	timeout := time.After(coretesting.LongWait)
	changeCount := 0
	updateCount := 0
	for i := 0; i < 100; i++ {
		select {
		case hi, ok := <-out:
			c.Fatalf("got unexpected hook: %#v %#v", hi, ok)
		case source.ChangesC <- source.NewChange("sent"):
			changeCount++
		case update, ok := <-source.UpdatesC:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.Equals, "sent")
			updateCount++
		case <-timeout:
			c.Fatalf("not enough things happened in time")
		}
	}

	// Check sane end state.
	c.Check(changeCount, gc.Equals, 50)
	c.Check(updateCount, gc.Equals, 50)
}
