// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/cleaner"
)

type CleanerSuite struct {
	coretesting.BaseSuite
	mockState *cleanerMock
}

var _ = gc.Suite(&CleanerSuite{})

func (s *CleanerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockState = &cleanerMock{
		calls: make(chan string),
	}
	s.mockState.watcher = s.newMockNotifyWatcher(nil)
}

func (s *CleanerSuite) AssertReceived(c *gc.C, expect string) {
	select {
	case call := <-s.mockState.calls:
		c.Assert(call, gc.Matches, expect)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Timed out waiting for %s", expect)
	}
}

func (s *CleanerSuite) AssertEmpty(c *gc.C) {
	select {
	case call, ok := <-s.mockState.calls:
		c.Fatalf("Unexpected %s (ok: %v)", call, ok)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *CleanerSuite) TestCleaner(c *gc.C) {
	cln, err := cleaner.NewCleaner(s.mockState)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Stop(cln), jc.ErrorIsNil) }()

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")

	s.mockState.watcher.Change()
	s.AssertReceived(c, "Cleanup")
}

func (s *CleanerSuite) TestWatchCleanupsError(c *gc.C) {
	s.mockState.err = []error{errors.New("hello")}
	cln, err := cleaner.NewCleaner(s.mockState)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertReceived(c, "WatchCleanups")
	s.AssertEmpty(c)
	err = worker.Stop(cln)
	c.Assert(err, gc.ErrorMatches, "hello")
}

func (s *CleanerSuite) TestCleanupError(c *gc.C) {
	s.mockState.err = []error{nil, errors.New("hello")}
	cln, err := cleaner.NewCleaner(s.mockState)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")
	err = worker.Stop(cln)
	c.Assert(err, jc.ErrorIsNil)
	log := c.GetTestLog()
	c.Assert(log, jc.Contains, "ERROR juju.worker.cleaner cannot cleanup state: hello")
}

func (s *CleanerSuite) newMockNotifyWatcher(err error) *mockNotifyWatcher {
	m := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
		err:     err,
	}
	go func() {
		defer m.tomb.Done()
		defer m.tomb.Kill(m.err)
		<-m.tomb.Dying()
	}()
	s.AddCleanup(func(c *gc.C) {
		err := worker.Stop(m)
		c.Check(err, jc.ErrorIsNil)
	})
	m.Change()
	return m
}

type mockNotifyWatcher struct {
	watcher.NotifyWatcher

	tomb    tomb.Tomb
	err     error
	changes chan struct{}
}

func (m *mockNotifyWatcher) Kill() {
	m.tomb.Kill(nil)
}

func (m *mockNotifyWatcher) Wait() error {
	return m.tomb.Wait()
}

func (m *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return m.changes
}

func (m *mockNotifyWatcher) Change() {
	m.changes <- struct{}{}
}

// cleanerMock is used to check the
// calls of Cleanup() and WatchCleanups()
type cleanerMock struct {
	cleaner.StateCleaner
	watcher *mockNotifyWatcher
	calls   chan string
	err     []error
}

func (m *cleanerMock) getError() (e error) {
	if len(m.err) > 0 {
		e = m.err[0]
		m.err = m.err[1:]
	}
	return
}

func (m *cleanerMock) Cleanup() error {
	m.calls <- "Cleanup"
	return m.getError()
}

func (m *cleanerMock) WatchCleanups() (watcher.NotifyWatcher, error) {
	m.calls <- "WatchCleanups"
	return m.watcher, m.getError()
}
