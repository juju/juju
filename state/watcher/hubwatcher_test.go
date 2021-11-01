// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type HubWatcherSuite struct {
	testing.BaseSuite

	w   watcher.BaseWatcher
	hub *pubsub.SimpleHub
	ch  chan watcher.Change

	clock *testclock.Clock
}

var _ = gc.Suite(&HubWatcherSuite{})

func (s *HubWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	logger := loggo.GetLogger("HubWatcherSuite")
	logger.SetLogLevel(loggo.TRACE)

	s.clock = testclock.NewClock(time.Now())

	s.hub = pubsub.NewSimpleHub(nil)
	s.ch = make(chan watcher.Change)
	var started <-chan struct{}
	s.w, started = watcher.NewTestHubWatcher(s.hub, s.clock, "model-uuid", logger)
	s.AddCleanup(func(c *gc.C) {
		worker.Stop(s.w)
	})
	select {
	case <-started:
		// all good
	case <-time.After(testing.LongWait):
		c.Error("hub watcher worker didn't start")
	}
}

func (s *HubWatcherSuite) publish(c *gc.C, changes ...watcher.Change) {
	var processed <-chan struct{}
	for _, change := range changes {
		processed = pubsub.Wait(s.hub.Publish(watcher.TxnWatcherCollection, change))
	}
	select {
	case <-processed:
		// all good.
	case <-time.After(testing.LongWait):
		c.Error("event not processed")
	}

}

func (s *HubWatcherSuite) TestErrAndDead(c *gc.C) {
	c.Assert(s.w.Err(), gc.Equals, tomb.ErrStillAlive)
	select {
	case <-s.w.Dead():
		c.Fatalf("Dead channel fired unexpectedly")
	default:
	}
	c.Assert(worker.Stop(s.w), jc.ErrorIsNil)
	select {
	case <-s.w.Dead():
	default:
		c.Fatalf("Dead channel should have fired")
	}
}

func (s *HubWatcherSuite) TestTxnWatcherSyncErrWorker(c *gc.C) {
	// When the TxnWatcher hits a sync error and restarts, the hub watcher needs
	// to restart too as there may be missed events, so all the watches this hub
	// has need to be invalidated. This happens by the worker dying.
	s.hub.Publish(watcher.TxnWatcherSyncErr, errors.New("boom"))

	select {
	case <-s.w.Dead():
	case <-time.After(testing.LongWait):
		c.Fatalf("Dead channel should have fired")
	}

	c.Assert(s.w.Err(), gc.ErrorMatches, "hub txn watcher sync error: boom")
}

