// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state"
	mwWorker "github.com/juju/juju/worker/multiwatcher"
	"github.com/juju/juju/worker/multiwatcher/testbacking"
)

type watcherSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) startWorker(c *gc.C, backing state.AllWatcherBacking) *mwWorker.Worker {
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	config := mwWorker.Config{
		Logger:               logger,
		Backing:              backing,
		PrometheusRegisterer: noopRegisterer{},
	}
	w, err := mwWorker.NewWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, w)
	})
	return w
}

func (s *watcherSuite) TestEmptyModel(c *gc.C) {
	b := testbacking.New(nil)
	f := s.startWorker(c, b)
	w := f.WatchController()
	checkNext(c, w, nil, "")
}

func (s *watcherSuite) TestRun(c *gc.C) {
	b := testbacking.New([]multiwatcher.EntityInfo{
		&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "wordpress"},
	})
	mw := s.startWorker(c, b)
	w := mw.WatchController()

	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "wordpress"}},
	}, "")

	b.UpdateEntity(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0", InstanceID: "i-0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0", InstanceID: "i-0"}},
	}, "")

	b.DeleteEntity(multiwatcher.EntityID{"machine", "uuid", "0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Removed: true, Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"}},
	}, "")
}

func (s *watcherSuite) TestMultipleModels(c *gc.C) {
	b := testbacking.New([]multiwatcher.EntityInfo{
		&multiwatcher.MachineInfo{ModelUUID: "uuid0", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"},
		&multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "wordpress"},
		&multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"},
	})
	mw := s.startWorker(c, b)
	w := mw.WatchController()

	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid0", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"}},
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "wordpress"}},
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"}},
	}, "")

	b.UpdateEntity(&multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0", InstanceID: "i-0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0", InstanceID: "i-0"}},
	}, "")

	b.DeleteEntity(multiwatcher.EntityID{"machine", "uuid2", "0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Removed: true, Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"}},
	}, "")

	b.UpdateEntity(&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging", Exposed: true})
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging", Exposed: true}},
	}, "")
}

func (s *watcherSuite) TestModelFiltering(c *gc.C) {
	b := testbacking.New([]multiwatcher.EntityInfo{
		&multiwatcher.MachineInfo{ModelUUID: "uuid0", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"},
		&multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "wordpress"},
		&multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"},
	})
	mw := s.startWorker(c, b)
	w := watchWatcher(c, mw.WatchModel("uuid0"))
	c.Logf("w.assertNext")
	w.assertNext([]multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid0", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"}},
	})
	c.Logf("w.assertNext")

	// Updating uuid1 shouldn't signal a next call
	b.UpdateEntity(&multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0", InstanceID: "i-0"})
	b.DeleteEntity(multiwatcher.EntityID{"machine", "uuid2", "0"})
	c.Logf("w.assertNext")
	w.assertNoChange()

	c.Logf("w.assertNext")
	b.UpdateEntity(&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging", Exposed: true})
	w.assertNext([]multiwatcher.Delta{
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging", Exposed: true}},
	})
}

func (s *watcherSuite) TestWatcherStop(c *gc.C) {
	mw := s.startWorker(c, testbacking.New(nil))
	w := mw.WatchController()

	err := w.Stop()
	c.Assert(err, jc.ErrorIsNil)
	checkNext(c, w, nil, multiwatcher.NewErrStopped().Error())
}

func (s *watcherSuite) TestWatcherStopBecauseBackingError(c *gc.C) {
	b := testbacking.New([]multiwatcher.EntityInfo{&multiwatcher.MachineInfo{ID: "0"}})
	mw := s.startWorker(c, b)
	w := mw.WatchController()

	// Receive one delta to make sure that the storeManager
	// has seen the initial state.
	checkNext(c, w, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}}, "")
	c.Logf("setting fetch error")
	b.SetFetchError(errors.New("some error"))

	c.Logf("updating entity")
	b.UpdateEntity(&multiwatcher.MachineInfo{ID: "1"})
	checkNext(c, w, nil, "some error")
}

