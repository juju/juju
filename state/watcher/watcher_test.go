// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
)

// Test tuning parameters.
const (
	// worstCase is used for timeouts when timing out
	// will fail the test. Raising this value should
	// not affect the overall running time of the tests
	// unless they fail.
	worstCase = testing.LongWait

	// justLongEnough is used for timeouts that
	// are expected to happen for a test to complete
	// successfully. Reducing this value will make
	// the tests run faster at the expense of making them
	// fail more often on heavily loaded or slow hardware.
	justLongEnough = testing.ShortWait

	// fastPeriod specifies the period of the watcher for
	// tests where the timing is not critical.
	fastPeriod = 10 * time.Millisecond

	// slowPeriod specifies the period of the watcher
	// for tests where the timing is important.
	slowPeriod = 1 * time.Second
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type watcherSuite struct {
	testing.MgoSuite
	testing.BaseSuite

	log       *mgo.Collection
	stash     *mgo.Collection
	runner    *txn.Runner
	w         *watcher.Watcher
	ch        chan watcher.Change
	oldPeriod time.Duration
}

// FastPeriodSuite implements tests that should
// work regardless of the watcher refresh period.
type FastPeriodSuite struct {
	watcherSuite
}

func (s *FastPeriodSuite) SetUpSuite(c *gc.C) {
	s.watcherSuite.SetUpSuite(c)
	watcher.Period = fastPeriod
}

var _ = gc.Suite(&FastPeriodSuite{})

func (s *watcherSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.oldPeriod = watcher.Period
}

func (s *watcherSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
	watcher.Period = s.oldPeriod
}

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("juju")
	s.log = db.C("txnlog")
	s.log.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: 1000000,
	})
	s.stash = db.C("txn.stash")
	s.runner = txn.NewRunner(db.C("txn"))
	s.runner.ChangeLog(s.log)
	s.w = watcher.New(s.log)
	s.ch = make(chan watcher.Change)
}

func (s *watcherSuite) TearDownTest(c *gc.C) {
	c.Assert(s.w.Stop(), gc.IsNil)

	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

type M map[string]interface{}

func assertChange(c *gc.C, watch <-chan watcher.Change, want watcher.Change) {
	select {
	case got := <-watch:
		if got != want {
			c.Fatalf("watch reported %v, want %v", got, want)
		}
	case <-time.After(worstCase):
		c.Fatalf("watch reported nothing, want %v", want)
	}
}

func assertNoChange(c *gc.C, watch <-chan watcher.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(justLongEnough):
	}
}

func assertOrder(c *gc.C, revnos ...int64) {
	last := int64(-2)
	for _, revno := range revnos {
		if revno <= last {
			c.Fatalf("got bad revno sequence: %v", revnos)
		}
		last = revno
	}
}

func (s *watcherSuite) revno(c string, id interface{}) (revno int64) {
	var doc struct {
		Revno int64 "txn-revno"
	}
	err := s.log.Database.C(c).FindId(id).One(&doc)
	if err != nil {
		panic(err)
	}
	return doc.Revno
}

func (s *watcherSuite) insert(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Insert: M{"n": 1}}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	revno = s.revno(coll, id)
	c.Logf("insert(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *watcherSuite) insertAll(c *gc.C, coll string, ids ...interface{}) (revnos []int64) {
	var ops []txn.Op
	for _, id := range ids {
		ops = append(ops, txn.Op{C: coll, Id: id, Insert: M{"n": 1}})
	}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	for _, id := range ids {
		revnos = append(revnos, s.revno(coll, id))
	}
	c.Logf("insertAll(%#v, %v) => revnos %v", coll, ids, revnos)
	return revnos
}

func (s *watcherSuite) update(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Update: M{"$inc": M{"n": 1}}}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	revno = s.revno(coll, id)
	c.Logf("update(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *watcherSuite) remove(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Remove: true}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	c.Logf("remove(%#v, %#v) => revno -1", coll, id)
	return -1
}

func (s *FastPeriodSuite) TestErrAndDead(c *gc.C) {
	c.Assert(s.w.Err(), gc.Equals, tomb.ErrStillAlive)
	select {
	case <-s.w.Dead():
		c.Fatalf("Dead channel fired unexpectedly")
	default:
	}
	c.Assert(s.w.Stop(), gc.IsNil)
	c.Assert(s.w.Err(), gc.IsNil)
	select {
	case <-s.w.Dead():
	default:
		c.Fatalf("Dead channel should have fired")
	}
}

func (s *FastPeriodSuite) TestWatchBeforeKnown(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	revno := s.insert(c, "test", "a")

	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno)
}

func (s *FastPeriodSuite) TestWatchAfterKnown(c *gc.C) {
	revno := s.insert(c, "test", "a")

	s.w.StartSync()

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno)
}

