// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type HubWatcherSuite struct {
	testing.BaseSuite

	w   watcher.BaseWatcher
	hub *pubsub.SimpleHub
	ch  chan watcher.Change
}

var _ = gc.Suite(&HubWatcherSuite{})

func (s *HubWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	logger := loggo.GetLogger("HubWatcherSuite")
	logger.SetLogLevel(loggo.TRACE)

	s.hub = pubsub.NewSimpleHub(nil)
	s.ch = make(chan watcher.Change)
	var started <-chan struct{}
	s.w, started = watcher.NewTestHubWatcher(s.hub, clock.WallClock, "model-uuid", logger)
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
		processed = s.hub.Publish(watcher.TxnWatcherCollection, change)
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
	s.hub.Publish(watcher.TxnWatcherSyncErr, nil)

	select {
	case <-s.w.Dead():
	case <-time.After(testing.LongWait):
		c.Fatalf("Dead channel should have fired")
	}

	c.Assert(s.w.Err(), gc.ErrorMatches, "txn watcher sync error")
}

func (s *HubWatcherSuite) TestWatchBeforeKnown(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	change := watcher.Change{"test", "a", 5}
	s.publish(c, change)

	assertChange(c, s.ch, change)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchAfterKnown(c *gc.C) {
	change := watcher.Change{"test", "a", 5}
	s.publish(c, change)

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, change)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchIgnoreUnwatched(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)

	s.publish(c, watcher.Change{"test", "b", 5})

	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchOrder(c *gc.C) {
	first := watcher.Change{"test", "a", 3}
	second := watcher.Change{"test", "b", 4}
	third := watcher.Change{"test", "c", 5}

	for _, id := range []string{"a", "b", "c", "d"} {
		s.w.Watch("test", id, -1, s.ch)
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
	s.w.Watch("test1", 1, -1, ch1)
	s.w.Watch("test2", 2, -1, ch2)
	s.w.Watch("test3", 3, -1, ch3)

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

func (s *HubWatcherSuite) TestWatchKnownRemove(c *gc.C) {
	change := watcher.Change{"test", "a", -1}
	s.publish(c, change)

	s.w.Watch("test", "a", 2, s.ch)
	assertChange(c, s.ch, change)
	assertNoChange(c, s.ch)
}

func (s *HubWatcherSuite) TestWatchUnwatchOnQueue(c *gc.C) {
	const N = 10
	for i := 0; i < N; i++ {
		s.publish(c, watcher.Change{"test", i, int64(i + 3)})
	}
	for i := 0; i < N; i++ {
		s.w.Watch("test", i, -1, s.ch)
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

	s.w.Watch("testA", 1, -1, chA1)
	s.w.Watch("testB", 1, -1, chB1)
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

	s.w.Watch("test", "a", -1, s.ch)

	removed := watcher.Change{"test", "a", -1}
	s.publish(c, removed)

	assertChange(c, s.ch, added)
	assertChange(c, s.ch, removed)
}

func (s *HubWatcherSuite) TestWatchStoppedWhileFlushing(c *gc.C) {
	first := watcher.Change{"test", "a", 2}
	second := watcher.Change{"test", "a", 3}

	s.w.Watch("test", "a", -1, s.ch)

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
