// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/mgo/v3/txn"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type TxnWatcherSuite struct {
	mgotesting.MgoSuite
	testing.BaseSuite

	log    *mgo.Collection
	stash  *mgo.Collection
	runner jujutxn.Runner
	w      *watcher.TxnWatcher
	ch     chan watcher.Change
	clock  clock.Clock
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
	s.log = db.C("testingsstxns.log")
	s.log.Create(&mgo.CollectionInfo{})
	s.stash = db.C("txn.stash")
	s.clock = testclock.NewDilatedWallClock(100 * time.Millisecond)
}

func (s *TxnWatcherSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *TxnWatcherSuite) newWatcher(c *gc.C, expect int) (*watcher.TxnWatcher, *fakeHub) {
	return s.newWatcherWithError(c, expect, nil, nil)
}

func (s *TxnWatcherSuite) newRunner(c *gc.C) {
	runnerSession := s.MgoSuite.Session.New()
	s.AddCleanup(func(c *gc.C) {
		s.runner = nil
		runnerSession.Close()
	})
	runnerSession.SetMode(mgo.Strong, true)
	s.runner = jujutxn.NewRunner(jujutxn.RunnerParams{
		Database:                  runnerSession.DB("juju"),
		TransactionCollectionName: "txn",
		ChangeLogName:             "-",
		ServerSideTransactions:    true,
		MaxRetryAttempts:          3,
	})
}

func (s *TxnWatcherSuite) newWatcherWithError(c *gc.C, expect int, watcherError error, runCmd func(*mgo.Database, any, any) error) (*watcher.TxnWatcher, *fakeHub) {
	hub := newFakeHub(c, expect)
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)

	session := s.MgoSuite.Session.New()
	w, err := watcher.NewTxnWatcher(watcher.TxnWatcherConfig{
		Session:      session,
		JujuDBName:   "juju",
		Hub:          hub,
		Clock:        s.clock,
		Logger:       logger,
		RunCmd:       runCmd,
		PollInterval: 50 * time.Millisecond,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		if watcherError == nil {
			c.Check(w.Stop(), jc.ErrorIsNil)
		} else {
			err := w.Stop()
			c.Check(errors.Is(err, watcherError), jc.IsTrue,
				gc.Commentf("%s should match error %s", err.Error(), watcherError.Error()))
		}
		session.Close()
	})
	err = w.Ready()
	c.Assert(err, jc.ErrorIsNil)
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
	err := s.runner.Run(func(attempt int) ([]txn.Op, error) {
		return ops, nil
	})
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
	err := s.runner.Run(func(attempt int) ([]txn.Op, error) {
		return ops, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	for _, id := range ids {
		revnos = append(revnos, s.revno(c, coll, id))
	}
	c.Logf("insertAll(%#v, %v) => revnos %v", coll, ids, revnos)
	return revnos
}

func (s *TxnWatcherSuite) update(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Update: M{"$inc": M{"n": 1}}}}
	err := s.runner.Run(func(attempt int) ([]txn.Op, error) {
		return ops, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	revno = s.revno(c, coll, id)
	c.Logf("update(%#v, %#v) => revno %d", coll, id, revno)
	return revno
}

func (s *TxnWatcherSuite) remove(c *gc.C, coll string, id interface{}) (revno int64) {
	ops := []txn.Op{{C: coll, Id: id, Remove: true}}
	err := s.runner.Run(func(attempt int) ([]txn.Op, error) {
		return ops, nil
	})
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
	s.newRunner(c)
	_, hub := s.newWatcher(c, 1)

	revno := s.insert(c, "test", "a")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestUpdate(c *gc.C) {
	s.newRunner(c)
	s.insert(c, "test", "a")

	_, hub := s.newWatcher(c, 1)
	revno := s.update(c, "test", "a")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestRemove(c *gc.C) {
	s.newRunner(c)
	s.insert(c, "test", "a")

	_, hub := s.newWatcher(c, 1)
	revno := s.remove(c, "test", "a")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno},
	})
}

