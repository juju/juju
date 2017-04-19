// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"fmt"
	"math/rand"
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/testing"
)

type prunerSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	presence *mgo.Collection
	beings   *mgo.Collection
	pings    *mgo.Collection
	modelTag names.ModelTag
}

var _ = gc.Suite(&prunerSuite{})

func (s *prunerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.modelTag = names.NewModelTag(uuid.String())
}

func (s *prunerSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *prunerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("presence")
	s.presence = db.C("presence")
	s.beings = db.C("presence.beings")
	s.pings = db.C("presence.pings")

	FakeTimeSlot(0)
}

func (s *prunerSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)

	RealTimeSlot()
	RealPeriod()
}

func findBeing(c *gc.C, beingsC *mgo.Collection, modelUUID string, seq int64) (beingInfo, error) {
	var being beingInfo
	err := beingsC.FindId(docIDInt64(modelUUID, seq)).One(&being)
	return being, err
}

func checkCollectionCount(c *gc.C, coll *mgo.Collection, count int) {
	count, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, count)
}

func (s *prunerSuite) TestPrunesOldPingsAndBeings(c *gc.C) {
	keys := []string{"key1", "key2"}
	pingers := make([]*Pinger, len(keys))
	for i, key := range keys {
		pingers[i] = NewPinger(s.presence, s.modelTag, key)
	}
	const numSlots= 10
	sequences := make([][]int64, len(keys))
	for i := range keys {
		sequences[i] = make([]int64, numSlots)
	}

	for i := 0; i < numSlots; i++ {
		FakeTimeSlot(i)
		for j, p := range(pingers) {
			// Create a new being sequence, and force a ping in this
			// time slot. We don't Start()/Stop() them so we don't
			// have to worry about things being async.
			p.prepare()
			p.ping()
			sequences[j][i] = p.beingSeq
		}
	}
	// At this point, we should have 10 ping slots active, and 10 different
	// beings representing each key
	checkCollectionCount(c, s.beings, numSlots*len(keys))
	checkCollectionCount(c, s.pings, numSlots)
	// Now we prune them, and assert that it removed items, but preserved the
	// latest beings (things referenced by the latest pings)
	pruner := NewPruner(s.modelTag.Id(), s.beings, s.pings, 0)
	c.Assert(pruner.Prune(), jc.ErrorIsNil)
	checkCollectionCount(c, s.pings, 4)
	checkCollectionCount(c, s.beings, 2*len(keys))
	for i, key := range keys {
		expectedSeq := sequences[i][numSlots-2]
		being, err := findBeing(c, s.beings, s.modelTag.Id(), expectedSeq)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(being.Seq, gc.Equals, expectedSeq)
		c.Check(being.Key, gc.Equals, key)
	}
}

func (s *prunerSuite) TestPreservesLatestSequence(c *gc.C) {
	FakePeriod(1)

	key := "blah"
	p1 := NewPinger(s.presence, s.modelTag, key)
	p1.Start()
	assertStopped(c, p1)
	p2 := NewPinger(s.presence, s.modelTag, key)
	p2.Start()
	assertStopped(c, p2)
	// we're starting p2 second, so it should get a higher sequence
	c.Check(p1.beingSeq, gc.Not(gc.Equals), int64(0))
	c.Check(p1.beingSeq, jc.LessThan, p2.beingSeq)
	// Before pruning, we expect both beings to exist
	being, err := findBeing(c, s.beings, s.modelTag.Id(), p1.beingSeq)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(being.Key, gc.Equals, key)
	c.Check(being.Seq, gc.Equals, p1.beingSeq)
	being, err = findBeing(c, s.beings, s.modelTag.Id(), p2.beingSeq)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(being.Key, gc.Equals, key)
	c.Check(being.Seq, gc.Equals, p2.beingSeq)

	pruner := NewPruner(s.modelTag.Id(), s.beings, s.pings, 0)
	c.Assert(pruner.Prune(), jc.ErrorIsNil)
	// After pruning, p2 should still be available
	c.Check(being.Seq, gc.Equals, p2.beingSeq)
}

