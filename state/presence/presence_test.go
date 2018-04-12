// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type PresenceSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	presence *mgo.Collection
	pings    *mgo.Collection
	modelTag names.ModelTag
}

var _ = gc.Suite(&PresenceSuite{})

func (s *PresenceSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.modelTag = names.NewModelTag(uuid.String())
}

func (s *PresenceSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *PresenceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("presence")
	s.presence = db.C("presence")
	s.pings = db.C("presence.pings")

	presence.FakeTimeSlot(0)
}

func (s *PresenceSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)

	presence.RealTimeSlot()
	presence.RealPeriod()
}

func assertChange(c *gc.C, watch <-chan presence.Change, want presence.Change) {
	select {
	case got := <-watch:
		if got != want {
			c.Fatalf("watch reported %v, want %v", got, want)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("watch reported nothing, want %v", want)
	}
}

func assertNoChange(c *gc.C, watch <-chan presence.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(testing.ShortWait):
	}
}

func assertAlive(c *gc.C, w *presence.Watcher, key string, expAlive bool) {
	realAlive, err := w.Alive(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(realAlive, gc.Equals, expAlive)
}

// assertStopped stops a worker and waits until it reports stopped.
// Use this method in favor of defer w.Stop() because you _must_ ensure
// that the worker has stopped, and thus is no longer using its mgo
// session before TearDownTest shuts down the connection.
func assertStopped(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}

func (s *PresenceSuite) TestErrAndDead(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	defer assertStopped(c, w)

	c.Assert(errors.Cause(w.Err()), gc.Equals, tomb.ErrStillAlive)
	select {
	case <-w.Dead():
		c.Fatalf("Dead channel fired unexpectedly")
	default:
	}
	c.Assert(w.Stop(), gc.IsNil)
	c.Assert(w.Err(), gc.IsNil)
	select {
	case <-w.Dead():
	default:
		c.Fatalf("Dead channel should have fired")
	}
}

func (s *PresenceSuite) getDirectRecorder() presence.PingRecorder {
	return presence.DirectRecordFunc(s.presence)
}

func (s *PresenceSuite) TestAliveError(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	c.Assert(w.Stop(), gc.IsNil)

	alive, err := w.Alive("a")
	c.Assert(err, gc.ErrorMatches, ".*: watcher is dying")
	c.Assert(alive, jc.IsFalse)
	w.Wait()
}

func (s *PresenceSuite) TestWorkflow(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	pa := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	pb := presence.NewPinger(s.presence, s.modelTag, "b", s.getDirectRecorder)
	defer assertStopped(c, w)
	defer assertStopped(c, pa)
	defer assertStopped(c, pb)

	assertAlive(c, w, "a", false)
	assertAlive(c, w, "b", false)

	// Buffer one entry to avoid blocking the watcher here.
	cha := make(chan presence.Change, 1)
	chb := make(chan presence.Change, 1)
	w.Watch("a", cha)
	w.Watch("b", chb)

	// Initial events with current status.
	assertChange(c, cha, presence.Change{"a", false})
	assertChange(c, chb, presence.Change{"b", false})

	w.StartSync()
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	c.Assert(pa.Start(), gc.IsNil)

	w.StartSync()
	assertChange(c, cha, presence.Change{"a", true})
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	assertAlive(c, w, "a", true)
	assertAlive(c, w, "b", false)

	// Changes while the channel is out are not observed.
	w.Unwatch("a", cha)
	assertNoChange(c, cha)
	pa.KillForTesting()
	w.Sync()
	pa = presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	pa.Start()
	w.StartSync()
	assertNoChange(c, cha)

	// We can still query it manually, though.
	assertAlive(c, w, "a", true)
	assertAlive(c, w, "b", false)

	// Initial positive event. No refresh needed.
	w.Watch("a", cha)
	assertChange(c, cha, presence.Change{"a", true})

	c.Assert(pb.Start(), gc.IsNil)

	w.StartSync()
	assertChange(c, chb, presence.Change{"b", true})
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	c.Assert(pa.Stop(), gc.IsNil)

	w.StartSync()
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	// pb is running, pa isn't.
	c.Assert(pa.KillForTesting(), gc.IsNil)
	c.Assert(pb.KillForTesting(), gc.IsNil)

	w.StartSync()
	assertChange(c, cha, presence.Change{"a", false})
	assertChange(c, chb, presence.Change{"b", false})

	assertStopped(c, w)
}

func (s *PresenceSuite) TestScale(c *gc.C) {
	const N = 1000
	var ps []*presence.Pinger
	defer func() {
		for _, p := range ps {
			p.Stop()
		}
	}()

	c.Logf("Starting %d pingers...", N)
	for i := 0; i < N; i++ {
		p := presence.NewPinger(s.presence, s.modelTag, strconv.Itoa(i), s.getDirectRecorder)
		c.Assert(p.Start(), gc.IsNil)
		ps = append(ps, p)
	}

	c.Logf("Killing odd ones...")
	for i := 1; i < N; i += 2 {
		c.Assert(ps[i].KillForTesting(), gc.IsNil)
	}

	c.Logf("Checking who's still alive...")
	w := presence.NewWatcher(s.presence, s.modelTag)
	defer assertStopped(c, w)
	w.Sync()
	ch := make(chan presence.Change)
	for i := 0; i < N; i++ {
		k := strconv.Itoa(i)
		w.Watch(k, ch)
		if i%2 == 0 {
			assertChange(c, ch, presence.Change{k, true})
		} else {
			assertChange(c, ch, presence.Change{k, false})
		}
	}
}

func (s *PresenceSuite) TestExpiry(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	p := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	defer assertStopped(c, w)
	defer assertStopped(c, p)

	ch := make(chan presence.Change)
	w.Watch("a", ch)
	assertChange(c, ch, presence.Change{"a", false})

	c.Assert(p.Start(), gc.IsNil)
	w.StartSync()
	assertChange(c, ch, presence.Change{"a", true})

	// Still alive in previous slot.
	presence.FakeTimeSlot(1)
	w.StartSync()
	assertNoChange(c, ch)

	// Two last slots are empty.
	presence.FakeTimeSlot(2)
	w.StartSync()
	assertChange(c, ch, presence.Change{"a", false})

	// Already dead so killing isn't noticed.
	p.KillForTesting()
	w.StartSync()
	assertNoChange(c, ch)
}

func (s *PresenceSuite) TestWatchPeriod(c *gc.C) {
	presence.FakePeriod(1)
	presence.RealTimeSlot()

	w := presence.NewWatcher(s.presence, s.modelTag)
	p := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	defer assertStopped(c, w)
	defer assertStopped(c, p)

	ch := make(chan presence.Change)
	w.Watch("a", ch)
	assertChange(c, ch, presence.Change{"a", false})

	// A single ping.
	c.Assert(p.Start(), gc.IsNil)
	c.Assert(p.Stop(), gc.IsNil)

	// Wait for next periodic refresh.
	time.Sleep(1 * time.Second)
	assertChange(c, ch, presence.Change{"a", true})
}

func (s *PresenceSuite) TestWatchUnwatchOnQueue(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	defer assertStopped(c, w)
	ch := make(chan presence.Change, 100)
	for i := 0; i < 100; i++ {
		key := strconv.Itoa(i)
		c.Logf("Adding %q", key)
		w.Watch(key, ch)
	}
	for i := 1; i < 100; i += 2 {
		key := strconv.Itoa(i)
		c.Logf("Removing %q", key)
		w.Unwatch(key, ch)
	}
	alive := make(map[string]bool)
	for i := 0; i < 50; i++ {
		change := <-ch
		c.Logf("Got change for %q: %v", change.Key, change.Alive)
		alive[change.Key] = change.Alive
	}
	for i := 0; i < 100; i += 2 {
		key := strconv.Itoa(i)
		c.Logf("Checking %q...", key)
		c.Assert(alive[key], jc.IsFalse)
	}
}

func (s *PresenceSuite) TestRestartWithoutGaps(c *gc.C) {
	p := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	c.Assert(p.Start(), gc.IsNil)
	defer assertStopped(c, p)

	done := make(chan bool)
	go func() {
		stop := false
		for !stop {
			if !c.Check(p.Stop(), gc.IsNil) {
				break
			}
			if !c.Check(p.Start(), gc.IsNil) {
				break
			}
			select {
			case stop = <-done:
			default:
			}
		}
	}()
	go func() {
		stop := false
		for !stop {
			w := presence.NewWatcher(s.presence, s.modelTag)
			w.Sync()
			alive, err := w.Alive("a")
			assertStopped(c, w)
			if !c.Check(err, jc.ErrorIsNil) || !c.Check(alive, jc.IsTrue) {
				break
			}
			select {
			case stop = <-done:
			default:
			}
		}
	}()
	// TODO(jam): This forceful delay of 500ms sounds like a bad test,
	//  since we always sleep for the full timeout
	time.Sleep(500 * time.Millisecond)
	done <- true
	done <- true
}

func (s *PresenceSuite) TestPingerPeriodAndResilience(c *gc.C) {
	// This test verifies both the periodic pinging,
	// and also a great property of the design: deaths
	// also expire, which means erroneous scenarios are
	// automatically recovered from.

	const period = 1
	presence.FakePeriod(period)
	presence.RealTimeSlot()

	w := presence.NewWatcher(s.presence, s.modelTag)
	p1 := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	p2 := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	defer assertStopped(c, w)
	defer assertStopped(c, p1)
	defer assertStopped(c, p2)

	// Start p1 and let it go on.
	c.Assert(p1.Start(), gc.IsNil)

	w.Sync()
	assertAlive(c, w, "a", true)

	// Start and kill p2, which will temporarily
	// invalidate p1 and set the key as dead.
	c.Assert(p2.Start(), gc.IsNil)
	c.Assert(p2.KillForTesting(), gc.IsNil)

	w.Sync()
	assertAlive(c, w, "a", false)

	// Wait for two periods, and check again. Since
	// p1 is still alive, p2's death will expire and
	// the key will come back.
	time.Sleep(period * 2 * time.Second)

	w.Sync()
	assertAlive(c, w, "a", true)
}

func (s *PresenceSuite) TestStartSync(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	p := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	defer assertStopped(c, w)
	defer assertStopped(c, p)

	ch := make(chan presence.Change)
	w.Watch("a", ch)
	assertChange(c, ch, presence.Change{"a", false})

	c.Assert(p.Start(), gc.IsNil)

	done := make(chan bool)
	go func() {
		w.StartSync()
		w.StartSync()
		w.StartSync()
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("StartSync failed to return")
	}

	assertChange(c, ch, presence.Change{"a", true})
}

func (s *PresenceSuite) TestSync(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.modelTag)
	p := presence.NewPinger(s.presence, s.modelTag, "a", s.getDirectRecorder)
	defer assertStopped(c, w)
	defer assertStopped(c, p)

	ch := make(chan presence.Change)
	w.Watch("a", ch)
	assertChange(c, ch, presence.Change{"a", false})

	// Nothing to do here.
	w.Sync()

	c.Assert(p.Start(), gc.IsNil)

	done := make(chan bool)
	go func() {
		w.Sync()
		done <- true
	}()

	select {
	case <-done:
		c.Fatalf("Sync returned too early")
		// Note(jam): This used to wait 200ms to ensure that
		// Sync was actually blocked waiting for a presence
		// change. Is ShortWait long enough for this assurance?
	case <-time.After(testing.ShortWait):
	}

	assertChange(c, ch, presence.Change{"a", true})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("Sync failed to returned")
	}
}

