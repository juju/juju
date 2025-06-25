// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type baseObjectStoreSuite struct {
	testhelpers.IsolationSuite

	clock         *MockClock
	claimer       *MockClaimer
	claimExtender *MockClaimExtender
}

func TestBaseObjectStoreSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &baseObjectStoreSuite{})
}

func (s *baseObjectStoreSuite) TestScopedContext(c *tc.C) {
	w := &baseObjectStore{}

	ctx, cancel := w.scopedContext()
	c.Assert(ctx.Err(), tc.IsNil)

	cancel()
	c.Assert(ctx.Err(), tc.ErrorIs, context.Canceled)
}

func (s *baseObjectStoreSuite) TestScopedContextTomb(c *tc.C) {
	w := &baseObjectStore{}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), tc.IsNil)

	w.tomb.Kill(nil)

	c.Assert(ctx.Err(), tc.ErrorIs, context.Canceled)
}

func (s *baseObjectStoreSuite) TestLockOnCancelledContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.

	w := &baseObjectStore{
		claimer: s.claimer,
	}

	ctx, cancel := w.scopedContext()
	c.Assert(ctx.Err(), tc.ErrorIsNil)

	cancel()

	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		return nil
	})
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *baseObjectStoreSuite) TestLocking(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Expect the claimer to be called and then released when the lock is
	// released.

	s.expectClaimRelease("hash")
	s.expectExtendDuration(time.Second)

	// The clock after might not be called, as we might schedule the goroutine
	// fast enough, that the second goroutine isn't called.
	s.maybeExpectClockAfter(make(chan time.Time))

	w := &baseObjectStore{
		claimer: s.claimer,
		clock:   s.clock,
	}

	ctx, _ := w.scopedContext()
	c.Assert(ctx.Err(), tc.ErrorIsNil)

	var called bool
	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *baseObjectStoreSuite) TestLockingForBlockedFunc(c *tc.C) {
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
	c.Assert(ctx.Err(), tc.ErrorIsNil)

	s.claimExtender.EXPECT().Extend(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		close(block)
		return nil
	})

	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		close(wait)

		select {
		case <-block:
			return nil
		case <-time.After(testhelpers.LongWait):
			c.Fatal("timed out waiting for block")
			return nil
		}
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseObjectStoreSuite) TestBlockedLock(c *tc.C) {
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
	c.Assert(ctx.Err(), tc.ErrorIsNil)

	var called bool
	err := w.withLock(ctx, "hash", func(ctx context.Context) error {
		time.Sleep(time.Second)
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)

	// Make sure we've extended the lock. 5 is just arbitrary, but should be
	// enough to ensure we've extended the lock. In theory it should be at
	// least 9, but we have to account for slowness of runners.
	c.Check(attempts > 5, tc.IsTrue)
}

func (s *baseObjectStoreSuite) TestLockingForTombKill(c *tc.C) {
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

	wait := make(chan struct{})

	go func() {
		<-block
		w.Kill()
		close(wait)
	}()

	err := w.withLock(c.Context(), "hash", func(ctx context.Context) error {
		close(block)
		time.Sleep(time.Millisecond * 100)
		return nil
	})
	c.Assert(err, tc.ErrorIs, tomb.ErrDying)

	select {
	case <-wait:
	case <-time.After(testhelpers.LongWait):
		c.Fatal("timed out waiting for complete state")
	}
}

func (s *baseObjectStoreSuite) TestPruneWithNoData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we don't panic if we have no data to prune.

	w := &baseObjectStore{
		logger:  loggertesting.WrapCheckLog(c),
		claimer: s.claimer,
		clock:   clock.WallClock,
	}

	list := func(ctx context.Context) ([]objectstore.Metadata, []string, error) {
		return nil, nil, nil
	}
	delete := func(ctx context.Context, hash string) error {
		c.Fatalf("failed if called")
		return nil
	}

	err := w.prune(c.Context(), list, delete)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseObjectStoreSuite) TestPruneWithJustMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// If we only have metadata and no objects, we expect no pruning to occur.

	w := &baseObjectStore{
		logger:  loggertesting.WrapCheckLog(c),
		claimer: s.claimer,
		clock:   clock.WallClock,
	}

	list := func(ctx context.Context) ([]objectstore.Metadata, []string, error) {
		return []objectstore.Metadata{{
			SHA384: "hash",
		}}, nil, nil
	}
	delete := func(ctx context.Context, hash string) error {
		c.Fatalf("failed if called")
		return nil
	}

	err := w.prune(c.Context(), list, delete)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseObjectStoreSuite) TestPruneWithJustObjects(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Expect that we delete the objects if we have no metadata.

	s.expectClaimRelease("foo")
	s.expectExtendDuration(time.Second)

	w := &baseObjectStore{
		logger:  loggertesting.WrapCheckLog(c),
		claimer: s.claimer,
		clock:   clock.WallClock,
	}

	list := func(ctx context.Context) ([]objectstore.Metadata, []string, error) {
		return nil, []string{"foo"}, nil
	}
	delete := func(ctx context.Context, hash string) error {
		c.Check(hash, tc.Equals, "foo")
		return nil
	}

	err := w.prune(c.Context(), list, delete)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseObjectStoreSuite) TestPruneWithMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Expect that we delete the objects if we have no metadata and ignore
	// the ones that do.

	s.expectClaimRelease("foo")
	s.expectExtendDuration(time.Second)

	w := &baseObjectStore{
		logger:  loggertesting.WrapCheckLog(c),
		claimer: s.claimer,
		clock:   clock.WallClock,
	}

	list := func(ctx context.Context) ([]objectstore.Metadata, []string, error) {
		return []objectstore.Metadata{{
			SHA384: "bar",
		}}, []string{"bar", "foo"}, nil
	}
	delete := func(ctx context.Context, hash string) error {
		c.Check(hash, tc.Equals, "foo")
		return nil
	}

	err := w.prune(c.Context(), list, delete)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseObjectStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.claimer = NewMockClaimer(ctrl)
	s.claimExtender = NewMockClaimExtender(ctrl)

	return ctrl
}

func (s *baseObjectStoreSuite) maybeExpectClockAfter(ch chan time.Time) {
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(dur time.Duration) <-chan time.Time {
		return ch
	}).AnyTimes()
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
