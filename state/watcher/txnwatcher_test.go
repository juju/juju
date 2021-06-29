// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/txn"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type TxnWatcherSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite

	log          *mgo.Collection
	stash        *mgo.Collection
	runner       *txn.Runner
	w            *watcher.TxnWatcher
	ch           chan watcher.Change
	iteratorFunc func(collection *mgo.Collection) mongo.Iterator
	clock        *testclock.Clock
}

var _ = gc.Suite(&TxnWatcherSuite{})

func (s *TxnWatcherSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *TxnWatcherSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *TxnWatcherSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("juju")
	s.log = db.C("txnlog")
	s.log.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: 1000000,
	})
	s.stash = db.C("txn.stash")
	s.runner = txn.NewRunner(db.C("txn"))
	s.runner.ChangeLog(s.log)
	s.clock = testclock.NewClock(time.Now())
	s.iteratorFunc = nil
}

func (s *TxnWatcherSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *TxnWatcherSuite) advanceTime(c *gc.C, d time.Duration, waiters int) {
	// Here we are assuming that there is one and only one thing
	// using the After function on the testing clock, that being our
	// watcher.
	c.Assert(s.clock.WaitAdvance(d, testing.ShortWait, waiters), jc.ErrorIsNil)
}

func (s *TxnWatcherSuite) newWatcher(c *gc.C, expect int) (*watcher.TxnWatcher, *fakeHub) {
	return s.newWatcherWithError(c, expect, nil)
}

func (s *TxnWatcherSuite) newWatcherWithError(c *gc.C, expect int, watcherError error) (*watcher.TxnWatcher, *fakeHub) {
	hub := newFakeHub(c, expect)
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	w, err := watcher.NewTxnWatcher(watcher.TxnWatcherConfig{
		Session:        s.MgoSuite.Session,
		JujuDBName:     "juju",
		CollectionName: s.log.Name,
		Hub:            hub,
		Clock:          s.clock,
		Logger:         logger,
		IteratorFunc:   s.iteratorFunc,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Wait for the main loop to have started.
	select {
	case <-hub.started:
	case <-time.After(testing.LongWait):
		c.Error("txn worker failed to start")
	}
	s.AddCleanup(func(c *gc.C) {
		if watcherError == nil {
			c.Assert(w.Stop(), jc.ErrorIsNil)
		} else {
			c.Assert(w.Stop(), gc.Equals, watcherError)
		}
	})
	return w, hub
}

func (s *TxnWatcherSuite) revno(c *gc.C, coll string, id interface{}) (revno int64) {
	var doc struct {
		Revno int64 `bson:"txn-revno"`
	}
	err := s.log.Database.C(coll).FindId(id).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	return doc.Revno
}

func (s *TxnWatcherSuite) insert(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Insert: M{"n": 1}}}
	err := s.runner.Run(ops, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	revno = s.revno(c, coll, id)
	c.Logf("insert(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *TxnWatcherSuite) insertAll(c *gc.C, coll string, ids ...interface{}) (revnos []int64) {
	var ops []txn.Op
	for _, id := range ids {
		ops = append(ops, txn.Op{C: coll, Id: id, Insert: M{"n": 1}})
	}
	err := s.runner.Run(ops, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	for _, id := range ids {
		revnos = append(revnos, s.revno(c, coll, id))
	}
	c.Logf("insertAll(%#v, %v) => revnos %v", coll, ids, revnos)
	return revnos
}

func (s *TxnWatcherSuite) update(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Update: M{"$inc": M{"n": 1}}}}
	err := s.runner.Run(ops, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	revno = s.revno(c, coll, id)
	c.Logf("update(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *TxnWatcherSuite) remove(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Remove: true}}
	err := s.runner.Run(ops, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("remove(%#v, %#v) => revno -1", coll, id)
	return -1
}

func (s *TxnWatcherSuite) TestErrAndDead(c *gc.C) {
	w, _ := s.newWatcher(c, 0)
	c.Assert(w.Err(), gc.Equals, tomb.ErrStillAlive)
	select {
	case <-w.Dead():
		c.Fatalf("Dead channel fired unexpectedly")
	default:
	}
	c.Assert(w.Stop(), jc.ErrorIsNil)
	c.Assert(w.Err(), jc.ErrorIsNil)
	select {
	case <-w.Dead():
	default:
		c.Fatalf("Dead channel should have fired")
	}
}

func (s *TxnWatcherSuite) TestInsert(c *gc.C) {
	_, hub := s.newWatcher(c, 1)

	revno := s.insert(c, "test", "a")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestUpdate(c *gc.C) {
	s.insert(c, "test", "a")

	_, hub := s.newWatcher(c, 1)
	revno := s.update(c, "test", "a")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestRemove(c *gc.C) {
	s.insert(c, "test", "a")

	_, hub := s.newWatcher(c, 1)
	revno := s.remove(c, "test", "a")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestWatchOrder(c *gc.C) {
	_, hub := s.newWatcher(c, 3)

	revno1 := s.insert(c, "test", "a")
	revno2 := s.insert(c, "test", "b")
	revno3 := s.insert(c, "test", "c")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno1},
		{"test", "b", revno2},
		{"test", "c", revno3},
	})
}

func (s *TxnWatcherSuite) TestTransactionWithMultiple(c *gc.C) {
	_, hub := s.newWatcher(c, 3)

	revnos := s.insertAll(c, "test", "a", "b", "c")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revnos[0]},
		{"test", "b", revnos[1]},
		{"test", "c", revnos[2]},
	})
}

func (s *TxnWatcherSuite) TestScale(c *gc.C) {
	const N = 500
	const T = 10

	_, hub := s.newWatcher(c, N)

	c.Logf("Creating %d documents, %d per transaction...", N, T)
	ops := make([]txn.Op, T)
	for i := 0; i < (N / T); i++ {
		ops = ops[:0]
		for j := 0; j < T && i*T+j < N; j++ {
			ops = append(ops, txn.Op{C: "test", Id: i*T + j, Insert: M{"n": 1}})
		}
		err := s.runner.Run(ops, "", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	count, err := s.Session.DB("juju").C("test").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("Got %d documents in the collection...", count)
	c.Assert(count, gc.Equals, N)

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForExpected(c)

	for i := 0; i < N; i++ {
		c.Assert(hub.values[i].Id, gc.Equals, i)
	}
}

func (s *TxnWatcherSuite) TestInsertThenRemove(c *gc.C) {
	_, hub := s.newWatcher(c, 2)

	revno1 := s.insert(c, "test", "a")
	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	revno2 := s.remove(c, "test", "a")
	s.advanceTime(c, watcher.TxnWatcherShortWait, 2)

	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno1},
		{"test", "a", revno2},
	})
}