func (s *watcherSuite) TestWatcherErrorWhenWorkerStopped(c *gc.C) {
	b := testbacking.New([]multiwatcher.EntityInfo{&multiwatcher.MachineInfo{ID: "0"}})
	mw := s.startWorker(c, b)
	w := mw.WatchController()

	// Receive one delta to make sure that the storeManager
	// has seen the initial state.
	checkNext(c, w, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}}, "")

	workertest.CleanKill(c, mw)

	b.UpdateEntity(&multiwatcher.MachineInfo{ID: "1"})

	d, err := w.Next()
	c.Assert(err, gc.ErrorMatches, "shared state watcher was stopped")
	c.Assert(err, jc.Satisfies, multiwatcher.IsErrStopped)
	c.Assert(d, gc.HasLen, 0)
}

func getNext(c *gc.C, w multiwatcher.Watcher, timeout time.Duration) ([]multiwatcher.Delta, error) {
	var deltas []multiwatcher.Delta
	var err error
	ch := make(chan struct{}, 1)
	go func() {
		deltas, err = w.Next()
		ch <- struct{}{}
	}()
	select {
	case <-ch:
		return deltas, err
	case <-time.After(timeout):
	}
	return nil, errors.New("no change received in sufficient time")
}

func checkNext(c *gc.C, w multiwatcher.Watcher, deltas []multiwatcher.Delta, expectErr string) {
	d, err := getNext(c, w, 1*time.Second)
	if expectErr != "" {
		c.Check(err, gc.ErrorMatches, expectErr)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	checkDeltasEqual(c, d, deltas)
}

func checkDeltasEqual(c *gc.C, d0, d1 []multiwatcher.Delta) {
	// Deltas are returned in arbitrary order, so we compare them as maps.
	c.Check(deltaMap(d0), jc.DeepEquals, deltaMap(d1))
}

func deltaMap(deltas []multiwatcher.Delta) map[interface{}]multiwatcher.EntityInfo {
	m := make(map[interface{}]multiwatcher.EntityInfo)
	for _, d := range deltas {
		id := d.Entity.EntityID()
		if d.Removed {
			m[id] = nil
		} else {
			m[id] = d.Entity
		}
	}
	return m
}

// Need a way to test Next calls that block. This is needed to test filtering.

type watcher struct {
	c      *gc.C
	inner  multiwatcher.Watcher
	next   chan []multiwatcher.Delta
	err    chan error
	logger loggo.Logger
}

func watchWatcher(c *gc.C, w multiwatcher.Watcher) *watcher {
	result := &watcher{
		c:     c,
		inner: w,
		// We use a buffered channels here so the final next call that returns
		// an error when the worker stops can be pushed to the channel and allow
		// the goroutine to stop.
		next:   make(chan []multiwatcher.Delta, 1),
		err:    make(chan error, 1),
		logger: loggo.GetLogger("test"),
	}
	go result.loop()
	return result
}

func (w *watcher) loop() {
	for true {
		deltas, err := w.inner.Next()
		if err != nil {
			w.err <- err
			return
		}
		select {
		case w.next <- deltas:
			w.logger.Tracef("sent %d deltas down next", len(deltas))
		case <-time.After(testing.LongWait):
			w.c.Fatalf("no one listening")
		}
	}
}

func (w *watcher) assertNextErr(expectErr string) {
	select {
	case err := <-w.err:
		w.c.Assert(err, gc.ErrorMatches, expectErr)
	case next := <-w.next:
		w.c.Fatalf("unexpected results %#v", next)
	case <-time.After(testing.LongWait):
		w.c.Fatalf("no results returned")
	}
}
func (w *watcher) assertNext(deltas []multiwatcher.Delta) {
	select {
	case err := <-w.err:
		w.c.Fatalf("watcher had err: %v", err)
	case next := <-w.next:
		checkDeltasEqual(w.c, next, deltas)
	case <-time.After(testing.LongWait):
		w.c.Fatalf("no results returned")
	}
}

func (w *watcher) assertNoChange() {
	select {
	case <-time.After(testing.ShortWait):
		// all good
	case err := <-w.err:
		w.c.Fatalf("watcher had err: %v", err)
	case next := <-w.next:
		w.c.Fatalf("unexpected results %#v", next)
	}
}
