// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/cleaner"
)

type CleanerSuite struct {
	coretesting.BaseSuite
	mockState *cleanerMock
	mockClock *testclock.Clock
	logger    logger.Logger
}

var _ = tc.Suite(&CleanerSuite{})

func (s *CleanerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockState = &cleanerMock{
		calls: make(chan string, 1),
	}
	s.mockState.watcher = s.newMockNotifyWatcher(nil)
	s.mockClock = testclock.NewClock(time.Time{})
	s.logger = loggertesting.WrapCheckLog(c)
}

func (s *CleanerSuite) AssertReceived(c *tc.C, expect string) {
	select {
	case call := <-s.mockState.calls:
		c.Assert(call, tc.Matches, expect)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Timed out waiting for %s", expect)
	}
}

func (s *CleanerSuite) AssertEmpty(c *tc.C) {
	select {
	case call, ok := <-s.mockState.calls:
		c.Fatalf("Unexpected %s (ok: %v)", call, ok)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *CleanerSuite) TestCleaner(c *tc.C) {
	cln, err := cleaner.NewCleaner(context.Background(), s.mockState, s.mockClock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Stop(cln), jc.ErrorIsNil) }()

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")
	s.AssertEmpty(c)

	s.mockState.watcher.Change()
	s.AssertReceived(c, "Cleanup")
	s.AssertEmpty(c)
}

func (s *CleanerSuite) TestCleanerPeriodic(c *tc.C) {
	cln, err := cleaner.NewCleaner(context.Background(), s.mockState, s.mockClock, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(worker.Stop(cln), jc.ErrorIsNil) }()

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")
	s.AssertEmpty(c)

	// The cleaner will start a timer that waits for 30 seconds after
	// each call to Cleanup, regardless of whether the previous call
	// to Cleanup was triggered by the watcher or a timer.
	for i := 0; i < 2; i++ {
		s.mockClock.WaitAdvance(29*time.Second, coretesting.LongWait, 1)
		s.AssertEmpty(c)
		s.mockClock.WaitAdvance(1*time.Second, coretesting.LongWait, 1)
		s.AssertReceived(c, "Cleanup")
		s.AssertEmpty(c)
	}
}

func (s *CleanerSuite) TestWatchCleanupsError(c *tc.C) {
	s.mockState.err = []error{errors.New("hello")}
	_, err := cleaner.NewCleaner(context.Background(), s.mockState, s.mockClock, s.logger)
	c.Assert(err, tc.ErrorMatches, "hello")

	s.AssertReceived(c, "WatchCleanups")
	s.AssertEmpty(c)
}

func (s *CleanerSuite) TestCleanupError(c *tc.C) {
	s.mockState.err = []error{nil, errors.New("hello")}
	cln, err := cleaner.NewCleaner(context.Background(), s.mockState, s.mockClock, s.logger)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertReceived(c, "WatchCleanups")
	s.AssertReceived(c, "Cleanup")
	err = worker.Stop(cln)
	c.Assert(err, jc.ErrorIsNil)
	log := c.GetTestLog()
	c.Assert(log[:len(log)-1], tc.Matches, "ERROR.*cannot cleanup state.*hello.*")
}

func (s *CleanerSuite) newMockNotifyWatcher(err error) *mockNotifyWatcher {
	m := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
		err:     err,
	}
	m.tomb.Go(func() error {
		<-m.tomb.Dying()
		return m.err
	})
	s.AddCleanup(func(c *tc.C) {
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

func (m *cleanerMock) Cleanup(context.Context) error {
	m.calls <- "Cleanup"
	return m.getError()
}

func (m *cleanerMock) WatchCleanups(context.Context) (watcher.NotifyWatcher, error) {
	m.calls <- "WatchCleanups"
	return m.watcher, m.getError()
}