func (s *PresenceSuite) TestTwoEnvironments(c *gc.C) {
	key := "a"
	w1, p1, ch1 := s.setup(c, key)
	defer assertStopped(c, w1)
	defer assertStopped(c, p1)

	w2, p2, ch2 := s.setup(c, key)
	defer assertStopped(c, w2)
	defer assertStopped(c, p2)

	c.Assert(p1.Start(), gc.IsNil)
	w1.StartSync()
	w2.StartSync()
	assertNoChange(c, ch2)
	assertChange(c, ch1, presence.Change{"a", true})

	c.Assert(p2.Start(), gc.IsNil)
	w1.StartSync()
	w2.StartSync()
	assertNoChange(c, ch1)
	assertChange(c, ch2, presence.Change{"a", true})

	err := p1.KillForTesting()
	c.Assert(err, jc.ErrorIsNil)
	presence.FakeTimeSlot(1)
	w1.StartSync()
	w2.StartSync()
	assertChange(c, ch1, presence.Change{"a", false})
	assertNoChange(c, ch2)

	err = p2.KillForTesting()
	c.Assert(err, jc.ErrorIsNil)
	presence.FakeTimeSlot(2)
	w1.StartSync()
	w2.StartSync()
	assertChange(c, ch2, presence.Change{"a", false})
	assertNoChange(c, ch1)
}

