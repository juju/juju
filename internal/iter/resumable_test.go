// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package iter

import (
	"slices"
	"testing"

	"github.com/juju/tc"
)

// resumableSeqSuite defines a suite of tests for asserting the behaviour of
// [ResumableSeq].
type resumableSeqSuite struct{}

// TestResumableSeqSuite runs the suite of tests for [ResumableSeq].
func TestResumableSeqSuite(t *testing.T) {
	tc.Run(t, resumableSeqSuite{})
}

// TestResume asserts that when a yield function returns false, causing early
// termination, subsequent calls to the sequence resume from the next unconsumed
// value rather than restarting from the beginning of the sequence.
func (resumableSeqSuite) TestResume(c *tc.C) {
	t := []int{1, 2, 3}
	seq, close := ResumableSeq(slices.Values(t))
	defer close()
	seq(func(v int) bool {
		c.Check(v, tc.Equals, 1)
		return false
	})
	seq(func(v int) bool {
		c.Check(v, tc.Equals, 2)
		return false
	})
	seq(func(v int) bool {
		c.Check(v, tc.Equals, 3)
		return false
	})
}

// TestResumeEmpty asserts that when [ResumableSeq] is given an empty sequence,
// the returned sequence terminates immediately without invoking the yield
// function.
func (resumableSeqSuite) TestResumeEmpty(c *tc.C) {
	seq, close := ResumableSeq(slices.Values([]int{}))
	defer close()

	seq(func(v int) bool {
		c.Error("yield called on empty resumable sequence")
		return false
	})
}