func (s *FastPeriodSuite) TestWatchIgnoreUnwatched(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	s.insert(c, "test", "b")

	s.w.StartSync()
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestWatchOrder(c *gc.C) {
	s.w.StartSync()
	for _, id := range []string{"a", "b", "c", "d"} {
		s.w.Watch("test", id, -1, s.ch)
	}
	revno1 := s.insert(c, "test", "a")
	revno2 := s.insert(c, "test", "b")
	revno3 := s.insert(c, "test", "c")

	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno1})
	assertChange(c, s.ch, watcher.Change{"test", "b", revno2})
	assertChange(c, s.ch, watcher.Change{"test", "c", revno3})
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestTransactionWithMultiple(c *gc.C) {
	s.w.StartSync()
	for _, id := range []string{"a", "b", "c"} {
		s.w.Watch("test", id, -1, s.ch)
	}
	revnos := s.insertAll(c, "test", "a", "b", "c")
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revnos[0]})
	assertChange(c, s.ch, watcher.Change{"test", "b", revnos[1]})
	assertChange(c, s.ch, watcher.Change{"test", "c", revnos[2]})
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestWatchMultipleChannels(c *gc.C) {
	ch1 := make(chan watcher.Change)
	ch2 := make(chan watcher.Change)
	ch3 := make(chan watcher.Change)
	s.w.Watch("test1", 1, -1, ch1)
	s.w.Watch("test2", 2, -1, ch2)
	s.w.Watch("test3", 3, -1, ch3)
	revno1 := s.insert(c, "test1", 1)
	revno2 := s.insert(c, "test2", 2)
	revno3 := s.insert(c, "test3", 3)
	s.w.StartSync()
	s.w.Unwatch("test2", 2, ch2)
	assertChange(c, ch1, watcher.Change{"test1", 1, revno1})
	_ = revno2
	assertChange(c, ch3, watcher.Change{"test3", 3, revno3})
	assertNoChange(c, ch1)
	assertNoChange(c, ch2)
	assertNoChange(c, ch3)
}

func (s *FastPeriodSuite) TestIgnoreAncientHistory(c *gc.C) {
	s.insert(c, "test", "a")

	w := watcher.New(s.log)
	defer w.Stop()
	w.StartSync()

	w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestUpdate(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno1})
	assertNoChange(c, s.ch)

	revno2 := s.update(c, "test", "a")
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, -1, revno1, revno2)
}

func (s *FastPeriodSuite) TestRemove(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno1})
	assertNoChange(c, s.ch)

	revno2 := s.remove(c, "test", "a")
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", -1})
	assertNoChange(c, s.ch)

	revno3 := s.insert(c, "test", "a")
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno3})
	assertNoChange(c, s.ch)

	assertOrder(c, revno2, revno1)
	assertOrder(c, revno2, revno3)
}

func (s *FastPeriodSuite) TestWatchKnownRemove(c *gc.C) {
	revno1 := s.insert(c, "test", "a")
	revno2 := s.remove(c, "test", "a")
	s.w.StartSync()

	s.w.Watch("test", "a", revno1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, revno2, revno1)
}

func (s *FastPeriodSuite) TestScale(c *gc.C) {
	const N = 500
	const T = 10

	c.Logf("Creating %d documents, %d per transaction...", N, T)
	ops := make([]txn.Op, T)
	for i := 0; i < (N / T); i++ {
		ops = ops[:0]
		for j := 0; j < T && i*T+j < N; j++ {
			ops = append(ops, txn.Op{C: "test", Id: i*T + j, Insert: M{"n": 1}})
		}
		err := s.runner.Run(ops, "", nil)
		c.Assert(err, gc.IsNil)
	}

	c.Logf("Watching all documents...")
	for i := 0; i < N; i++ {
		s.w.Watch("test", i, -1, s.ch)
	}

	c.Logf("Forcing a refresh...")
	s.w.StartSync()

	count, err := s.Session.DB("juju").C("test").Count()
	c.Assert(err, gc.IsNil)
	c.Logf("Got %d documents in the collection...", count)
	c.Assert(count, gc.Equals, N)

	c.Logf("Reading all changes...")
	seen := make(map[interface{}]bool)
	for i := 0; i < N; i++ {
		select {
		case change := <-s.ch:
			seen[change.Id] = true
		case <-time.After(worstCase):
			c.Fatalf("not enough changes: got %d, want %d", len(seen), N)
		}
	}
	c.Assert(len(seen), gc.Equals, N)
}