func newModelTag(c *gc.C) names.ModelTag {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	modelUUID := uuid.String()
	return names.NewModelTag(modelUUID)
}

func (s *PresenceSuite) setup(c *gc.C, key string) (*presence.Watcher, *presence.Pinger, <-chan presence.Change) {
	modelTag := newModelTag(c)

	w := presence.NewWatcher(s.presence, modelTag)
	p := presence.NewPinger(s.presence, modelTag, key, s.getDirectRecorder)

	ch := make(chan presence.Change)
	w.Watch(key, ch)
	assertChange(c, ch, presence.Change{key, false})
	return w, p, ch
}

func countModelIds(c *gc.C, coll *mgo.Collection, modelTag names.ModelTag) int {
	count, err := coll.Find(bson.M{"_id": bson.RegEx{"^" + modelTag.Id() + ":", ""}}).Count()
	// either the error is NotFound or nil
	if err != nil {
		c.Assert(err, gc.Equals, mgo.ErrNotFound)
	}
	return count
}

func (s *PresenceSuite) TestRemovePresenceForModel(c *gc.C) {
	key := "a"

	// Start a pinger in this model
	w1 := presence.NewWatcher(s.presence, s.modelTag)
	p1 := presence.NewPinger(s.presence, s.modelTag, key, s.getDirectRecorder)
	ch1 := make(chan presence.Change)
	w1.Watch(key, ch1)
	assertChange(c, ch1, presence.Change{key, false})
	defer assertStopped(c, w1)
	defer assertStopped(c, p1)
	p1.Start()
	w1.StartSync()
	assertChange(c, ch1, presence.Change{"a", true})

	// Start a second model and pinger with the same key
	modelTag2 := newModelTag(c)
	w2 := presence.NewWatcher(s.presence, modelTag2)
	p2 := presence.NewPinger(s.presence, modelTag2, key, s.getDirectRecorder)
	ch2 := make(chan presence.Change)
	w2.Watch(key, ch2)
	assertChange(c, ch2, presence.Change{key, false})
	defer assertStopped(c, w2)
	defer assertStopped(c, p2)
	// Start them, and check that we see they're alive
	p2.Start()
	w2.StartSync()
	assertChange(c, ch2, presence.Change{"a", true})

	beings := s.presence.Database.C(s.presence.Name + ".beings")
	pings := s.presence.Database.C(s.presence.Name + ".pings")
	seqs := s.presence.Database.C(s.presence.Name + ".seqs")
	// we should have a being and pings for both pingers
	c.Check(countModelIds(c, beings, s.modelTag), gc.Equals, 1)
	c.Check(countModelIds(c, beings, modelTag2), gc.Equals, 1)
	c.Check(countModelIds(c, pings, s.modelTag), jc.GreaterThan, 0)
	c.Check(countModelIds(c, pings, modelTag2), jc.GreaterThan, 0)
	c.Check(countModelIds(c, seqs, s.modelTag), gc.Equals, 1)
	c.Check(countModelIds(c, seqs, modelTag2), gc.Equals, 1)

	// kill everything in the first model
	assertStopped(c, w1)
	assertStopped(c, p1)
	// And cleanup the resources
	err := presence.RemovePresenceForModel(s.presence, s.modelTag)
	c.Assert(err, jc.ErrorIsNil)

	// Should not cause the second pinger to go dead
	w2.StartSync()
	assertNoChange(c, ch2)

	// And we should only have the second model in the databases
	c.Check(countModelIds(c, beings, s.modelTag), gc.Equals, 0)
	c.Check(countModelIds(c, beings, modelTag2), gc.Equals, 1)
	c.Check(countModelIds(c, pings, s.modelTag), gc.Equals, 0)
	c.Check(countModelIds(c, pings, modelTag2), jc.GreaterThan, 0)
	c.Check(countModelIds(c, seqs, s.modelTag), gc.Equals, 0)
	c.Check(countModelIds(c, seqs, modelTag2), gc.Equals, 1)

	// Removing a Model that is no longer there should not be an error
	err = presence.RemovePresenceForModel(s.presence, s.modelTag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PresenceSuite) TestMultiplePingersForEntity(c *gc.C) {
	// We should be able to track multiple sequences for a given Entity, without having to reread the database all the time
	key := "a"

	// Start a pinger in this model
	w := presence.NewWatcher(s.presence, s.modelTag)
	defer assertStopped(c, w)
	p1 := presence.NewPinger(s.presence, s.modelTag, key, s.getDirectRecorder)
	p1.Start()
	assertStopped(c, p1)
	p2 := presence.NewPinger(s.presence, s.modelTag, key, s.getDirectRecorder)
	p2.Start()
	assertStopped(c, p2)
	p3 := presence.NewPinger(s.presence, s.modelTag, key, s.getDirectRecorder)
	p3.Start()
	assertStopped(c, p3)
	w.Sync()
	loads := w.BeingLoads()
	c.Check(loads, jc.GreaterThan, uint64(0))
	alive, err := w.Alive(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(alive, jc.IsTrue)
	// When we sync a second time, all of the above entities should already be cached, so we don't have to load them again
	w.Sync()
	c.Check(w.BeingLoads(), gc.Equals, loads)
}

func (s *PresenceSuite) TestRobustness(c *gc.C) {
	// There used to be a potential condition, where during a flush() we wait for a channel send, and while we're
	// waiting for it, we would handle events, which might cause us to grow our pending array, which would realloc
	// the slice. If while that happened the original watch was unwatched, then we nil the channel, but the object
	// we were hung pending on part of the reallocated slice.
	w := presence.NewWatcher(s.presence, s.modelTag)
	defer assertStopped(c, w)
	// Start a watch for changes to 'key'. Never listen for actual events on that channel, though, so we know flush()
	// will always be blocked, but allowing other events while waiting to send that event.
	rootKey := "key"
	keyChan := make(chan presence.Change, 0)
	w.Watch(rootKey, keyChan)
	// Whenever we successfully watch in the main loop(), it starts a flush. We should now be able to build up more
	// watches while waiting. Create enough of these that we know the slice gets reallocated
	var wg sync.WaitGroup
	defer wg.Wait()
	var observed uint32
	const numKeys = 10
	for i := 0; i < numKeys; i++ {
		k := fmt.Sprintf("k%d", i)
		kChan := make(chan presence.Change, 0)
		w.Watch("key", kChan)
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-kChan:
				atomic.AddUint32(&observed, 1)
				return
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting %s for %q to see its event", testing.LongWait, k)
			}
		}()
	}
	// None of them should actually have triggered, since the very first pending object has not been listened to
	// And now we unwatch that object
	time.Sleep(testing.ShortWait)
	c.Check(atomic.LoadUint32(&observed), gc.Equals, uint32(0))
	w.Unwatch(rootKey, keyChan)
	// This should unblock all of them, and everything should go to observed
	failTime := time.After(testing.LongWait)
	o := atomic.LoadUint32(&observed)
	for o != numKeys {
		select {
		case <-time.After(time.Millisecond):
			o = atomic.LoadUint32(&observed)
		case <-failTime:
			c.Fatalf("only observed %d changes (expected %d) after %s time", atomic.LoadUint32(&observed), numKeys, testing.LongWait)
		}
	}
	c.Check(atomic.LoadUint32(&observed), gc.Equals, uint32(numKeys))
}
