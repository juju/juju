package watcher_test

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/mstate/watcher"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type WatcherSuite struct {
	testing.MgoSuite
	testing.LoggingSuite

	log    *mgo.Collection
	stash  *mgo.Collection
	runner *txn.Runner
	w      *watcher.Watcher
	ch     chan watcher.Change
}

var _ = Suite(&WatcherSuite{})

func (s *WatcherSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *WatcherSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *WatcherSuite) SetUpTest(c *C) {
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

func (s *WatcherSuite) TearDownTest(c *C) {
	c.Assert(s.w.Stop(), IsNil)

	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)

	watcher.RealPeriod()
}

func assertChange(c *C, watch <-chan watcher.Change, want watcher.Change) {
	select {
	case got := <-watch:
		if got != want {
			c.Fatalf("watch reported %v, want %v", got, want)
		}
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("watch reported nothing, want %v", want)
	}
}

func assertNoChange(c *C, watch <-chan watcher.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(50 * time.Millisecond):
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

type M map[string]interface{}

func (s *WatcherSuite) revno(c string, id interface{}) (revno int64) {
	var doc struct {
		Revno int64 "txn-revno"
	}
	err := s.log.Database.C(c).FindId(id).One(&doc)
	if err != nil {
		panic(err)
	}
	return doc.Revno
}

func (s *WatcherSuite) insert(c *C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Insert: M{"n": 1}}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	revno = s.revno(coll, id)
	c.Logf("insert(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *WatcherSuite) insertAll(c *C, coll string, ids ...interface{}) (revnos []int64) {
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

func (s *WatcherSuite) update(c *C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Update: M{"$inc": M{"n": 1}}}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	revno = s.revno(coll, id)
	c.Logf("insert(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *WatcherSuite) remove(c *C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Remove: true}}
	err := s.runner.Run(ops, "", nil)
	if err != nil {
		panic(err)
	}
	c.Logf("remove(%#v, %#v) => revno -1", coll, id)
	return -1
}

func (s *WatcherSuite) TestWatchBeforeKnown(c *C) {
	s.w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)

	revno := s.insert(c, "test", "a")

	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno)
}

func (s *WatcherSuite) TestWatchAfterKnown(c *C) {
	revno := s.insert(c, "test", "a")

	s.w.StartSync()

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
	assertNoChange(c, s.ch)

	assertOrder(c, -1, revno)
}

func (s *WatcherSuite) TestWatchOrder(c *C) {
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

func (s *WatcherSuite) TestTransactionWithMultiple(c *C) {
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

func (s *WatcherSuite) TestWatchMultipleChannels(c *C) {
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

func (s *WatcherSuite) TestIgnoreAncientHistory(c *C) {
	s.insert(c, "test", "a")

	w := watcher.New(s.log)
	defer w.Stop()
	w.StartSync()

	w.Watch("test", "a", -1, s.ch)
	assertNoChange(c, s.ch)
}

func (s *WatcherSuite) TestUpdate(c *C) {
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

func (s *WatcherSuite) TestDoubleUpdate(c *C) {
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

func (s *WatcherSuite) TestRemove(c *C) {
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

func (s *WatcherSuite) TestWatchBeforeRemoveKnown(c *C) {
	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()
	revno2 := s.remove(c, "test", "a")

	s.w.Watch("test", "a", -1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno1})
	s.w.StartSync()
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, revno2, revno1)
}

func (s *WatcherSuite) TestWatchKnownRemove(c *C) {
	revno1 := s.insert(c, "test", "a")
	revno2 := s.remove(c, "test", "a")
	s.w.StartSync()

	s.w.Watch("test", "a", revno1, s.ch)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, revno2, revno1)
}

func (s *WatcherSuite) TestScale(c *C) {
	const N = 500
	const T = 10

	// Too much data.. doesn't help.
	debug := log.Debug
	defer func() { log.Debug = debug }()
	log.Debug = false

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
		case <-time.After(5 * time.Second):
			c.Fatalf("not enough changes: got %d, want %d", len(seen), N)
		}
	}
	c.Assert(len(seen), Equals, N)
}

func (s *WatcherSuite) TestWatchPeriod(c *C) {
	period := 1 * time.Second
	watcher.FakePeriod(period)
	revno1 := s.insert(c, "test", "a")
	s.w.StartSync()
	s.w.Watch("test", "a", revno1, s.ch)
	revno2 := s.update(c, "test", "a")

	// Wait for next periodic refresh.
	time.Sleep(period)
	assertChange(c, s.ch, watcher.Change{"test", "a", revno2})

	assertOrder(c, -1, revno1, revno2)
}

func (s *WatcherSuite) TestWatchUnwatchOnQueue(c *C) {
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
	for i := 0; i < N; i++ {
		s.update(c, "test", i)
	}
	seen := make(map[interface{}]bool)
	for i := 0; i < N/2; i++ {
		select {
		case change := <-s.ch:
			seen[change.Id] = true
		case <-time.After(5 * time.Second):
			c.Fatalf("not enough changes: got %d, want %d", len(seen), N/2)
		}
	}
	c.Assert(len(seen), Equals, N/2)
	assertNoChange(c, s.ch)
}

func (s *WatcherSuite) TestStartSync(c *C) {
	s.w.Watch("test", "a", -1, s.ch)

	// Nothing to do.
	s.w.StartSync()

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
	case <-time.After(100 * time.Millisecond):
		c.Fatalf("SyncStart failed to return")
	}

	assertChange(c, s.ch, watcher.Change{"test", "a", revno})
}

func (s *WatcherSuite) TestSync(c *C) {
	s.w.Watch("test", "a", -1, s.ch)

	// Nothing to do, so the syncronous request does nothing.
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
	case <-time.After(500 * time.Millisecond):
	}

	assertChange(c, s.ch, watcher.Change{"test", "a", revno})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		c.Fatalf("Sync failed to returned")
	}
}