func waitForFirstChange(c *gc.C, watch <-chan Change, want Change) {
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

// assertStopped stops a worker and waits until it reports stopped.
// Use this method in favor of defer w.Stop() because you _must_ ensure
// that the worker has stopped, and thus is no longer using its mgo
// session before TearDownTest shuts down the connection.
func assertStopped(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}

func (s *prunerSuite) TestDeepStressStaysSane(c *gc.C) {
	const period = 1
	FakePeriod(period)
	RealTimeSlot()
	// Each Pinger potentially grabs another socket to Mongo,
	// which can exhaust connections. Somewhere around 5000 pingers on my
	// machine everything starts failing because Mongo refuses new connections.
	keys := make([]string, 3500)
	for i := 0; i < len(keys); i++ {
		keys[i] = fmt.Sprintf("being-%04d", i)
	}
	// To create abuse on the system, we leave 2 pingers active for every
	// key. We then keep creating new pingers for each one, and rotate them
	// into old, and stop them when they rotate out. So we potentially have
	// 3 active pingers for each key. We should never see any key go
	// inactive, because there is always at least 1 active pinger for each
	// one
	oldPingers := make([]*Pinger, len(keys))
	newPingers := make([]*Pinger, len(keys))
	ch := make(chan Change)
	w := NewWatcher(s.presence, s.modelTag)
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
	t := time.Now()
	sleepUSRange := int(1000000.*float64(period)/float64(len(keys)) - 500)
	if sleepUSRange <= 0 {
		sleepUSRange = 1
	}
	for i, key := range keys {
		w.Watch(key, ch)
		// we haven't started the pinger yet, so the initial state must be stopped
		// As this is a busy channel, we may be queued up behind some other
		// pinger showing up as alive, so allow up to LongWait for the event to show up
		waitForFirstChange(c, ch, Change{key, false})
		p := NewPinger(s.presence, s.modelTag, key)
		err := p.Start()
		c.Assert(err, jc.ErrorIsNil)
		newPingers[i] = p
		// All newPingers will be checked that they stop cleanly
		time.Sleep(time.Duration(rand.Intn(sleepUSRange)) * time.Microsecond)
	}
	fmt.Printf("initialized %d pingers in %v: sleeping %dus\n", len(newPingers), time.Since(t), sleepUSRange)
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
			case <-time.After(time.Duration(rand.Intn(500)+(period*1000)) * time.Millisecond):
				oldPruner := NewPruner(s.modelTag.Id(), beings, s.pings, 0)
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
			p := NewPinger(s.presence, s.modelTag, keys[i])
			err := p.Start()
			c.Assert(err, jc.ErrorIsNil)
			newPingers[i] = p
			time.Sleep(time.Duration(rand.Intn(sleepUSRange)) * time.Microsecond)
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
	seqDocID := s.modelTag.Id() + ":beings"
	err := seq.FindId(seqDocID).One(&sequence)
	c.Assert(err, jc.ErrorIsNil)
	// we should have created N keys Y+1 times (once in init, once per loop)
	seqCount := int64(len(keys) * (loopCount + 1))
	c.Check(sequence.Seq, gc.Equals, seqCount)
	oldPruner := NewPruner(s.modelTag.Id(), beings, s.pings, 0)
	c.Assert(oldPruner.Prune(), jc.ErrorIsNil)
	count, err := beings.Count()
	c.Assert(err, jc.ErrorIsNil)
	// After pruning, we should have at least one sequence for each key,
	// but not more than fits in the last pings
	c.Check(count, jc.GreaterThan, len(keys))
	c.Check(count, jc.LessThan, len(keys)*4)
	// Run the pruner again, it should essentially be a no-op
	oldPruner = NewPruner(s.modelTag.Id(), beings, s.pings, 0)
	c.Assert(oldPruner.Prune(), jc.ErrorIsNil)
	c.Fatal("dumping logs")
}
