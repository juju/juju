// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"errors"
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/cleaner"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type CleanerSuite struct {
	coretesting.BaseSuite
	mockState *cleanerMock
}

var _ = gc.Suite(&CleanerSuite{})

var _ worker.NotifyWatchHandler = (*cleaner.Cleaner)(nil)

func (s *CleanerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockState = &cleanerMock{
		calls: make(chan string),
	}
	s.mockState.watcher = newMockNotifyWatcher(nil)
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
	cln := cleaner.NewCleaner(s.mockState)
	defer func() { c.Assert(worker.Stop(cln), jc.ErrorIsNil) }()

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")

	s.mockState.watcher.Change()
	s.AssertReceived(c, "Cleanup")
}

func (s *CleanerSuite) TestWatchCleanupsError(c *gc.C) {
	s.mockState.err = []error{errors.New("hello")}
	cln := cleaner.NewCleaner(s.mockState)

	s.AssertReceived(c, "WatchCleanups")
	s.AssertEmpty(c)
	err := worker.Stop(cln)
	c.Assert(err, gc.ErrorMatches, "hello")
}

func (s *CleanerSuite) TestCleanupError(c *gc.C) {
	s.mockState.err = []error{nil, errors.New("hello")}
	cln := cleaner.NewCleaner(s.mockState)

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")
	err := worker.Stop(cln)
	c.Assert(err, jc.ErrorIsNil)
	log := c.GetTestLog()
	c.Assert(log, jc.Contains, "ERROR juju.worker.cleaner cannot cleanup state: hello")
}

// cleanerMock is used to check the
// calls of Cleanup() and WatchCleanups()
type cleanerMock struct {
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

var _ cleaner.StateCleaner = (*cleanerMock)(nil)

type mockNotifyWatcher struct {
	watcher.NotifyWatcher

	err     error
	changes chan struct{}
}

func newMockNotifyWatcher(err error) *mockNotifyWatcher {
	m := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
		err:     err,
	}
	m.Change()
	return m
}

func (m *mockNotifyWatcher) Err() error {
	return m.err
}

func (m *mockNotifyWatcher) Changes() <-chan struct{} {
	if m.err != nil {
		close(m.changes)
	}
	return m.changes
}

func (m *mockNotifyWatcher) Stop() error {
	return m.err
}

func (m *mockNotifyWatcher) Change() {
	m.changes <- struct{}{}
}
