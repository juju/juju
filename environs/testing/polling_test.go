// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/utils"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type testingSuite struct{}

var _ = gc.Suite(&testingSuite{})

func (*testingSuite) TestSaveAttemptStrategiesSaves(c *gc.C) {
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
	original := utils.AttemptStrategy{
		Total: time.Second,
		Delay: time.Millisecond,
	}
	attempt := original

	saveAttemptStrategies([]*utils.AttemptStrategy{&attempt})

	c.Check(attempt, gc.DeepEquals, original)
}

func (*testingSuite) TestInternalPatchAttemptStrategiesPatches(c *gc.C) {
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

	cleanup := PatchAttemptStrategies()
	defer cleanup()

	c.Check(common.LongAttempt, gc.DeepEquals, impatientAttempt)
	c.Check(common.ShortAttempt, gc.DeepEquals, impatientAttempt)
}

func (*testingSuite) TestPatchAttemptStrategiesPatchesGivenAttempts(c *gc.C) {
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
