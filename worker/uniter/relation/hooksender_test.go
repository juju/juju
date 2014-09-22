// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"gopkg.in/juju/charm.v3/hooks"
	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

type HookSenderSuite struct{}

var _ = gc.Suite(&HookSenderSuite{})

func assertNext(c *gc.C, out chan hook.Info, expect hook.Info) {
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %v", expect)
	case actual, ok := <-out:
		c.Assert(ok, jc.IsTrue)
		c.Assert(actual, gc.Equals, expect)
	}
}

func assertEmpty(c *gc.C, out chan hook.Info) {
	select {
	case <-time.After(coretesting.ShortWait):
	case actual, ok := <-out:
		c.Fatalf("got unexpected %v %v", actual, ok)
	}
}

func hookList(kinds ...hooks.Kind) []hook.Info {
	result := make([]hook.Info, len(kinds))
	for i, kind := range kinds {
		result[i].Kind = kind
	}
	return result
}

func (s *HookSenderSuite) TestSendsHooks(c *gc.C) {
	expect := hookList(hooks.Install, hooks.ConfigChanged, hooks.Start)
	source := relation.NewListSource(expect)
	out := make(chan hook.Info)
	sender := relation.NewHookSender(out, source)
	defer statetesting.AssertStop(c, sender)

	for i := range expect {
		assertNext(c, out, expect[i])
	}
	assertEmpty(c, out)
	statetesting.AssertStop(c, sender)
	c.Assert(source.Empty(), jc.IsTrue)
}

func (s *HookSenderSuite) TestStopsHooks(c *gc.C) {
	expect := hookList(hooks.Install, hooks.ConfigChanged, hooks.Start)
	source := relation.NewListSource(expect)
	out := make(chan hook.Info)
	sender := relation.NewHookSender(out, source)
	defer statetesting.AssertStop(c, sender)

	assertNext(c, out, expect[0])
	assertNext(c, out, expect[1])
	statetesting.AssertStop(c, sender)
	assertEmpty(c, out)
	c.Assert(source.Next(), gc.Equals, expect[2])
}

func (s *HookSenderSuite) TestHandlesUpdatesFullQueue(c *gc.C) {
	source := &updateSource{
		changes: make(chan params.RelationUnitsChange),
		updates: make(chan params.RelationUnitsChange),
	}
	out := make(chan hook.Info)
	sender := relation.NewHookSender(out, source)
	defer statetesting.AssertStop(c, sender)
	defer source.tomb.Done()

	// Check we're being sent hooks but not updates.
	assertActive := func() {
		assertNext(c, out, hook.Info{Kind: hooks.Install})
		select {
		case update, ok := <-source.updates:
			c.Fatalf("got unexpected update: %v %v", update, ok)
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertActive()

	// Send an event on the Changes() chan.
	sent := params.RelationUnitsChange{Departed: []string{"sent"}}
	select {
	case source.changes <- sent:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("could not send change")
	}

	// Now that a change has been delivered, nothing should be sent on the out
	// chan, or read from the changes chan, until the Update method has completed.
	notSent := params.RelationUnitsChange{Departed: []string{"notSent"}}
	select {
	case source.changes <- notSent:
		c.Fatalf("sent extra change while updating queue")
	case hi, ok := <-out:
		c.Fatalf("got unexpected hook while updating queue: %v %v", hi, ok)
	case got, ok := <-source.updates:
		c.Assert(ok, jc.IsTrue)
		c.Assert(got, gc.DeepEquals, sent)
	case <-time.After(coretesting.ShortWait):
	}

	// Check we're still being sent hooks and not updates.
	assertActive()
}

func (s *HookSenderSuite) TestHandlesUpdatesFullQueueSpam(c *gc.C) {
	source := &updateSource{
		changes: make(chan params.RelationUnitsChange),
		updates: make(chan params.RelationUnitsChange),
	}
	out := make(chan hook.Info)
	sender := relation.NewHookSender(out, source)
	defer statetesting.AssertStop(c, sender)
	defer source.tomb.Done()

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
		case source.changes <- params.RelationUnitsChange{}:
			changeCount++
		case update, ok := <-source.updates:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.DeepEquals, params.RelationUnitsChange{})
			updateCount++
		case <-timeout:
			c.Fatalf("not enough things happened in time")
		}
	}

	// Check sane end state.
	c.Check(hookCount, gc.Not(gc.Equals), 0)
	c.Check(changeCount, gc.Not(gc.Equals), 0)
	c.Check(updateCount, gc.Not(gc.Equals), 0)
	unhandledChanges := changeCount - updateCount
	c.Check(unhandledChanges >= 0, jc.IsTrue)
	c.Check(unhandledChanges <= 1, jc.IsTrue)
}

