// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iter

import (
	"slices"
	"testing"

	"github.com/juju/tc"
)

// transformSeqSuite defines a suite of tests for asserting the behaviour of
// [TransformSeq].
type transformSeqSuite struct{}

// TestTransformSeqSuite runs the suite of tests for [TransformSeq].
func TestTransformSeqSuite(t *testing.T) {
	tc.Run(t, transformSeqSuite{})
}

// TestAllValues asserts that TransformSeq correctly applies the transform
// function to every value in the sequence, returning a new sequence containing
// all transformed values in the original order.
func (transformSeqSuite) TestAllValues(c *tc.C) {
	seq := TransformSeq(
		slices.Values([]int{1, 2, 3}),
		func(v int) int { return v * 2 },
	)
	c.Assert(slices.Collect(seq), tc.DeepEquals, []int{2, 4, 6})
}

// TestSeqEmpty asserts that when TransformSeq is given an empty sequence, the
// returned sequence produces no values and the transform function is never
// invoked.
func (transformSeqSuite) TestSeqEmpty(c *tc.C) {
	seq := TransformSeq(
		slices.Values([]int{}),
		func(v int) int { return v * 2 },
	)
	c.Assert(slices.Collect(seq), tc.DeepEquals, []int{})
}