func (s *TxnWatcherSuite) TestDoubleUpdate(c *gc.C) {
	_, hub := s.newWatcher(c, 2)

	revno1 := s.insert(c, "test", "a")
	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	s.update(c, "test", "a")
	revno3 := s.update(c, "test", "a")
	s.advanceTime(c, watcher.TxnWatcherShortWait, 2)

	hub.waitForExpected(c)

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno1},
		{"test", "a", revno3},
	})
}

func (s *TxnWatcherSuite) TestErrorRetry(c *gc.C) {
	syncCh := make(chan struct{}, 1)
	s.PatchValue(&watcher.TxnPollNotifyFunc, func() {
		syncCh <- struct{}{}
	})

	fakeIter := &fakeIterator{err: errors.New("boom")}
	s.iteratorFunc = func(collection *mgo.Collection) mongo.Iterator {
		fakeIter.iter = s.log.Find(nil).Batch(10).Sort("-$natural").Iter()
		return fakeIter
	}
	_, hub := s.newWatcher(c, 1)
	revno := s.insert(c, "test", "a")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	select {
	case <-syncCh:
	case <-time.After(testing.LongWait):
		c.Error("txn watcher didn't sync")
	}

	fakeIter.err = nil
	s.advanceTime(c, watcher.TxnWatcherErrorShortWait, 2)
	hub.waitForExpected(c)
	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestOutOfSyncError(c *gc.C) {
	fakeIter := &fakeIterator{err: watcher.OutOfSyncError}
	s.iteratorFunc = func(collection *mgo.Collection) mongo.Iterator {
		fakeIter.iter = s.log.Find(nil).Batch(10).Sort("-$natural").Iter()
		return fakeIter
	}
	_, hub := s.newWatcherWithError(c, 1, watcher.OutOfSyncError)
	s.insert(c, "test", "a")

	s.advanceTime(c, watcher.TxnWatcherShortWait, 1)
	hub.waitForError(c)
}

type fakeIterator struct {
	iter mongo.Iterator
	err  error
}

func (i *fakeIterator) Next(result interface{}) bool {
	return i.iter.Next(result)
}

func (i *fakeIterator) Timeout() bool {
	return i.iter.Timeout()
}

func (i *fakeIterator) Close() error {
	err := i.iter.Close()
	if i.err != nil {
		err = i.err
	}
	return err
}

type fakeHub struct {
	c       *gc.C
	expect  int
	values  []watcher.Change
	started chan struct{}
	done    chan struct{}
	error   chan struct{}
}

func newFakeHub(c *gc.C, expected int) *fakeHub {
	return &fakeHub{
		c:       c,
		expect:  expected,
		started: make(chan struct{}),
		done:    make(chan struct{}),
		error:   make(chan struct{}),
	}
}

func (hub *fakeHub) Publish(topic string, data interface{}) <-chan struct{} {
	switch topic {
	case watcher.TxnWatcherStarting:
		close(hub.started)
	case watcher.TxnWatcherCollection:
		change := data.(watcher.Change)
		hub.values = append(hub.values, change)
		if len(hub.values) == hub.expect {
			close(hub.done)
		}
	case watcher.TxnWatcherSyncErr:
		close(hub.error)
	default:
		hub.c.Errorf("unknown topic %q", topic)
	}
	return nil
}

func (hub *fakeHub) waitForExpected(c *gc.C) {
	select {
	case <-hub.done:
	case <-time.After(testing.LongWait):
		c.Error("hub didn't get the expected number of changes")
	}
}

func (hub *fakeHub) waitForError(c *gc.C) {
	select {
	case <-hub.error:
	case <-time.After(testing.LongWait):
		c.Error("hub didn't get an error")
	}
}