func (s *FastPeriodSuite) TestWatchUnwatchOnQueue(c *gc.C) {
	const N = 10
	for i := 0; i < N; i++ {
		s.insert(c, "test", i)
	}
	s.w.StartSync()
	for i := 0; i < N; i++ {
		s.w.Watch("test", i, -1, s.ch)
	}
	for i := 1; i < N; i += 2 {
		s.w.Unwatch("test", i, s.ch)
	}
	s.w.StartSync()
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

func (s *FastPeriodSuite) TestStartSync(c *gc.C) {
	s.w.Watch("test", "a", -1, s.ch)

	revno := s.insert(c, "test", "a")

	done := make(chan bool)
	go func() {
		s.w.StartSync()
		s.w.StartSync()
		s.w.StartSync()
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(worstCase):
		c.Fatalf("StartSync failed to return")
	}

	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
}

func (s *FastPeriodSuite) TestWatchCollection(c *gc.C) {
	chA1 := make(chan watcher.Change)
	chB1 := make(chan watcher.Change)
	chA := make(chan watcher.Change)
	chB := make(chan watcher.Change)

	s.w.Watch("testA", 1, -1, chA1)
	s.w.Watch("testB", 1, -1, chB1)
	s.w.WatchCollection("testA", chA)
	s.w.WatchCollection("testB", chB)

	revno1 := s.insert(c, "testA", 1)
	revno2 := s.insert(c, "testA", 2)
	revno3 := s.insert(c, "testB", 1)
	revno4 := s.insert(c, "testB", 2)

	s.w.StartSync()

	seen := map[chan<- watcher.Change][]watcher.Change{}
	timeout := time.After(testing.LongWait)
	n := 0
Loop1:
	for n < 6 {
		select {
		case chg := <-chA1:
			seen[chA1] = append(seen[chA1], chg)
		case chg := <-chB1:
			seen[chB1] = append(seen[chB1], chg)
		case chg := <-chA:
			seen[chA] = append(seen[chA], chg)
		case chg := <-chB:
			seen[chB] = append(seen[chB], chg)
		case <-timeout:
			break Loop1
		}
		n++
	}

	c.Check(seen[chA1], gc.DeepEquals, []watcher.Change{{"testA", 1, revno1}})
	c.Check(seen[chB1], gc.DeepEquals, []watcher.Change{{"testB", 1, revno3}})
	c.Check(seen[chA], gc.DeepEquals, []watcher.Change{{"testA", 1, revno1}, {"testA", 2, revno2}})
	c.Check(seen[chB], gc.DeepEquals, []watcher.Change{{"testB", 1, revno3}, {"testB", 2, revno4}})
	if c.Failed() {
		return
	}

	s.w.UnwatchCollection("testB", chB)
	s.w.Unwatch("testB", 1, chB1)

	revno1 = s.update(c, "testA", 1)
	revno3 = s.update(c, "testB", 1)

	s.w.StartSync()

	timeout = time.After(testing.LongWait)
	seen = map[chan<- watcher.Change][]watcher.Change{}
	n = 0
Loop2:
	for n < 2 {
		select {
		case chg := <-chA1:
			seen[chA1] = append(seen[chA1], chg)
		case chg := <-chB1:
			seen[chB1] = append(seen[chB1], chg)
		case chg := <-chA:
			seen[chA] = append(seen[chA], chg)
		case chg := <-chB:
			seen[chB] = append(seen[chB], chg)
		case <-timeout:
			break Loop2
		}
		n++
	}
	c.Check(seen[chA1], gc.DeepEquals, []watcher.Change{{"testA", 1, revno1}})
	c.Check(seen[chB1], gc.IsNil)
	c.Check(seen[chA], gc.DeepEquals, []watcher.Change{{"testA", 1, revno1}})
	c.Check(seen[chB], gc.IsNil)

	// Check that no extra events arrive.
	seen = map[chan<- watcher.Change][]watcher.Change{}
	timeout = time.After(testing.ShortWait)
Loop3:
	for {
		select {
		case chg := <-chA1:
			seen[chA1] = append(seen[chA1], chg)
		case chg := <-chB1:
			seen[chB1] = append(seen[chB1], chg)
		case chg := <-chA:
			seen[chA] = append(seen[chA], chg)
		case chg := <-chB:
			seen[chB] = append(seen[chB], chg)
		case <-timeout:
			break Loop3
		}
	}
	c.Check(seen[chA1], gc.IsNil)
	c.Check(seen[chB1], gc.IsNil)
	c.Check(seen[chA], gc.IsNil)
	c.Check(seen[chB], gc.IsNil)
}

func (s *FastPeriodSuite) TestUnwatchCollectionWithFilter(c *gc.C) {
	filter := func(key interface{}) bool {
		id := key.(int)
		return id != 2
	}
	chA := make(chan watcher.Change)
	s.w.WatchCollectionWithFilter("testA", chA, filter)
	revnoA := s.insert(c, "testA", 1)
	assertChange(c, chA, watcher.Change{"testA", 1, revnoA})
	s.insert(c, "testA", 2)
	assertNoChange(c, chA)
	s.insert(c, "testA", 3)
	s.w.StartSync()
	assertChange(c, chA, watcher.Change{"testA", 3, revnoA})
}

func (s *FastPeriodSuite) TestUnwatchCollectionWithOutstandingRequest(c *gc.C) {
	chA := make(chan watcher.Change)
	s.w.WatchCollection("testA", chA)
	chB := make(chan watcher.Change)
	s.w.Watch("testB", 1, -1, chB)
	revnoA := s.insert(c, "testA", 1)
	s.insert(c, "testA", 2)
	// By inserting this *after* the testA document, we ensure that
	// the watcher will try to send on chB after sending on chA.
	// The original bug that this test guards against meant that the
	// UnwatchCollection did not correctly cancel the outstanding
	// request, so the loop would never get around to sending on
	// chB.
	revnoB := s.insert(c, "testB", 1)
	s.w.StartSync()
	// When we receive the first change on chA, we know that
	// the watcher is trying to send changes on all the
	// watcher channels (2 changes on chA and 1 change on chB).
	assertChange(c, chA, watcher.Change{"testA", 1, revnoA})
	s.w.UnwatchCollection("testA", chA)
	assertChange(c, chB, watcher.Change{"testB", 1, revnoB})
}

func (s *FastPeriodSuite) TestNonMutatingTxn(c *gc.C) {
	chA1 := make(chan watcher.Change)
	chA := make(chan watcher.Change)

	revno1 := s.insert(c, "test", "a")

	s.w.StartSync()

	s.w.Watch("test", 1, revno1, chA1)
	s.w.WatchCollection("test", chA)

	revno2 := s.insert(c, "test", "a")

	c.Assert(revno1, gc.Equals, revno2)

	s.w.StartSync()

	assertNoChange(c, chA1)
	assertNoChange(c, chA)
}

// SlowPeriodSuite implements tests
// that are flaky when the watcher refresh period
// is small.
type SlowPeriodSuite struct {
	watcherSuite
}

func (s *SlowPeriodSuite) SetUpSuite(c *gc.C) {
	s.watcherSuite.SetUpSuite(c)
	watcher.Period = slowPeriod
}

var _ = gc.Suite(&SlowPeriodSuite{})

func (s *SlowPeriodSuite) TestWatchBeforeRemoveKnown(c *gc.C) {
	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()
	revno2 := s.remove(c, "test", "a")

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno1})
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, revno2, revno1)
}

