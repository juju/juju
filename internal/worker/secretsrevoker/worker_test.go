// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/secretsrevoker"
)

type workerSuite struct {
	testing.LoggingSuite

	facade *MockSecretsRevokerFacade
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facade = NewMockSecretsRevokerFacade(ctrl)
	return ctrl
}

func (s *workerSuite) TestWorkerWithSingleRevoke(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clk := testclock.NewDilatedWallClock(time.Millisecond)
	now := clk.Now()
	last := now.Add(10 * time.Minute)

	ch := make(chan []string, 1)
	ch <- []string(nil)
	expiryWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.CheckKilled(c, expiryWatcher)
	s.facade.EXPECT().WatchIssuedTokenExpiry().Return(expiryWatcher, nil)

	done := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		close(done)
		c.Assert(until, jc.After, last)
		return time.Time{}, nil
	})

	w, err := secretsrevoker.NewWorker(secretsrevoker.Config{
		Facade: s.facade,
		Logger: loggo.GetLogger("test"),
		Clock:  clk,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	ch <- []string{last.Format(time.RFC3339)}
	<-done
}

func (s *workerSuite) TestWorkerWithMoreToRevoke(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clk := testclock.NewDilatedWallClock(time.Millisecond)
	now := clk.Now().UTC()
	first := now.Add(10 * time.Minute)
	next := first.Add(10 * time.Minute)

	ch := make(chan []string, 1)
	ch <- []string(nil)
	expiryWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.CheckKilled(c, expiryWatcher)
	s.facade.EXPECT().WatchIssuedTokenExpiry().Return(expiryWatcher, nil)

	done := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		if until.After(next) {
			close(done)
			return time.Time{}, nil
		}
		c.Assert(until, jc.After, first)
		return next, nil
	}).Times(2)

	w, err := secretsrevoker.NewWorker(secretsrevoker.Config{
		Facade: s.facade,
		Logger: loggo.GetLogger("test"),
		Clock:  clk,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	ch <- []string{first.Format(time.RFC3339)}
	<-done
}

func (s *workerSuite) TestWorkerWithBreaks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clk := testclock.NewDilatedWallClock(time.Millisecond)

	ch := make(chan []string, 1)
	ch <- []string(nil)
	expiryWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.CheckKilled(c, expiryWatcher)
	s.facade.EXPECT().WatchIssuedTokenExpiry().Return(expiryWatcher, nil)

	w, err := secretsrevoker.NewWorker(secretsrevoker.Config{
		Facade: s.facade,
		Logger: loggo.GetLogger("test"),
		Clock:  clk,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	t := clk.Now().Add(30 * time.Second)
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		c.Assert(until, jc.After, t)
		t = clk.Now().Add(10 * time.Minute)
		return t, nil
	})
	ch <- []string{t.Format(time.RFC3339)}

	done := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		defer close(done)
		c.Assert(until, jc.After, t)
		return time.Time{}, nil
	})
	<-done

	// Break until a new send on the watcher.

	t = clk.Now().Add(30 * time.Second)
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		c.Assert(until, jc.After, t)
		t = clk.Now().Add(10 * time.Minute)
		return t, nil
	})
	ch <- []string{t.Format(time.RFC3339)}

	done = make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		defer close(done)
		c.Assert(until, jc.After, t)
		return time.Time{}, nil
	})
	<-done
}
