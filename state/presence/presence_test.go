// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"strconv"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"launchpad.net/tomb"

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
	envTag   names.EnvironTag
}

var _ = gc.Suite(&PresenceSuite{})

func (s *PresenceSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.envTag = names.NewEnvironTag(uuid.String())
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

func assertAlive(c *gc.C, w *presence.Watcher, key string, alive bool) {
	alive, err := w.Alive("a")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, gc.Equals, alive)
}

func (s *PresenceSuite) TestErrAndDead(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.envTag)
	defer w.Stop()

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
	w := presence.NewWatcher(s.presence, s.envTag)
	c.Assert(w.Stop(), gc.IsNil)

	alive, err := w.Alive("a")
	c.Assert(err, gc.ErrorMatches, ".*: watcher is dying")
	c.Assert(alive, jc.IsFalse)
}

func (s *PresenceSuite) TestWorkflow(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.envTag)
	pa := presence.NewPinger(s.presence, s.envTag, "a")
	pb := presence.NewPinger(s.presence, s.envTag, "b")
	defer w.Stop()
	defer pa.Stop()
	defer pb.Stop()

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
	pa.Kill()
	w.Sync()
	pa = presence.NewPinger(s.presence, s.envTag, "a")
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
	c.Assert(pa.Kill(), gc.IsNil)
	c.Assert(pb.Kill(), gc.IsNil)

	w.StartSync()
	assertChange(c, cha, presence.Change{"a", false})
	assertChange(c, chb, presence.Change{"b", false})

	c.Assert(w.Stop(), gc.IsNil)
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
		p := presence.NewPinger(s.presence, s.envTag, strconv.Itoa(i))
		c.Assert(p.Start(), gc.IsNil)
		ps = append(ps, p)
	}

	c.Logf("Killing odd ones...")
	for i := 1; i < N; i += 2 {
		c.Assert(ps[i].Kill(), gc.IsNil)
	}

	c.Logf("Checking who's still alive...")
	w := presence.NewWatcher(s.presence, s.envTag)
	defer w.Stop()
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
	w := presence.NewWatcher(s.presence, s.envTag)
	p := presence.NewPinger(s.presence, s.envTag, "a")
	defer w.Stop()
	defer p.Stop()

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
	p.Kill()
	w.StartSync()
	assertNoChange(c, ch)
}

func (s *PresenceSuite) TestWatchPeriod(c *gc.C) {
	presence.FakePeriod(1)
	presence.RealTimeSlot()

	w := presence.NewWatcher(s.presence, s.envTag)
	p := presence.NewPinger(s.presence, s.envTag, "a")
	defer w.Stop()
	defer p.Stop()

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
	w := presence.NewWatcher(s.presence, s.envTag)
	ch := make(chan presence.Change)
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
	p := presence.NewPinger(s.presence, s.envTag, "a")
	c.Assert(p.Start(), gc.IsNil)
	defer p.Stop()

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
			w := presence.NewWatcher(s.presence, s.envTag)
			w.Sync()
			alive, err := w.Alive("a")
			c.Check(w.Stop(), gc.IsNil)
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

	w := presence.NewWatcher(s.presence, s.envTag)
	p1 := presence.NewPinger(s.presence, s.envTag, "a")
	p2 := presence.NewPinger(s.presence, s.envTag, "a")
	defer w.Stop()
	defer p1.Stop()
	defer p2.Stop()

	// Start p1 and let it go on.
	c.Assert(p1.Start(), gc.IsNil)

	w.Sync()
	assertAlive(c, w, "a", true)

	// Start and kill p2, which will temporarily
	// invalidate p1 and set the key as dead.
	c.Assert(p2.Start(), gc.IsNil)
	c.Assert(p2.Kill(), gc.IsNil)

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
	w := presence.NewWatcher(s.presence, s.envTag)
	p := presence.NewPinger(s.presence, s.envTag, "a")
	defer w.Stop()
	defer p.Stop()

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
	w := presence.NewWatcher(s.presence, s.envTag)
	p := presence.NewPinger(s.presence, s.envTag, "a")
	defer w.Stop()
	defer p.Stop()

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

func (s *PresenceSuite) TestFindAllBeings(c *gc.C) {
	w := presence.NewWatcher(s.presence, s.envTag)
	p := presence.NewPinger(s.presence, s.envTag, "a")
	defer w.Stop()
	defer p.Stop()

	ch := make(chan presence.Change)
	w.Watch("a", ch)
	assertChange(c, ch, presence.Change{"a", false})
	c.Assert(p.Start(), gc.IsNil)
	done := make(chan bool)
	go func() {
		w.Sync()
		done <- true
	}()
	assertChange(c, ch, presence.Change{"a", true})
	results, err := presence.FindAllBeings(w)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("Sync failed to returned")
	}
}

func (s *PresenceSuite) TestTwoEnvironments(c *gc.C) {
	key := "a"
	w1, p1, ch1 := s.setup(c, key)
	defer w1.Stop()
	defer p1.Stop()

	w2, p2, ch2 := s.setup(c, key)
	defer w2.Stop()
	defer p2.Stop()

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

	err := p1.Kill()
	c.Assert(err, jc.ErrorIsNil)
	presence.FakeTimeSlot(1)
	w1.StartSync()
	w2.StartSync()
	assertChange(c, ch1, presence.Change{"a", false})
	assertNoChange(c, ch2)

	err = p2.Kill()
	c.Assert(err, jc.ErrorIsNil)
	presence.FakeTimeSlot(2)
	w1.StartSync()
	w2.StartSync()
	assertChange(c, ch2, presence.Change{"a", false})
	assertNoChange(c, ch1)
}

func (s *PresenceSuite) setup(c *gc.C, key string) (*presence.Watcher, *presence.Pinger, <-chan presence.Change) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	envUUID := uuid.String()

	w := presence.NewWatcher(s.presence, names.NewEnvironTag(envUUID))
	p := presence.NewPinger(s.presence, names.NewEnvironTag(envUUID), key)

	ch := make(chan presence.Change)
	w.Watch(key, ch)
	assertChange(c, ch, presence.Change{key, false})
	return w, p, ch
}