func (s *SlowPeriodSuite) TestDoubleUpdate(c *gc.C) {
	assertNoChange(c, s.ch)

	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()

	revno2 := s.update(c, "test", "a")
	revno3 := s.update(c, "test", "a")

	s.w.Watch("test", "a", revno2, s.ch)
	assertNoChange(c, s.ch)

	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno3})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno1, revno2, revno3)
}

func (s *SlowPeriodSuite) TestWatchPeriod(c *gc.C) {
	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()
	t0 := time.Now()
	s.w.Watch("test", "a", revno1, s.ch)
	revno2 := s.update(c, "test", "a")

	leeway := watcher.Period / 4
	select {
	case got := <-s.ch:
		gotPeriod := time.Since(t0)
		c.Assert(got, gc.Equals, watcher.Change{"test", "a", revno2})
		if gotPeriod < watcher.Period-leeway {
			c.Fatalf("watcher not waiting long enough; got %v want %v", gotPeriod, watcher.Period)
		}
	case <-time.After(watcher.Period + leeway):
		gotPeriod := time.Since(t0)
		c.Fatalf("watcher waited too long; got %v want %v", gotPeriod, watcher.Period)
	}

	assertOrder(c, -1, revno1, revno2)
}

func (s *SlowPeriodSuite) TestStartSyncStartsImmediately(c *gc.C) {
	// Ensure we're at the start of a sync cycle.
	s.w.StartSync()
	time.Sleep(justLongEnough)

	// Watching after StartSync should see the current state of affairs.
	revno := s.insert(c, "test", "a")
	s.w.StartSync()
	s.w.Watch("test", "a", -1, s.ch)
	select {
	case got := <-s.ch:
		c.Assert(got.Revno, gc.Equals, revno)
	case <-time.After(watcher.Period / 2):
		c.Fatalf("watch after StartSync is still using old info")
	}

	s.remove(c, "test", "a")
	s.w.StartSync()
	ch := make(chan watcher.Change)
	s.w.Watch("test", "a", -1, ch)
	select {
	case got := <-ch:
		c.Fatalf("got event %#v when starting watcher after doc was removed", got)
	case <-time.After(justLongEnough):
	}
}