func (s *TxnWatcherSuite) TestWatchOrder(c *gc.C) {
	s.newRunner(c)
	_, hub := s.newWatcher(c, 3)

	revno1 := s.insert(c, "test", "a")
	revno2 := s.insert(c, "test", "b")
	revno3 := s.insert(c, "test", "c")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno1},
		{"test", "b", revno2},
		{"test", "c", revno3},
	})
}

func (s *TxnWatcherSuite) TestMissingOplogCollection(c *gc.C) {
	db := s.MgoSuite.Session.DB("juju")
	s.log = db.C("missingsstxns.log")

	s.newRunner(c)
	_, hub := s.newWatcher(c, 3)

	revno1 := s.insert(c, "test", "a")
	revno2 := s.insert(c, "test", "b")
	revno3 := s.insert(c, "test", "c")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno1},
		{"test", "b", revno2},
		{"test", "c", revno3},
	})
}

func (s *TxnWatcherSuite) TestTransactionWithMultiple(c *gc.C) {
	s.newRunner(c)
	_, hub := s.newWatcher(c, 3)

	revnos := s.insertAll(c, "test", "a", "b", "c")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revnos[0]},
		{"test", "b", revnos[1]},
		{"test", "c", revnos[2]},
	})
}

func (s *TxnWatcherSuite) TestScale(c *gc.C) {
	const N = 500
	const T = 10

	s.newRunner(c)
	_, hub := s.newWatcher(c, N)

	c.Logf("Creating %d documents, %d per transaction...", N, T)
	ops := make([]txn.Op, T)
	for i := 0; i < (N / T); i++ {
		ops = ops[:0]
		for j := 0; j < T && i*T+j < N; j++ {
			ops = append(ops, txn.Op{C: "test", Id: i*T + j, Insert: M{"n": 1}})
		}
		err := s.runner.Run(func(attempt int) ([]txn.Op, error) {
			return ops, nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	count, err := s.Session.DB("juju").C("test").Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("Got %d documents in the collection...", count)
	c.Assert(count, gc.Equals, N)

	hub.waitForExpected()

	for i := 0; i < N; i++ {
		c.Assert(hub.values[i].Id, gc.Equals, i)
	}
}

func (s *TxnWatcherSuite) TestInsertThenRemove(c *gc.C) {
	s.newRunner(c)
	_, hub := s.newWatcher(c, 2)

	revno1 := s.insert(c, "test", "a")
	revno2 := s.remove(c, "test", "a")

	hub.waitForExpected()

	c.Assert(hub.values, jc.DeepEquals, []watcher.Change{
		{"test", "a", revno1},
		{"test", "a", revno2},
	})
}

func (s *TxnWatcherSuite) TestShouldRetryGetMore(c *gc.C) {
	s.newRunner(c)
	getMoreErrors := make(chan error, 10)
	for i := 0; i < 10; i++ {
		getMoreErrors <- &mgo.QueryError{Code: 1, Message: "resumeable for sure"}
	}
	numResumeOrStart := int32(0)
	run := func(db *mgo.Database, cmd, resp any) error {
		b, ok := cmd.(bson.D)
		c.Assert(ok, jc.IsTrue)
		switch b[0].Name {
		case "aggregate":
			atomic.AddInt32(&numResumeOrStart, 1)
		case "getMore":
			select {
			case err := <-getMoreErrors:
				return err
			default:
			}
		}
		return db.Run(cmd, resp)
	}
	_, hub := s.newWatcherWithError(c, 1, nil, run)
	s.insert(c, "test", "a")
	hub.waitForExpected()
	c.Assert(atomic.LoadInt32(&numResumeOrStart), gc.Equals, int32(2))
}

func (s *TxnWatcherSuite) TestShouldResume(c *gc.C) {
	s.newRunner(c)
	getMoreErrors := make(chan error, 1)
	for i := 0; i < 1; i++ {
		getMoreErrors <- &mgo.QueryError{Code: 43, Message: "cursor died maybe resume"}
	}
	numResumeOrStart := int32(0)
	run := func(db *mgo.Database, cmd, resp any) error {
		b, ok := cmd.(bson.D)
		c.Assert(ok, jc.IsTrue)
		switch b[0].Name {
		case "aggregate":
			atomic.AddInt32(&numResumeOrStart, 1)
		case "getMore":
			select {
			case err := <-getMoreErrors:
				return err
			default:
			}
		}
		return db.Run(cmd, resp)
	}
	_, hub := s.newWatcherWithError(c, 1, nil, run)
	s.insert(c, "test", "a")
	hub.waitForExpected()
	c.Assert(atomic.LoadInt32(&numResumeOrStart), gc.Equals, int32(2))
}

func (s *TxnWatcherSuite) TestNotResumable(c *gc.C) {
	s.newRunner(c)
	numResumeOrStart := int32(0)
	run := func(db *mgo.Database, cmd, resp any) error {
		b, ok := cmd.(bson.D)
		c.Assert(ok, jc.IsTrue)
		switch b[0].Name {
		case "aggregate":
			atomic.AddInt32(&numResumeOrStart, 1)
		case "getMore":
			return &mgo.QueryError{Code: 234, Message: "bad"}
		}
		return db.Run(cmd, resp)
	}
	_, hub := s.newWatcherWithError(c, 1, watcher.FatalChangeStreamError, run)
	s.insert(c, "test", "a")
	hub.waitForError()
	c.Assert(atomic.LoadInt32(&numResumeOrStart), gc.Equals, int32(1))
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
	c      *gc.C
	expect int
	values []watcher.Change
	done   chan struct{}
	error  chan struct{}

	syncMu sync.RWMutex
	sync   chan struct{}
}

func newFakeHub(c *gc.C, expected int) *fakeHub {
	return &fakeHub{
		c:      c,
		expect: expected,
		done:   make(chan struct{}),
		error:  make(chan struct{}),
	}
}

func (hub *fakeHub) Publish(topic string, data interface{}) func() {
	switch topic {
	case watcher.TxnWatcherCollection:
		change := data.(watcher.Change)
		hub.values = append(hub.values, change)
		hub.doSync()

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

// setupSync should be called prior to clock advancement if you need to
// synchronise on a subsequent change.
// This can be used to prevent a scenario where steps like:
// - change
// - clock advance
// - change
// race with the worker loop causing both change events to be processed
// in a single pass.
// Failing to call waitSync at some point after setupSync will block the
// hub from processing publish events.
func (hub *fakeHub) setupSync() {
	hub.syncMu.Lock()
	defer hub.syncMu.Unlock()

	if hub.sync != nil {
		hub.c.Errorf("sync is already set up; did you fail to call waitSync?")
	}
	hub.sync = make(chan struct{})
}

// This is executed on a different Goroutine to setupSync and waitSync;
// hence the read lock protection.
func (hub *fakeHub) doSync() {
	hub.syncMu.RLock()
	defer hub.syncMu.RUnlock()

	if hub.sync != nil {
		hub.sync <- struct{}{}
	}
}

// waitSync unblocks after a publish event.
// if setupSync was not called prior, an error results.
func (hub *fakeHub) waitSync() {
	hub.syncMu.RLock()
	if hub.sync == nil {
		hub.syncMu.RUnlock()
		hub.c.Errorf("waitSync called without preceding setupSync")
		return
	}

	select {
	case <-hub.sync:
	case <-time.After(testing.LongWait):
		hub.c.Error("hub did not receive a publish event")
	}

	hub.syncMu.RUnlock()
	hub.syncMu.Lock()
	hub.sync = nil
	hub.syncMu.Unlock()
}

func (hub *fakeHub) waitForExpected() {
	select {
	case <-hub.done:
	case <-time.After(testing.LongWait):
		hub.c.Error("hub didn't get the expected number of changes")
	}
}

func (hub *fakeHub) waitForError() {
	select {
	case <-hub.error:
	case <-time.After(testing.LongWait):
		hub.c.Error("hub didn't get an error")
	}
}
