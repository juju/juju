// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinelock_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/paths"
	jujutesting "github.com/juju/juju/testing"
)

type Lock interface {
	Acquire(machinelock.Spec) (func(), error)
	Report(...machinelock.ReportOption) (string, error)
}

type lockSuite struct {
	testing.IsolationSuite
	logfile string
	clock   *fakeClock
	lock    Lock

	notify       chan struct{}
	allowAcquire chan struct{}
	release      chan struct{}
}

var _ = gc.Suite(&lockSuite{})

func (s *lockSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = &fakeClock{time.Date(2018, 7, 10, 12, 0, 0, 0, time.UTC)}

	s.logfile = filepath.Join(c.MkDir(), "logfile")

	s.notify = make(chan struct{})
	s.allowAcquire = make(chan struct{})
	s.release = make(chan struct{})

	lock, err := machinelock.NewTestLock(machinelock.Config{
		AgentName:   "test",
		Clock:       s.clock,
		Logger:      loggo.GetLogger("test"),
		LogFilename: s.logfile,
	}, s.acquireLock)
	c.Assert(err, jc.ErrorIsNil)
	s.lock = lock

	s.AddCleanup(func(c *gc.C) {
		// release all the pending goroutines
		close(s.allowAcquire)
	})
}

func (s *lockSuite) TestLogFilePermissions(c *gc.C) {
	info, err := os.Stat(s.logfile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Mode(), gc.Equals, paths.LogfilePermission)
}

func (s *lockSuite) TestEmptyOutput(c *gc.C) {
	output, err := s.lock.Report()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: none
`[1:])

	output, err = s.lock.Report(machinelock.ShowDetailsYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: null
`[1:])
}

func (s *lockSuite) TestWaitingOutput(c *gc.C) {
	s.addWaiting(c, "worker1", "being busy")
	s.clock.Advance(time.Minute)
	s.addWaiting(c, "worker", "")
	s.clock.Advance(time.Minute)

	output, err := s.lock.Report()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: none
  waiting:
  - worker1 (being busy), waiting 2m0s
  - worker, waiting 1m0s
`[1:])

	output, err = s.lock.Report(machinelock.ShowDetailsYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: null
  waiting:
  - worker: worker1
    comment: being busy
    requested: 2018-07-10 12:00:00 +0000 UTC
    wait-time: 2m0s
  - worker: worker
    requested: 2018-07-10 12:01:00 +0000 UTC
    wait-time: 1m0s
`[1:])
}

func (s *lockSuite) TestHoldingOutput(c *gc.C) {
	s.addAcquired(c, "worker1", "being busy", 0)
	s.clock.Advance(time.Minute * 2)

	output, err := s.lock.Report()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: worker1 (being busy), holding 2m0s
`[1:])

	output, err = s.lock.Report(machinelock.ShowDetailsYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder:
    worker: worker1
    comment: being busy
    requested: 2018-07-10 12:00:00 +0000 UTC
    acquired: 2018-07-10 12:00:00 +0000 UTC
    hold-time: 2m0s
`[1:])

}

func (s *lockSuite) TestHistoryOutput(c *gc.C) {
	short := 5 * time.Second
	long := 2*time.Minute + short
	s.addHistory(c, "uniter", "config-changed", "2018-07-21 15:36:01", time.Second, long)
	s.addHistory(c, "uniter", "update-status", "2018-07-21 15:37:05", time.Second, short)
	s.addHistory(c, "uniter", "update-status", "2018-07-21 15:42:11", time.Second, short)
	s.addHistory(c, "uniter", "update-status", "2018-07-21 15:47:13", time.Second, short)

	output, err := s.lock.Report()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: none
`[1:])

	output, err = s.lock.Report(machinelock.ShowHistory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: none
  history:
  - 2018-07-21 15:47:13 uniter (update-status), waited 1s, held 5s
  - 2018-07-21 15:42:11 uniter (update-status), waited 1s, held 5s
  - 2018-07-21 15:37:05 uniter (update-status), waited 1s, held 5s
  - 2018-07-21 15:36:01 uniter (config-changed), waited 1s, held 2m5s
`[1:])

	output, err = s.lock.Report(machinelock.ShowHistory, machinelock.ShowDetailsYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, `
test:
  holder: null
  history:
  - worker: uniter
    comment: update-status
    requested: 2018-07-21 15:47:07 +0000 UTC
    acquired: 2018-07-21 15:47:08 +0000 UTC
    released: 2018-07-21 15:47:13 +0000 UTC
    wait-time: 1s
    hold-time: 5s
  - worker: uniter
    comment: update-status
    requested: 2018-07-21 15:42:05 +0000 UTC
    acquired: 2018-07-21 15:42:06 +0000 UTC
    released: 2018-07-21 15:42:11 +0000 UTC
    wait-time: 1s
    hold-time: 5s
  - worker: uniter
    comment: update-status
    requested: 2018-07-21 15:36:59 +0000 UTC
    acquired: 2018-07-21 15:37:00 +0000 UTC
    released: 2018-07-21 15:37:05 +0000 UTC
    wait-time: 1s
    hold-time: 5s
  - worker: uniter
    comment: config-changed
    requested: 2018-07-21 15:33:55 +0000 UTC
    acquired: 2018-07-21 15:33:56 +0000 UTC
    released: 2018-07-21 15:36:01 +0000 UTC
    wait-time: 1s
    hold-time: 2m5s
`[1:])
}

