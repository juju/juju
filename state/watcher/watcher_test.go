// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"launchpad.net/tomb"
	stdtesting "testing"
	"time"
)

// Test tuning parameters.
const (
	// worstCase is used for timeouts when timing out
	// will fail the test. Raising this value should
	// not affect the overall running time of the tests
	// unless they fail.
	// TODO(jam): Unify this with coretesting.LongWait
	worstCase = 10 * time.Second

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
	testing.LoggingSuite

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

func (s *FastPeriodSuite) SetUpSuite(c *C) {
	s.watcherSuite.SetUpSuite(c)
	watcher.Period = fastPeriod
}

var _ = Suite(&FastPeriodSuite{})

func (s *watcherSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.oldPeriod = watcher.Period
}

func (s *watcherSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
	watcher.Period = s.oldPeriod
}

func (s *watcherSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
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

func (s *watcherSuite) TearDownTest(c *C) {
	c.Assert(s.w.Stop(), IsNil)

	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

type M map[string]interface{}

func assertChange(c *C, watch <-chan watcher.Change, want watcher.Change) {
	select {
	case got := <-watch:
		if got != want {
			c.Fatalf("watch reported %v, want %v", got, want)
		}
	case <-time.After(worstCase):
		c.Fatalf("watch reported nothing, want %v", want)
	}
}

func assertNoChange(c *C, watch <-chan watcher.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(justLongEnough):
	}
}

func assertOrder(c *C, revnos ...int64) {
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

func (s *watcherSuite) insert(c *C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Insert: M{"n": 1}}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	revno = s.revno(coll, id)
	c.Logf("insert(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *watcherSuite) insertAll(c *C, coll string, ids ...interface{}) (revnos []int64) {
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

func (s *watcherSuite) update(c *C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Update: M{"$inc": M{"n": 1}}}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	revno = s.revno(coll, id)
	c.Logf("update(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *watcherSuite) remove(c *C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Remove: true}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	c.Logf("remove(%#v, %#v) => revno -1", coll, id)
	return -1
}

func (s *FastPeriodSuite) TestErrAndDead(c *C) {
	c.Assert(s.w.Err(), Equals, tomb.ErrStillAlive)
	select {
	case <-s.w.Dead():
		c.Fatalf("Dead channel fired unexpectedly")
	default:
	}
	c.Assert(s.w.Stop(), IsNil)
	c.Assert(s.w.Err(), IsNil)
	select {
	case <-s.w.Dead():
	default:
		c.Fatalf("Dead channel should have fired")
	}
}

func (s *FastPeriodSuite) TestWatchBeforeKnown(c *C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	revno := s.insert(c, "test", "a")

	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno)
}

func (s *FastPeriodSuite) TestWatchAfterKnown(c *C) {
	revno := s.insert(c, "test", "a")

	s.w.StartSync()

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno)
}

func (s *FastPeriodSuite) TestWatchIgnoreUnwatched(c *C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	s.insert(c, "test", "b")

	s.w.StartSync()
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestWatchOrder(c *C) {
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

func (s *FastPeriodSuite) TestTransactionWithMultiple(c *C) {
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

func (s *FastPeriodSuite) TestWatchMultipleChannels(c *C) {
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

func (s *FastPeriodSuite) TestIgnoreAncientHistory(c *C) {
	s.insert(c, "test", "a")

	w := watcher.New(s.log)
	defer w.Stop()
	w.StartSync()

	w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestUpdate(c *C) {
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

func (s *FastPeriodSuite) TestRemove(c *C) {
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

func (s *FastPeriodSuite) TestWatchKnownRemove(c *C) {
	revno1 := s.insert(c, "test", "a")
	revno2 := s.remove(c, "test", "a")
	s.w.Sync()

	s.w.Watch("test", "a", revno1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, revno2, revno1)
}

func (s *FastPeriodSuite) TestScale(c *C) {
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
		c.Assert(err, IsNil)
	}

	c.Logf("Watching all documents...")
	for i := 0; i < N; i++ {
		s.w.Watch("test", i, -1, s.ch)
	}

	c.Logf("Forcing a refresh...")
	s.w.StartSync()

	count, err := s.Session.DB("juju").C("test").Count()
	c.Assert(err, IsNil)
	c.Logf("Got %d documents in the collection...", count)
	c.Assert(count, Equals, N)

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
	c.Assert(len(seen), Equals, N)
}

func (s *FastPeriodSuite) TestWatchUnwatchOnQueue(c *C) {
	const N = 10
	for i := 0; i < N; i++ {
		s.insert(c, "test", i)
	}
	s.w.Sync()
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
	c.Assert(len(seen), Equals, N/2)
	assertNoChange(c, s.ch)
}

func (s *FastPeriodSuite) TestStartSync(c *C) {
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

func (s *FastPeriodSuite) TestSync(c *C) {
	s.w.Watch("test", "a", -1, s.ch)

	// Nothing to do here.
	s.w.Sync()

	revno := s.insert(c, "test", "a")

	done := make(chan bool)
	go func() {
		s.w.Sync()
		done <- true
	}()

	select {
	case <-done:
		c.Fatalf("Sync returned too early")
	case <-time.After(justLongEnough):
	}

	assertChange(c, s.ch, watcher.Change{"test", "a", revno})

	select {
	case <-done:
	case <-time.After(justLongEnough):
		c.Fatalf("Sync failed to return")
	}
}

func (s *FastPeriodSuite) TestWatchCollection(c *C) {
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
Loop1:
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
		case <-time.After(justLongEnough):
			break Loop1
		}
	}

	c.Assert(seen[chA1], DeepEquals, []watcher.Change{{"testA", 1, revno1}})
	c.Assert(seen[chB1], DeepEquals, []watcher.Change{{"testB", 1, revno3}})
	c.Assert(seen[chA], DeepEquals, []watcher.Change{{"testA", 1, revno1}, {"testA", 2, revno2}})
	c.Assert(seen[chB], DeepEquals, []watcher.Change{{"testB", 1, revno3}, {"testB", 2, revno4}})

	s.w.UnwatchCollection("testB", chB)
	s.w.Unwatch("testB", 1, chB1)

	revno1 = s.update(c, "testA", 1)
	revno3 = s.update(c, "testB", 1)

	s.w.StartSync()

	seen = map[chan<- watcher.Change][]watcher.Change{}
Loop2:
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
		case <-time.After(justLongEnough):
			break Loop2
		}
	}

	c.Assert(seen[chA1], DeepEquals, []watcher.Change{{"testA", 1, revno1}})
	c.Assert(seen[chB1], IsNil)
	c.Assert(seen[chA], DeepEquals, []watcher.Change{{"testA", 1, revno1}})
	c.Assert(seen[chB], IsNil)
}

func (s *FastPeriodSuite) TestUnwatchCollectionWithFilter(c *C) {
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

func (s *FastPeriodSuite) TestUnwatchCollectionWithOutstandingRequest(c *C) {
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

func (s *FastPeriodSuite) TestNonMutatingTxn(c *C) {
	chA1 := make(chan watcher.Change)
	chA := make(chan watcher.Change)

	revno1 := s.insert(c, "test", "a")

	s.w.Sync()

	s.w.Watch("test", 1, revno1, chA1)
	s.w.WatchCollection("test", chA)

	revno2 := s.insert(c, "test", "a")

	c.Assert(revno1, Equals, revno2)

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

func (s *SlowPeriodSuite) SetUpSuite(c *C) {
	s.watcherSuite.SetUpSuite(c)
	watcher.Period = slowPeriod
}

var _ = Suite(&SlowPeriodSuite{})

func (s *SlowPeriodSuite) TestWatchBeforeRemoveKnown(c *C) {
	revno1 := s.insert(c, "test", "a")
	s.w.Sync()
	revno2 := s.remove(c, "test", "a")

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno1})
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, revno2, revno1)
}

func (s *SlowPeriodSuite) TestDoubleUpdate(c *C) {
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

func (s *SlowPeriodSuite) TestWatchPeriod(c *C) {
	revno1 := s.insert(c, "test", "a")
	s.w.Sync()
	t0 := time.Now()
	s.w.Watch("test", "a", revno1, s.ch)
	revno2 := s.update(c, "test", "a")

	leeway := watcher.Period / 4
	select {
	case got := <-s.ch:
		gotPeriod := time.Since(t0)
		c.Assert(got, Equals, watcher.Change{"test", "a", revno2})
		if gotPeriod < watcher.Period-leeway {
			c.Fatalf("watcher not waiting long enough; got %v want %v", gotPeriod, watcher.Period)
		}
	case <-time.After(watcher.Period + leeway):
		gotPeriod := time.Since(t0)
		c.Fatalf("watcher waited too long; got %v want %v", gotPeriod, watcher.Period)
	}

	assertOrder(c, -1, revno1, revno2)
}
