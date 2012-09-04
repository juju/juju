package presence_test

import (
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/mstate"
	"launchpad.net/juju-core/mstate/presence"
	"launchpad.net/juju-core/testing"
	"strconv"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var (
	period     = 50 * time.Millisecond
	longEnough = period * 6
)

type PresenceSuite struct {
	testing.MgoSuite
	testing.LoggingSuite
	presence *mgo.Collection
	pings    *mgo.Collection
	state    *state.State
}

var _ = Suite(&PresenceSuite{})

func (s *PresenceSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *PresenceSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *PresenceSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("presence")
	s.presence = db.C("presence")
	s.pings = db.C("presence.pings")

	var err error
	s.state, err = state.Dial(testing.MgoAddr)
	c.Assert(err, IsNil)

	presence.FakeTimeSlot(0)
}

func (s *PresenceSuite) TearDownTest(c *C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)

	presence.RealTimeSlot()
	presence.RealPeriod()
}

func assertChange(c *C, watch <-chan presence.Change, want presence.Change) {
	select {
	case got := <-watch:
		if got != want {
			c.Fatalf("watch reported %v, want %v", got, want)
		}
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("watch reported nothing, want %v", want)
	}
}

func assertNoChange(c *C, watch <-chan presence.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func (s *PresenceSuite) TestWorkflow(c *C) {
	w := presence.NewWatcher(s.presence)
	pa := presence.NewPinger(s.presence, "a")
	pb := presence.NewPinger(s.presence, "b")
	defer w.Stop()
	defer pa.Stop()
	defer pb.Stop()

	c.Assert(w.Alive("a"), Equals, false)
	c.Assert(w.Alive("b"), Equals, false)

	// Buffer one entry to avoid blocking the watcher here.
	cha := make(chan presence.Change, 1)
	chb := make(chan presence.Change, 1)
	w.Add("a", cha)
	w.Add("b", chb)

	// Initial events with current status.
	assertChange(c, cha, presence.Change{"a", false})
	assertChange(c, chb, presence.Change{"b", false})

	w.ForceRefresh()
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	c.Assert(pa.Start(), IsNil)

	w.ForceRefresh()
	assertChange(c, cha, presence.Change{"a", true})
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	c.Assert(w.Alive("a"), Equals, true)
	c.Assert(w.Alive("b"), Equals, false)

	// Changes while the channel is out are not observed.
	w.Remove("a", cha)
	assertNoChange(c, cha)
	pa.Kill()
	w.ForceRefresh()
	pa = presence.NewPinger(s.presence, "a")
	pa.Start()
	w.ForceRefresh()
	assertNoChange(c, cha)

	// We can still query it manually, though.
	c.Assert(w.Alive("a"), Equals, true)
	c.Assert(w.Alive("b"), Equals, false)

	// Initial positive event. No refresh needed.
	w.Add("a", cha)
	assertChange(c, cha, presence.Change{"a", true})

	c.Assert(pb.Start(), IsNil)

	w.ForceRefresh()
	assertChange(c, chb, presence.Change{"b", true})
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	c.Assert(pa.Stop(), IsNil)

	w.ForceRefresh()
	assertNoChange(c, cha)
	assertNoChange(c, chb)

	// pb is running, pa isn't.
	c.Assert(pa.Kill(), IsNil)
	c.Assert(pb.Kill(), IsNil)

	w.ForceRefresh()
	assertChange(c, cha, presence.Change{"a", false})
	assertChange(c, chb, presence.Change{"b", false})

	c.Assert(w.Stop(), IsNil)
}

func (s *PresenceSuite) TestScale(c *C) {
	const N = 1000
	var ps []*presence.Pinger
	defer func() {
		for _, p := range ps {
			p.Stop()
		}
	}()

	c.Logf("Starting %d pingers...", N)
	for i := 0; i < N; i++ {
		p := presence.NewPinger(s.presence, strconv.Itoa(i))
		c.Assert(p.Start(), IsNil)
		ps = append(ps, p)
	}

	c.Logf("Killing odd ones...")
	for i := 1; i < N; i += 2 {
		c.Assert(ps[i].Kill(), IsNil)
	}

	c.Logf("Checking who's still alive...")
	w := presence.NewWatcher(s.presence)
	defer w.Stop()
	w.ForceRefresh()
	ch := make(chan presence.Change)
	for i := 0; i < N; i++ {
		k := strconv.Itoa(i)
		w.Add(k, ch)
		if i%2 == 0 {
			assertChange(c, ch, presence.Change{k, true})
		} else {
			assertChange(c, ch, presence.Change{k, false})
		}
	}
}

func (s *PresenceSuite) TestExpiry(c *C) {
	w := presence.NewWatcher(s.presence)
	p := presence.NewPinger(s.presence, "a")
	defer w.Stop()
	defer p.Stop()

	ch := make(chan presence.Change)
	w.Add("a", ch)
	assertChange(c, ch, presence.Change{"a", false})

	c.Assert(p.Start(), IsNil)
	w.ForceRefresh()
	assertChange(c, ch, presence.Change{"a", true})

	// Still alive in previous slot.
	presence.FakeTimeSlot(1)
	w.ForceRefresh()
	assertNoChange(c, ch)

	// Two last slots are empty.
	presence.FakeTimeSlot(2)
	w.ForceRefresh()
	assertChange(c, ch, presence.Change{"a", false})

	// Already dead so killing isn't noticed.
	p.Kill()
	w.ForceRefresh()
	assertNoChange(c, ch)
}

func (s *PresenceSuite) TestWatchPeriod(c *C) {
	presence.FakePeriod(1)
	presence.RealTimeSlot()

	w := presence.NewWatcher(s.presence)
	p := presence.NewPinger(s.presence, "a")
	defer w.Stop()
	defer p.Stop()

	ch := make(chan presence.Change)
	w.Add("a", ch)
	assertChange(c, ch, presence.Change{"a", false})

	// A single ping.
	c.Assert(p.Start(), IsNil)
	c.Assert(p.Stop(), IsNil)

	// Wait for next periodic refresh.
	time.Sleep(1 * time.Second)
	assertChange(c, ch, presence.Change{"a", true})
}

func (s *PresenceSuite) TestAddRemoveOnQueue(c *C) {
	w := presence.NewWatcher(s.presence)
	ch := make(chan presence.Change)
	for i := 0; i < 100; i++ {
		key := strconv.Itoa(i)
		c.Logf("Adding %q", key)
		w.Add(key, ch)
	}
	for i := 1; i < 100; i += 2 {
		key := strconv.Itoa(i)
		c.Logf("Removing %q", key)
		w.Remove(key, ch)
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
		c.Assert(alive[key], Equals, false)
	}
}

func (s *PresenceSuite) TestRestartWithoutGaps(c *C) {
	p := presence.NewPinger(s.presence, "a")
	c.Assert(p.Start(), IsNil)
	defer p.Stop()

	done := make(chan bool)
	go func() {
		stop := false
		for !stop {
			if !c.Check(p.Stop(), IsNil) {
				break
			}
			if !c.Check(p.Start(), IsNil) {
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
			w := presence.NewWatcher(s.presence)
			w.ForceRefresh()
			alive := w.Alive("a")
			c.Check(w.Stop(), IsNil)
			if !c.Check(alive, Equals, true) {
				break
			}
			select {
			case stop = <-done:
			default:
			}
		}
	}()
	time.Sleep(500 * time.Millisecond)
	done <- true
	done <- true
}

func (s *PresenceSuite) TestPingerPeriodAndResilience(c *C) {
	// This test verifies both the periodic pinging,
	// and also a great property of the design: deaths
	// also expire, which means erroneous scenarios are
	// automatically recovered from.

	const period = 1
	presence.FakePeriod(period)
	presence.RealTimeSlot()

	w := presence.NewWatcher(s.presence)
	p1 := presence.NewPinger(s.presence, "a")
	p2 := presence.NewPinger(s.presence, "a")
	defer w.Stop()
	defer p1.Stop()
	defer p2.Stop()

	// Start p1 and let it go on.
	c.Assert(p1.Start(), IsNil)

	w.ForceRefresh()
	c.Assert(w.Alive("a"), Equals, true)

	// Start and kill p2, which will temporarily
	// invalidate p1 and set the key as dead.
	c.Assert(p2.Start(), IsNil)
	c.Assert(p2.Kill(), IsNil)

	w.ForceRefresh()
	c.Assert(w.Alive("a"), Equals, false)

	// Wait for two periods, and check again. Since
	// p1 is still alive, p2's death will expire and
	// the key will come back.
	time.Sleep(period * 2 * time.Second)

	w.ForceRefresh()
	c.Assert(w.Alive("a"), Equals, true)
}
