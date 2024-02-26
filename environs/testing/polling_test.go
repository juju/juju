// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type testingSuite struct{}

var _ = gc.Suite(&testingSuite{})

func (*testingSuite) TestSaveAttemptStrategiesSaves(c *gc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	attempt := utils.AttemptStrategy{
		Total: time.Second,
		Delay: time.Millisecond,
	}

	snapshot := saveAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Assert(snapshot, gc.HasLen, 1)
	c.Check(snapshot[0].address, gc.Equals, &attempt)
	c.Check(snapshot[0].original, gc.DeepEquals, attempt)
}

func (*testingSuite) TestSaveAttemptStrategiesLeavesOriginalsIntact(c *gc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	original := utils.AttemptStrategy{
		Total: time.Second,
		Delay: time.Millisecond,
	}
	attempt := original

	saveAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Check(attempt, gc.DeepEquals, original)
}

func (*testingSuite) TestInternalPatchAttemptStrategiesPatches(c *gc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	attempt := utils.AttemptStrategy{
		Total: 33 * time.Millisecond,
		Delay: 99 * time.Microsecond,
	}
	c.Assert(attempt, gc.Not(gc.DeepEquals), impatientAttempt)

	internalPatchAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Check(attempt, gc.DeepEquals, impatientAttempt)
}

// internalPatchAttemptStrategies returns a cleanup function that restores
// the given strategies to their original configurations.  For simplicity,
// these tests take this as sufficient proof that any strategy that gets
// patched, also gets restored by the cleanup function.
func (*testingSuite) TestInternalPatchAttemptStrategiesReturnsCleanup(c *gc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	original := utils.AttemptStrategy{
		Total: 22 * time.Millisecond,
		Delay: 77 * time.Microsecond,
	}
	c.Assert(original, gc.Not(gc.DeepEquals), impatientAttempt)
	attempt := original

	cleanup := internalPatchAttemptStrategies([]*utils.AttemptStrategy{&attempt})
	cleanup()

	c.Check(attempt, gc.DeepEquals, original)
}

func (*testingSuite) TestPatchAttemptStrategiesPatchesEnvironsStrategies(c *gc.C) {
	c.Assert(common.LongAttempt, gc.Not(gc.DeepEquals), impatientAttempt)
	c.Assert(common.ShortAttempt, gc.Not(gc.DeepEquals), impatientAttempt)
	c.Assert(environs.AddressesRefreshAttempt, gc.Not(gc.DeepEquals), impatientAttempt)

	cleanup := PatchAttemptStrategies()
	defer cleanup()

	c.Check(common.LongAttempt, gc.DeepEquals, impatientAttempt)
	c.Check(common.ShortAttempt, gc.DeepEquals, impatientAttempt)
	c.Check(environs.AddressesRefreshAttempt, gc.DeepEquals, impatientAttempt)
}

func (*testingSuite) TestPatchAttemptStrategiesPatchesGivenAttempts(c *gc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	attempt1 := utils.AttemptStrategy{
		Total: 33 * time.Millisecond,
		Delay: 99 * time.Microsecond,
	}
	attempt2 := utils.AttemptStrategy{
		Total: 82 * time.Microsecond,
		Delay: 62 * time.Nanosecond,
	}

	cleanup := PatchAttemptStrategies(&attempt1, &attempt2)
	defer cleanup()

	c.Check(attempt1, gc.DeepEquals, impatientAttempt)
	c.Check(attempt2, gc.DeepEquals, impatientAttempt)
}

// TODO(jack-w-shaw): 2022-01-21: Implementing funcs for both 'AttemptStrategy'
// patching and 'RetryStrategy' patching whilst lp:1611427 is in progress
//
// Remove AttemptStrategy patching when they are no longer in use i.e. when
// lp issue is resolved

func (*testingSuite) TestSaveRetrytrategiesSaves(c *gc.C) {
	retryStrategy := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: time.Second,
		Delay:       time.Millisecond,
	}

	snapshot := saveRetryStrategies([]*retry.CallArgs{&retryStrategy})

	c.Assert(snapshot, gc.HasLen, 1)
	c.Check(snapshot[0].address, gc.Equals, &retryStrategy)
	c.Check(snapshot[0].original, gc.DeepEquals, retryStrategy)
}

func (*testingSuite) TestSaveRetryStrategiesLeavesOriginalsIntact(c *gc.C) {
	original := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: time.Second,
		Delay:       time.Millisecond,
	}
	retryStrategy := original

	saveRetryStrategies([]*retry.CallArgs{&retryStrategy})

	c.Check(retryStrategy, gc.DeepEquals, original)
}

func (*testingSuite) TestInternalPatchRetryStrategiesPatches(c *gc.C) {
	retryStrategy := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 33 * time.Millisecond,
		Delay:       99 * time.Microsecond,
	}
	c.Assert(retryStrategy, gc.Not(gc.DeepEquals), impatientRetryStrategy)

	internalPatchRetryStrategies([]*retry.CallArgs{&retryStrategy})

	c.Check(retryStrategy, gc.DeepEquals, impatientRetryStrategy)
}

// internalPatchAttemptStrategies returns a cleanup function that restores
// the given strategies to their original configurations.  For simplicity,
// these tests take this as sufficient proof that any strategy that gets
// patched, also gets restored by the cleanup function.
func (*testingSuite) TestInternalPatchRetryStrategiesReturnsCleanup(c *gc.C) {
	original := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 22 * time.Millisecond,
		Delay:       77 * time.Microsecond,
	}
	c.Assert(original, gc.Not(gc.DeepEquals), impatientRetryStrategy)
	retryStrategy := original

	cleanup := internalPatchRetryStrategies([]*retry.CallArgs{&retryStrategy})
	cleanup()

	c.Check(retryStrategy, gc.DeepEquals, original)
}

func (*testingSuite) TestPatchRetryStrategiesPatchesGivenRetries(c *gc.C) {
	retryStrategy1 := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 33 * time.Millisecond,
		Delay:       99 * time.Microsecond,
	}
	retryStrategy2 := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 82 * time.Microsecond,
		Delay:       62 * time.Nanosecond,
	}

	cleanup := PatchRetryStrategies(&retryStrategy1, &retryStrategy2)
	defer cleanup()

	c.Check(retryStrategy1, gc.DeepEquals, impatientRetryStrategy)
	c.Check(retryStrategy2, gc.DeepEquals, impatientRetryStrategy)
}