func (s *lockSuite) TestLogfileOutput(c *gc.C) {
	short := 5 * time.Second
	long := 2*time.Minute + short
	s.addHistory(c, "uniter", "config-changed", "2018-07-21 15:36:01", time.Second, long)
	s.addHistory(c, "uniter", "update-status", "2018-07-21 15:37:05", time.Second, short)
	s.addHistory(c, "uniter", "update-status", "2018-07-21 15:42:11", time.Second, short)
	s.addHistory(c, "uniter", "update-status", "2018-07-21 15:47:13", time.Second, short)

	content, err := ioutil.ReadFile(s.logfile)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(content), gc.Equals, `
2018-07-10 12:00:00 === agent test started ===
2018-07-21 15:36:01 test: uniter (config-changed), waited 1s, held 2m5s
2018-07-21 15:37:05 test: uniter (update-status), waited 1s, held 5s
2018-07-21 15:42:11 test: uniter (update-status), waited 1s, held 5s
2018-07-21 15:47:13 test: uniter (update-status), waited 1s, held 5s
`[1:])
}

func (s *lockSuite) addWaiting(c *gc.C, worker, comment string) {
	go func() {
		_, err := s.lock.Acquire(machinelock.Spec{
			Cancel:  make(chan struct{}),
			Worker:  worker,
			Comment: comment,
		})
		c.Check(err, jc.ErrorIsNil)
	}()

	select {
	case <-s.notify:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("lock acquire didn't happen")
	}
}

func (s *lockSuite) addAcquired(c *gc.C, worker, comment string, wait time.Duration) func() {
	releaser := make(chan func())
	go func() {
		r, err := s.lock.Acquire(machinelock.Spec{
			Cancel:  make(chan struct{}),
			Worker:  worker,
			Comment: comment,
		})
		c.Check(err, jc.ErrorIsNil)
		releaser <- r
	}()

	select {
	case <-s.notify:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("lock acquire didn't happen")
	}
	s.clock.Advance(wait)
	select {
	case s.allowAcquire <- struct{}{}:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("lock acquire didn't advance")
	}
	select {
	case r := <-releaser:
		return r
	case <-time.After(jujutesting.LongWait):
		c.Fatal("no releaser returned")
	}
	panic("unreachable")
}

// This method needs the released time to be after the current suite clock time.
func (s *lockSuite) addHistory(c *gc.C, worker, comment string, released string, waited, held time.Duration) {
	releasedTime, err := time.Parse("2006-01-02 15:04:05", released)
	c.Assert(err, jc.ErrorIsNil)
	// First, advance the lock to the request time.
	diff := releasedTime.Sub(s.clock.Now())
	diff -= waited + held
	s.clock.Advance(diff)
	releaser := s.addAcquired(c, worker, comment, waited)
	s.clock.Advance(held)
	releaser()
}

func (s *lockSuite) acquireLock(spec mutex.Spec) (mutex.Releaser, error) {
	s.notify <- struct{}{}
	select {
	case <-s.allowAcquire:
	case <-spec.Cancel:
		return nil, errors.New("cancelled")
	}
	return noOpReleaser{}, nil
}

type noOpReleaser struct{}

func (noOpReleaser) Release() {}

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

// This function is necessary for the interface that the mutex package
// requires for the clock, but this isn't used in this test's suite as
// we are mocking out the acquire function.
func (f *fakeClock) After(time.Duration) <-chan time.Time {
	return nil
}
