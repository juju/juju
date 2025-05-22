// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
)


type testingSuite struct{}

func TestTestingSuite(t *stdtesting.T) { tc.Run(t, &testingSuite{}) }
func (*testingSuite) TestSaveAttemptStrategiesSaves(c *tc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	attempt := utils.AttemptStrategy{
		Total: time.Second,
		Delay: time.Millisecond,
	}

	snapshot := saveAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Assert(snapshot, tc.HasLen, 1)
	c.Check(snapshot[0].address, tc.Equals, &attempt)
	c.Check(snapshot[0].original, tc.DeepEquals, attempt)
}

func (*testingSuite) TestSaveAttemptStrategiesLeavesOriginalsIntact(c *tc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	original := utils.AttemptStrategy{
		Total: time.Second,
		Delay: time.Millisecond,
	}
	attempt := original

	saveAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Check(attempt, tc.DeepEquals, original)
}

func (*testingSuite) TestInternalPatchAttemptStrategiesPatches(c *tc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	attempt := utils.AttemptStrategy{
		Total: 33 * time.Millisecond,
		Delay: 99 * time.Microsecond,
	}
	c.Assert(attempt, tc.Not(tc.DeepEquals), impatientAttempt)

	internalPatchAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Check(attempt, tc.DeepEquals, impatientAttempt)
}

// internalPatchAttemptStrategies returns a cleanup function that restores
// the given strategies to their original configurations.  For simplicity,
// these tests take this as sufficient proof that any strategy that gets
// patched, also gets restored by the cleanup function.
func (*testingSuite) TestInternalPatchAttemptStrategiesReturnsCleanup(c *tc.C) {
	// TODO(katco): 2016-08-09: lp:1611427
	original := utils.AttemptStrategy{
		Total: 22 * time.Millisecond,
		Delay: 77 * time.Microsecond,
	}
	c.Assert(original, tc.Not(tc.DeepEquals), impatientAttempt)
	attempt := original

	cleanup := internalPatchAttemptStrategies([]*utils.AttemptStrategy{&attempt})
	cleanup()

	c.Check(attempt, tc.DeepEquals, original)
}

func (*testingSuite) TestPatchAttemptStrategiesPatchesEnvironsStrategies(c *tc.C) {
	c.Assert(common.LongAttempt, tc.Not(tc.DeepEquals), impatientAttempt)
	c.Assert(common.ShortAttempt, tc.Not(tc.DeepEquals), impatientAttempt)
	c.Assert(environs.AddressesRefreshAttempt, tc.Not(tc.DeepEquals), impatientAttempt)

	cleanup := PatchAttemptStrategies()
	defer cleanup()

	c.Check(common.LongAttempt, tc.DeepEquals, impatientAttempt)
	c.Check(common.ShortAttempt, tc.DeepEquals, impatientAttempt)
	c.Check(environs.AddressesRefreshAttempt, tc.DeepEquals, impatientAttempt)
}

func (*testingSuite) TestPatchAttemptStrategiesPatchesGivenAttempts(c *tc.C) {
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

	c.Check(attempt1, tc.DeepEquals, impatientAttempt)
	c.Check(attempt2, tc.DeepEquals, impatientAttempt)
}

// TODO(jack-w-shaw): 2022-01-21: Implementing funcs for both 'AttemptStrategy'
// patching and 'RetryStrategy' patching whilst lp:1611427 is in progress
//
// Remove AttemptStrategy patching when they are no longer in use i.e. when
// lp issue is resolved

func (*testingSuite) TestSaveRetrytrategiesSaves(c *tc.C) {
	retryStrategy := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: time.Second,
		Delay:       time.Millisecond,
	}

	snapshot := saveRetryStrategies([]*retry.CallArgs{&retryStrategy})

	c.Assert(snapshot, tc.HasLen, 1)
	c.Check(snapshot[0].address, tc.Equals, &retryStrategy)
	c.Check(snapshot[0].original, tc.DeepEquals, retryStrategy)
}

func (*testingSuite) TestSaveRetryStrategiesLeavesOriginalsIntact(c *tc.C) {
	original := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: time.Second,
		Delay:       time.Millisecond,
	}
	retryStrategy := original

	saveRetryStrategies([]*retry.CallArgs{&retryStrategy})

	c.Check(retryStrategy, tc.DeepEquals, original)
}

func (*testingSuite) TestInternalPatchRetryStrategiesPatches(c *tc.C) {
	retryStrategy := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 33 * time.Millisecond,
		Delay:       99 * time.Microsecond,
	}
	c.Assert(retryStrategy, tc.Not(tc.DeepEquals), impatientRetryStrategy)

	internalPatchRetryStrategies([]*retry.CallArgs{&retryStrategy})

	c.Check(retryStrategy, tc.DeepEquals, impatientRetryStrategy)
}

// internalPatchAttemptStrategies returns a cleanup function that restores
// the given strategies to their original configurations.  For simplicity,
// these tests take this as sufficient proof that any strategy that gets
// patched, also gets restored by the cleanup function.
func (*testingSuite) TestInternalPatchRetryStrategiesReturnsCleanup(c *tc.C) {
	original := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 22 * time.Millisecond,
		Delay:       77 * time.Microsecond,
	}
	c.Assert(original, tc.Not(tc.DeepEquals), impatientRetryStrategy)
	retryStrategy := original

	cleanup := internalPatchRetryStrategies([]*retry.CallArgs{&retryStrategy})
	cleanup()

	c.Check(retryStrategy, tc.DeepEquals, original)
}

func (*testingSuite) TestPatchRetryStrategiesPatchesGivenRetries(c *tc.C) {
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

	c.Check(retryStrategy1, tc.DeepEquals, impatientRetryStrategy)
	c.Check(retryStrategy2, tc.DeepEquals, impatientRetryStrategy)
}
