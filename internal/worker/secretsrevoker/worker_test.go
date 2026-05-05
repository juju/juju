// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker_test

import (
	"math/rand"
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
		Facade:       s.facade,
		Logger:       loggo.GetLogger("test"),
		Clock:        clk,
		QuantiseTime: secretsrevoker.DefaultQuantiseTime,
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
		Facade:       s.facade,
		Logger:       loggo.GetLogger("test"),
		Clock:        clk,
		QuantiseTime: secretsrevoker.DefaultQuantiseTime,
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
		Facade:       s.facade,
		Logger:       loggo.GetLogger("test"),
		Clock:        clk,
		QuantiseTime: secretsrevoker.DefaultQuantiseTime,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	t0 := clk.Now().Add(30 * time.Second)
	t1 := t0.Add(10 * time.Minute)
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		c.Assert(until, jc.After, t0)
		return t1, nil
	})
	done := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		defer close(done)
		c.Assert(until, jc.After, t1)
		return time.Time{}, nil
	})
	ch <- []string{t0.Format(time.RFC3339)}
	<-done

	// Break until a new send on the watcher.

	t2 := clk.Now().Add(30 * time.Second)
	t3 := t2.Add(10 * time.Minute)
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		c.Assert(until, jc.After, t2)
		return t3, nil
	})
	done2 := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(
		gomock.Any(),
	).DoAndReturn(func(until time.Time) (time.Time, error) {
		defer close(done2)
		c.Assert(until, jc.After, t3)
		return time.Time{}, nil
	})
	ch <- []string{t2.Format(time.RFC3339)}
	<-done2
}

func (s *workerSuite) TestWorkerQuantisedSchedule(c *gc.C) {
	defer s.setupMocks(c).Finish()

	const iterations = 100
	clk := testclock.NewDilatedWallClock(50 * time.Microsecond)
	type scheduledExpiry struct {
		when      time.Time
		quantised time.Time
	}
	times := make([]scheduledExpiry, 0, iterations)
	now := clk.Now().UTC().Truncate(time.Second)
	for range iterations {
		next := now.Add(time.Duration(rand.Intn(600)+1) * time.Second)
		times = append(times, scheduledExpiry{
			when:      next,
			quantised: secretsrevoker.DefaultQuantiseTime(next),
		})
		now = next
	}
	first := times[0].when

	done := make(chan struct{})
	for i, nextExpiry := range times {
		s.facade.EXPECT().RevokeIssuedTokens(gomock.Any()).DoAndReturn(func(until time.Time) (time.Time, error) {
			if !until.Equal(nextExpiry.quantised) {
				// If the clock has already passed the target expiry, the worker
				// intentionally schedules immediate revocation.
				c.Assert(until, jc.Before, clk.Now().Add(2*time.Second))
			}
			if i+1 >= len(times) {
				close(done)
				return time.Time{}, nil
			}
			return times[i+1].when, nil
		})
	}

	ch := make(chan []string, 1)
	ch <- []string(nil)
	expiryWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.CheckKilled(c, expiryWatcher)
	s.facade.EXPECT().WatchIssuedTokenExpiry().Return(expiryWatcher, nil)

	w, err := secretsrevoker.NewWorker(secretsrevoker.Config{
		Facade:       s.facade,
		Logger:       loggo.GetLogger("test"),
		Clock:        clk,
		QuantiseTime: secretsrevoker.DefaultQuantiseTime,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	ch <- []string{first.Format(time.RFC3339)}
	<-done
}

func (s *workerSuite) TestWorkerSchedulesImmediateForPastDueExpiry(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clk := testclock.NewDilatedWallClock(time.Millisecond)
	now := clk.Now().UTC().Truncate(time.Second)
	pastDue := now.Add(-10 * time.Second)

	ch := make(chan []string, 1)
	ch <- []string(nil)
	expiryWatcher := watchertest.NewMockStringsWatcher(ch)
	defer workertest.CheckKilled(c, expiryWatcher)
	s.facade.EXPECT().WatchIssuedTokenExpiry().Return(expiryWatcher, nil)

	done := make(chan struct{})
	s.facade.EXPECT().RevokeIssuedTokens(gomock.Any()).DoAndReturn(func(until time.Time) (time.Time, error) {
		defer close(done)
		c.Assert(until, jc.Before, now.Add(5*time.Second))
		return time.Time{}, nil
	})

	w, err := secretsrevoker.NewWorker(secretsrevoker.Config{
		Facade: s.facade,
		Logger: loggo.GetLogger("test"),
		Clock:  clk,
		QuantiseTime: func(t time.Time) time.Time {
			// Use a large offset to make accidental quantisation obvious.
			return t.Add(10 * time.Minute)
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	ch <- []string{pastDue.Format(time.RFC3339)}
	<-done
}

// TestDefaultQuantiseTimeFunction checks that the default time quantisation
// function creates minute buckets.
func (s *workerSuite) TestDefaultQuantiseTimeFunction(c *gc.C) {
	unique := 0
	last := time.Time{}.Add(time.Minute)
	accum := time.Time{}
	for range 60 {
		accum = accum.Add(10 * time.Second)
		accumQuant := secretsrevoker.DefaultQuantiseTime(accum)
		if last != accumQuant {
			unique++
			last = accumQuant
		}
	}
	c.Assert(unique, gc.Equals, 10)
}
