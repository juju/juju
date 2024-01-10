// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"
)

type baseObjectStoreSuite struct {
	testing.IsolationSuite

	clock         *MockClock
	claimer       *MockClaimer
	claimExtender *MockClaimExtender
}

var _ = gc.Suite(&baseObjectStoreSuite{})

func (s *baseObjectStoreSuite) TestScopedContext(c *gc.C) {
	w := &baseObjectStore{}

	ctx, cancel := w.scopedContext()
	c.Assert(ctx.Err(), gc.IsNil)

	cancel()
	c.Assert(ctx.Err(), jc.ErrorIs, context.Canceled)
}

func (s *baseObjectStoreSuite) TestScopedContextTomb(c *gc.C) {
	w := &baseObjectStore{}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), gc.IsNil)

	w.tomb.Kill(nil)

	c.Assert(ctx.Err(), jc.ErrorIs, context.Canceled)
}

func (s *baseObjectStoreSuite) TestLockOnCancelledContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.

	w := &baseObjectStore{
		claimer: s.claimer,
	}

	ctx, cancel := w.scopedContext()
	c.Assert(ctx.Err(), jc.ErrorIsNil)

	cancel()

	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		return nil
	})
	c.Assert(err, jc.ErrorIs, context.Canceled)
}

func (s *baseObjectStoreSuite) TestLocking(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.

	s.expectClaimRelease("hash")
	s.expectExtendDuration(time.Second)
	s.expectClockAfter(make(chan time.Time))

	w := &baseObjectStore{
		claimer: s.claimer,
		clock:   s.clock,
	}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), jc.ErrorIsNil)

	var called bool
	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		called = true
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *baseObjectStoreSuite) TestLockingForBlockedFunc(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.
	// Ensure if there is a blocking call, that the lock is extended.

	block := make(chan struct{})
	wait := make(chan time.Time)

	s.expectClaimRelease("hash")
	s.expectClockAfter(wait)
	s.expectExtendDuration(time.Second)
	s.expectClockAfter(make(chan time.Time))

	w := &baseObjectStore{
		claimer: s.claimer,
		clock:   s.clock,
	}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), jc.ErrorIsNil)

	s.claimExtender.EXPECT().Extend(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		close(block)
		return nil
	})

	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		close(wait)

		select {
		case <-block:
			return nil
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for block")
			return nil
		}
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseObjectStoreSuite) TestBlockedLock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.
	// Ensure if there is a blocking call, that the lock is extended.

	s.expectClaimRelease("hash")

	var attempts int
	s.expectExtend(time.Millisecond*100, func() {
		attempts++
	})

	w := &baseObjectStore{
		claimer: s.claimer,
		clock:   clock.WallClock,
	}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), jc.ErrorIsNil)

	var called bool
	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		time.Sleep(time.Second)
		called = true
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)

	// Make sure we've extended the lock. 5 is just arbitrary, but should be
	// enough to ensure we've extended the lock. In theory it should be at
	// least 9, but we have to account for slowness of runners.
	c.Check(attempts > 5, jc.IsTrue)
}

func (s *baseObjectStoreSuite) TestLockingForTombKill(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.
	// Ensure if there is a blocking call, that the lock is extended.

	block := make(chan struct{})

	s.expectClaimRelease("hash")
	s.expectExtendDuration(time.Second)

	w := &baseObjectStore{
		claimer: s.claimer,
		clock:   clock.WallClock,
	}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), jc.ErrorIsNil)

	wait := make(chan struct{})

	go func() {
		select {
		case <-block:
			w.Kill()
			close(wait)
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for block")
		}
	}()

	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		close(block)
		time.Sleep(time.Millisecond * 100)
		return nil
	})
	c.Assert(err, jc.ErrorIs, tomb.ErrDying)

	select {
	case <-wait:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for complete state")
	}
}

func (s *baseObjectStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.claimer = NewMockClaimer(ctrl)
	s.claimExtender = NewMockClaimExtender(ctrl)

	return ctrl
}

func (s *baseObjectStoreSuite) expectClockAfter(ch chan time.Time) {
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(dur time.Duration) <-chan time.Time {
		return ch
	})
}

func (s *baseObjectStoreSuite) expectClaimRelease(hash string) {
	s.claimer.EXPECT().Claim(gomock.Any(), hash).Return(s.claimExtender, nil)
	s.claimer.EXPECT().Release(gomock.Any(), hash)
}

func (s *baseObjectStoreSuite) expectExtendDuration(dur time.Duration) {
	s.claimExtender.EXPECT().Duration().Return(dur).AnyTimes()
}

func (s *baseObjectStoreSuite) expectExtend(dur time.Duration, fn func()) {
	s.claimExtender.EXPECT().Duration().Return(dur).AnyTimes()
	s.claimExtender.EXPECT().Extend(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		fn()
		return nil
	}).AnyTimes()
}