func (s *HookSenderSuite) TestHandlesUpdatesEmptyQueue(c *gc.C) {
	source := &updateSource{
		empty:   true,
		changes: make(chan params.RelationUnitsChange),
		updates: make(chan params.RelationUnitsChange),
	}
	out := make(chan hook.Info)
	sender := relation.NewHookSender(out, source)
	defer statetesting.AssertStop(c, sender)
	defer source.tomb.Done()

	// Check no hooks are sent and no updates delivered.
	assertIdle := func() {
		select {
		case hi, ok := <-out:
			c.Fatalf("got unexpected hook: %v %v", hi, ok)
		case update, ok := <-source.updates:
			c.Fatalf("got unexpected update: %v %v", update, ok)
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertIdle()

	// Send an event on the Changes() chan.
	sent := params.RelationUnitsChange{Departed: []string{"sent"}}
	select {
	case source.changes <- sent:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("could not send change")
	}

	// Now that a change has been delivered, nothing should be sent on the out
	// chan, or read from the changes chan, until the Update method has completed.
	notSent := params.RelationUnitsChange{Departed: []string{"notSent"}}
	select {
	case source.changes <- notSent:
		c.Fatalf("sent extra update while updating queue")
	case hi, ok := <-out:
		c.Fatalf("got unexpected hook while updating queue: %v %v", hi, ok)
	case got, ok := <-source.updates:
		c.Assert(ok, jc.IsTrue)
		c.Assert(got, gc.DeepEquals, sent)
	case <-time.After(coretesting.ShortWait):
	}

	// Now the change has been delivered, nothing should be happening.
	assertIdle()
}

func (s *HookSenderSuite) TestHandlesUpdatesEmptyQueueSpam(c *gc.C) {
	source := &updateSource{
		empty:   true,
		changes: make(chan params.RelationUnitsChange),
		updates: make(chan params.RelationUnitsChange),
	}
	out := make(chan hook.Info)
	sender := relation.NewHookSender(out, source)
	defer statetesting.AssertStop(c, sender)
	defer source.tomb.Done()

	// Spam all channels continuously for a bit.
	timeout := time.After(coretesting.LongWait)
	changeCount := 0
	updateCount := 0
	for i := 0; i < 100; i++ {
		select {
		case hi, ok := <-out:
			c.Fatalf("got unexpected hook: %v %v", hi, ok)
		case source.changes <- params.RelationUnitsChange{}:
			changeCount++
		case update, ok := <-source.updates:
			c.Assert(ok, jc.IsTrue)
			c.Assert(update, gc.DeepEquals, params.RelationUnitsChange{})
			updateCount++
		case <-timeout:
			c.Fatalf("not enough things happened in time")
		}
	}

	// Check sane end state.
	c.Check(changeCount, gc.Equals, 50)
	c.Check(updateCount, gc.Equals, 50)
}

type updateSource struct {
	tomb    tomb.Tomb
	empty   bool
	changes chan params.RelationUnitsChange
	updates chan params.RelationUnitsChange
}

func (source *updateSource) Stop() error {
	source.tomb.Kill(nil)
	return source.tomb.Wait()
}

func (source *updateSource) Changes() <-chan params.RelationUnitsChange {
	return source.changes
}

func (source *updateSource) Update(change params.RelationUnitsChange) error {
	select {
	case <-time.After(coretesting.LongWait):
		// We don't really care whether the update is collected, but we want to
		// give it every reasonable chance to be.
	case source.updates <- change:
	}
	return nil
}

func (source *updateSource) Empty() bool {
	return source.empty
}

func (source *updateSource) Next() hook.Info {
	if source.empty {
		panic(nil)
	}
	return hook.Info{Kind: hooks.Install}
}

func (source *updateSource) Pop() {
	if source.empty {
		panic(nil)
	}
}