func (s *HubWatcherSuite) TestWatchBeforeKnown(c *gc.C) {
	s.w.Watch("test", "a", s.ch)
	assertNoChange(c, s.ch)

	change := watcher.Change{"test", "a", 5}
	s.publish(c, change)

	assertChange(c, s.ch, change)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchAfterKnown(c *gc.C) {
	change := watcher.Change{"test", "a", 5}
	s.publish(c, change)

	// Watch doesn't publish an initial event, whether or not we've
	// seen the document before.
	s.w.Watch("test", "a", s.ch)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchIgnoreUnwatched(c *gc.C) {
	s.w.Watch("test", "a", s.ch)

	s.publish(c, watcher.Change{"test", "b", 5})

	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchMultiBeforeKnown(c *gc.C) {
	err := s.w.WatchMulti("test", []interface{}{"a", "b"}, s.ch)
	c.Assert(err, jc.ErrorIsNil)
	assertNoChange(c, s.ch)

	change := watcher.Change{"test", "a", 5}
	s.publish(c, change)

	assertChange(c, s.ch, change)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchMultiDuplicateWatch(c *gc.C) {
	s.w.Watch("test", "b", s.ch)
	assertNoChange(c, s.ch)
	err := s.w.WatchMulti("test", []interface{}{"a", "b"}, s.ch)
	c.Assert(err, gc.ErrorMatches, `tried to re-add channel .* for document "b" in collection "test"`)
	// Changes to "a" should not be watched as we had an error
	s.publish(c, watcher.Change{"test", "a", 5})
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchMultiInvalidId(c *gc.C) {
	err := s.w.WatchMulti("test", []interface{}{"a", nil}, s.ch)
	c.Assert(err, gc.ErrorMatches, `cannot watch a document with nil id`)
	// Changes to "a" should not be watched as we had an error
	s.publish(c, watcher.Change{"test", "a", 5})
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchMultiAfterKnown(c *gc.C) {
	s.publish(c, watcher.Change{"test", "a", 5})
	err := s.w.WatchMulti("test", []interface{}{"a", "b"}, s.ch)
	c.Assert(err, jc.ErrorIsNil)
	assertNoChange(c, s.ch)
	// We don't see the change that occurred before we started watching, but we see any changes after that fact
	change := watcher.Change{"test", "a", 6}
	s.publish(c, change)
	assertChange(c, s.ch, change)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchOrder(c *gc.C) {
	first := watcher.Change{"test", "a", 3}
	second := watcher.Change{"test", "b", 4}
	third := watcher.Change{"test", "c", 5}

	for _, id := range []string{"a", "b", "c", "d"} {
		s.w.Watch("test", id, s.ch)
	}

	s.publish(c, first, second, third)

	assertChange(c, s.ch, first)
	assertChange(c, s.ch, second)
	assertChange(c, s.ch, third)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchMultipleChannels(c *gc.C) {
	ch1 := make(chan watcher.Change)
	ch2 := make(chan watcher.Change)
	ch3 := make(chan watcher.Change)
	s.w.Watch("test1", 1, ch1)
	s.w.Watch("test2", 2, ch2)
	s.w.Watch("test3", 3, ch3)

	first := watcher.Change{"test1", 1, 3}
	second := watcher.Change{"test2", 2, 4}
	third := watcher.Change{"test3", 3, 5}
	s.publish(c, first, second, third)

	s.w.Unwatch("test2", 2, ch2)
	assertChange(c, ch1, first)
	assertChange(c, ch3, third)
	assertNoChange(c, ch1)
	assertNoChange(c, ch2)
	assertNoChange(c, ch3)
}

func (s *HubWatcherSuite) TestWatchAlreadyRemoved(c *gc.C) {
	change := watcher.Change{"test", "a", -1}
	s.publish(c, change)

	s.w.Watch("test", "a", s.ch)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchUnwatchOnQueue(c *gc.C) {
	const N = 10
	for i := 0; i < N; i++ {
		s.w.Watch("test", i, s.ch)
	}
	for i := 0; i < N; i++ {
		s.publish(c, watcher.Change{"test", i, int64(i + 3)})
	}
	for i := 1; i < N; i += 2 {
		s.w.Unwatch("test", i, s.ch)
	}
	seen := make(map[interface{}]bool)
	for i := 0; i < N/2; i++ {
		select {
		case change := <-s.ch:
			seen[change.Id] = true
		case <-time.After(worstCase):
			c.Fatalf("not enough changes: got %d, want %d", len(seen), N/2)
		}
	}
	c.Assert(len(seen), gc.Equals, N/2)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchCollection(c *gc.C) {
	chA1 := make(chan watcher.Change)
	chB1 := make(chan watcher.Change)
	chA := make(chan watcher.Change)
	chB := make(chan watcher.Change)

	s.w.Watch("testA", 1, chA1)
	s.w.Watch("testB", 1, chB1)
	s.w.WatchCollection("testA", chA)
	s.w.WatchCollection("testB", chB)

	changes := []watcher.Change{
		{"testA", 1, 3},
		{"testA", 2, 2},
		{"testB", 1, 5},
		{"testB", 2, 6},
	}
	s.publish(c, changes...)

	seen := map[chan<- watcher.Change][]watcher.Change{}

	waitForChanges := func(count int, seen map[chan<- watcher.Change][]watcher.Change, timeout time.Duration) {
		tooLong := time.After(timeout)
		for n := 0; n < count; n++ {
			select {
			case chg := <-chA1:
				seen[chA1] = append(seen[chA1], chg)
			case chg := <-chB1:
				seen[chB1] = append(seen[chB1], chg)
			case chg := <-chA:
				seen[chA] = append(seen[chA], chg)
			case chg := <-chB:
				seen[chB] = append(seen[chB], chg)
			case <-tooLong:
				return
			}
		}
	}

	waitForChanges(6, seen, testing.LongWait)

	c.Check(seen[chA1], jc.DeepEquals, []watcher.Change{changes[0]})
	c.Check(seen[chB1], jc.DeepEquals, []watcher.Change{changes[2]})
	c.Check(seen[chA], jc.DeepEquals, []watcher.Change{changes[0], changes[1]})
	c.Assert(seen[chB], jc.DeepEquals, []watcher.Change{changes[2], changes[3]})

	s.w.UnwatchCollection("testB", chB)
	s.w.Unwatch("testB", 1, chB1)

	next := watcher.Change{"testA", 1, 4}
	s.publish(c, next)

	seen = map[chan<- watcher.Change][]watcher.Change{}
	waitForChanges(2, seen, testing.LongWait)

	c.Check(seen[chA1], gc.DeepEquals, []watcher.Change{next})
	c.Check(seen[chB1], gc.IsNil)
	c.Check(seen[chA], gc.DeepEquals, []watcher.Change{next})
	c.Assert(seen[chB], gc.IsNil)

	// Check that no extra events arrive.
	seen = map[chan<- watcher.Change][]watcher.Change{}
	waitForChanges(1, seen, testing.ShortWait)

	c.Check(seen[chA1], gc.IsNil)
	c.Check(seen[chB1], gc.IsNil)
	c.Check(seen[chA], gc.IsNil)
	c.Check(seen[chB], gc.IsNil)
}

func (s *HubWatcherSuite) TestUnwatchCollectionWithFilter(c *gc.C) {
	filter := func(key interface{}) bool {
		id := key.(int)
		return id != 2
	}

	change := watcher.Change{"testA", 1, 3}
	s.w.WatchCollectionWithFilter("testA", s.ch, filter)
	s.publish(c, change)
	assertChange(c, s.ch, change)
	s.publish(c, watcher.Change{"testA", 2, 2})
	assertNoChange(c, s.ch)

	change = watcher.Change{"testA", 3, 3}
	s.publish(c, change)
	assertChange(c, s.ch, change)
}

func (s *HubWatcherSuite) TestWatchBeforeRemoveKnown(c *gc.C) {
	added := watcher.Change{"test", "a", 2}
	s.publish(c, added)

	s.w.Watch("test", "a", s.ch)

	removed := watcher.Change{"test", "a", -1}
	s.publish(c, removed)
	assertChange(c, s.ch, removed)
}

func (s *HubWatcherSuite) TestWatchStoppedWhileFlushing(c *gc.C) {
	first := watcher.Change{"test", "a", 2}
	second := watcher.Change{"test", "a", 3}

	s.w.Watch("test", "a", s.ch)

	s.publish(c, first)
	// The second event forces a reallocation of the slice in the
	// watcher.
	s.publish(c, second)
	// Unwatching should nil out the channel for all pending sync events.
	s.w.Unwatch("test", "a", s.ch)

	// Since we haven't removed anything off the channel before the
	// unwatch, all the pending events should be cleared.
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestDetectsDeadReceivers(c *gc.C) {
	logger := loggo.GetLogger("HubWatcherSuite")
	// Skip the trace logging.
	logger.SetLogLevel(loggo.CRITICAL)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("hubwatcher-tests", &tw), gc.IsNil)

	// Watch a and b, and publish changes for both of them.
	aCh := make(chan watcher.Change)

	s.w.Watch("test", "a", aCh)
	s.w.Watch("test", "b", s.ch)

	aEvent := watcher.Change{"test", "a", 2}
	bEvent := watcher.Change{"test", "b", 3}
	s.publish(c, aEvent)
	s.publish(c, bEvent)

	assertChange(c, aCh, aEvent)
	assertChange(c, s.ch, bEvent)

	// Stop receiving a changes - publish a and b changes again.
	aEvent = watcher.Change{"test", "a", 22}
	bEvent = watcher.Change{"test", "b", 33}
	s.publish(c, aEvent)
	s.publish(c, bEvent)

	assertNoChange(c, s.ch)
	// 3 waiters - the first two messages and then the blocked one.
	err := s.clock.WaitAdvance(10*time.Second, testing.LongWait, 3)
	c.Assert(err, jc.ErrorIsNil)

	// Eventually the watcher gives up on trying to send the a change.
	// Still sends the b change.
	assertChange(c, s.ch, bEvent)

	// And complains about the receiver that's not listening (with
	// stack trace).
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.CRITICAL,
		`0x.......... programming error, e.ch=0x.......... did not accept {test a 22} - missing Unwatch\?\nwatch source:\ngoroutine .*`,
	}})
}

func (s *HubWatcherSuite) TestWatchMultiDeadReceivers(c *gc.C) {
	logger := loggo.GetLogger("HubWatcherSuite")
	logger.SetLogLevel(loggo.CRITICAL)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("hubwatcher-tests", &tw), gc.IsNil)

	aCh := make(chan watcher.Change)
	err := s.w.WatchMulti("test", []interface{}{"a", "b"}, aCh)
	c.Assert(err, jc.ErrorIsNil)

	s.w.Watch("test", "b", s.ch)

	event := watcher.Change{"test", "b", 3}
	s.publish(c, event)

	assertNoChange(c, s.ch)
	err = s.clock.WaitAdvance(10*time.Second, testing.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	// Eventually the watcher gives up on trying to send to the first one.
	// Still sends the b change to the next watch.
	assertChange(c, s.ch, event)

	// And complains about the receiver that's not listening (with
	// stack trace).
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.CRITICAL,
		`0x.......... programming error, e.ch=0x.......... did not accept {test b 3} - missing Unwatch\?\nwatch source:\ngoroutine .*`,
	}})
}

func (s *HubWatcherSuite) TestWatchCollectionDeadReceivers(c *gc.C) {
	logger := loggo.GetLogger("HubWatcherSuite")
	logger.SetLogLevel(loggo.CRITICAL)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("hubwatcher-tests", &tw), gc.IsNil)

	aCh := make(chan watcher.Change)
	s.w.WatchCollection("test", aCh)
	s.w.Watch("test", "b", s.ch)

	event := watcher.Change{"test", "b", 3}
	s.publish(c, event)

	assertNoChange(c, s.ch)
	err := s.clock.WaitAdvance(10*time.Second, testing.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	// Eventually the watcher gives up on trying to send to the first one.
	// Still sends the b change to the next watch.
	assertChange(c, s.ch, event)

	// And complains about the receiver that's not listening (with
	// stack trace).
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.CRITICAL,
		`0x.......... programming error, e.ch=0x.......... did not accept {test b 3} - missing Unwatch\?\nwatch source:\ngoroutine .*`,
	}})
}
