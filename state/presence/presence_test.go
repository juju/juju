// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"fmt"
	"math/rand"
	"strconv"
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

func waitForFirstChange(c *gc.C, watch <-chan presence.Change, want presence.Change) {
	timeout := time.After(testing.LongWait)
	for {
		select {
		case got := <-watch:
			if got == want {
				return
			}
			if got.Alive == false {
				c.Fatalf("got a not-alive before the one we were expecting: %v (want %v)", got, want)
			}
		case <-timeout:
			c.Fatalf("watch reported nothing, want %v", want)
		}
	}
}

func assertNoChange(c *gc.C, watch <-chan presence.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(testing.ShortWait):
	}
}

func assertAlive(c *gc.C, w *presence.Watcher, key string, alive bool) {
	alive, err := w.Alive("a")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, gc.Equals, alive)
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
	pa := presence.NewPinger(s.presence, s.modelTag, "a")
	pb := presence.NewPinger(s.presence, s.modelTag, "b")
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
	pa = presence.NewPinger(s.presence, s.modelTag, "a")
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
		p := presence.NewPinger(s.presence, s.modelTag, strconv.Itoa(i))
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
	p := presence.NewPinger(s.presence, s.modelTag, "a")
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
	p := presence.NewPinger(s.presence, s.modelTag, "a")
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
	p := presence.NewPinger(s.presence, s.modelTag, "a")
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
	p1 := presence.NewPinger(s.presence, s.modelTag, "a")
	p2 := presence.NewPinger(s.presence, s.modelTag, "a")
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
	p := presence.NewPinger(s.presence, s.modelTag, "a")
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
	p := presence.NewPinger(s.presence, s.modelTag, "a")
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
	p := presence.NewPinger(s.presence, modelTag, key)

	ch := make(chan presence.Change)
	w.Watch(key, ch)
	assertChange(c, ch, presence.Change{key, false})
	return w, p, ch
}

func (s *PresenceSuite) TestDeepStressStaysSane(c *gc.C) {
	presence.FakePeriod(1)
	defer presence.RealPeriod()
	presence.RealTimeSlot()
	// Each Pinger potentially grabs another socket to Mongo,
	// which can exhaust connections. Somewhere around 5000 pingers on my
	// machine everything starts failing because Mongo refuses new connections.
	keys := make([]string, 3500)
	for i := 0; i < len(keys); i++ {
		keys[i] = fmt.Sprintf("being-%04d", i)
	}
	modelTag := newModelTag(c)
	// To create abuse on the system, we leave 2 pingers active for every
	// key. We then keep creating new pingers for each one, and rotate them
	// into old, and stop them when they rotate out. So we potentially have
	// 3 active pingers for each key. We should never see any key go
	// inactive, because there is always at least 1 active pinger for each
	// one
	oldPingers := make([]*presence.Pinger, len(keys))
	newPingers := make([]*presence.Pinger, len(keys))
	ch := make(chan presence.Change)
	w := presence.NewWatcher(s.presence, modelTag)
	// Ensure that all pingers and the watcher are clean at exit
	defer assertStopped(c, w)
	defer func() {
		for i, p := range oldPingers {
			if p == nil {
				continue
			}
			assertStopped(c, p)
			oldPingers[i] = nil
		}
		for i, p := range newPingers {
			if p == nil {
				continue
			}
			assertStopped(c, p)
			newPingers[i] = nil
		}
	}()
	for i, key := range keys {
		w.Watch(key, ch)
		// we haven't started the pinger yet, so the initial state must be stopped
		// As this is a busy channel, we may be queued up behind some other
		// pinger showing up as alive, so allow up to LongWait for the event to show up
		waitForFirstChange(c, ch, presence.Change{key, false})
		p := presence.NewPinger(s.presence, modelTag, key)
		err := p.Start()
		c.Assert(err, jc.ErrorIsNil)
		newPingers[i] = p
		// All newPingers will be checked that they stop cleanly
	}
	fmt.Printf("initialized %d pingers\n", len(newPingers))
	// Make sure all of the entities stay showing up as alive
	done := make(chan struct{})
	go func() {
		for {
			select {
			case got := <-ch:
				c.Check(got.Alive, jc.IsTrue, gc.Commentf("key %q reported dead", got.Key))
			case <-done:
				return
			}
		}
	}()
	defer close(done)
	beings := s.presence.Database.C(s.presence.Name + ".beings")
	// Create a background Pruner task, that prunes items independently of
	// when they are being updated
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Duration(rand.Intn(500)+3000) * time.Millisecond):
				oldPruner := presence.NewBeingPruner(modelTag.Id(), beings, s.pings, 0)
				// Don't assert in a goroutine, as the panic may do bad things
				c.Check(oldPruner.Prune(), jc.ErrorIsNil)
			}
		}
	}()
	const loopCount = 10
	for loop := 0; loop < loopCount; loop++ {
		t := time.Now()
		for _, i := range rand.Perm(len(keys)) {
			old := oldPingers[i]
			if old != nil {
				assertStopped(c, old)
			}
			oldPingers[i] = newPingers[i]
			p := presence.NewPinger(s.presence, modelTag, keys[i])
			err := p.Start()
			c.Assert(err, jc.ErrorIsNil)
			newPingers[i] = p
		}
		// no need to force w.Sync() it automatically full syncs every period seconds
		fmt.Printf("loop %d in %v\n", loop, time.Since(t))
	}
	// Now that we've gone through all of that, check that we've created as
	// many sequences as we think we have
	seq := s.presence.Database.C(s.presence.Name + ".seqs")
	var sequence struct {
		Seq int64 `bson:"seq"`
	}
	seqDocID := modelTag.Id() + ":beings"
	err := seq.FindId(seqDocID).One(&sequence)
	c.Assert(err, jc.ErrorIsNil)
	// we should have created N keys Y+1 times (once in init, once per loop)
	seqCount := int64(len(keys) * (loopCount + 1))
	c.Check(sequence.Seq, gc.Equals, seqCount)
	oldPruner := presence.NewBeingPruner(modelTag.Id(), beings, s.pings, 0)
	c.Assert(oldPruner.Prune(), jc.ErrorIsNil)
	count, err := beings.Count()
	c.Assert(err, jc.ErrorIsNil)
	// After pruning, we should have exactly 1 sequence for each key
	c.Logf("beings has %d keys", count)
	c.Check(count, gc.Equals, len(keys))
	// Run the pruner again, it should essentially be a no-op
	oldPruner = presence.NewBeingPruner(modelTag.Id(), beings, s.pings, 0)
	c.Assert(oldPruner.Prune(), jc.ErrorIsNil)
	c.Fatal("dumping logs")
}
