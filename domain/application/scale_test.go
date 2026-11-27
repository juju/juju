// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"testing"

	"github.com/juju/tc"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	coretesting "github.com/juju/juju/internal/testing"
)

type k8sUnitRangeSuite struct {
	coretesting.BaseSuite
}

func TestK8sUnitRangeSuite(t *testing.T) {
	tc.Run(t, &k8sUnitRangeSuite{})
}

func (s *k8sUnitRangeSuite) TestContiguousRange(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/0", "foo/1", "foo/2"}, 2)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *k8sUnitRangeSuite) TestNonZeroStart(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/2", "foo/3"}, 3)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *k8sUnitRangeSuite) TestSingleUnit(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/9"}, 9)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *k8sUnitRangeSuite) TestUnorderedRangePreservesInput(c *tc.C) {
	alive := []coreunit.Name{"foo/4", "foo/2", "foo/3"}
	err := application.ValidateK8sUnitRange("foo", alive, 4)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(alive, tc.DeepEquals, []coreunit.Name{"foo/4", "foo/2", "foo/3"})
}

func (s *k8sUnitRangeSuite) TestEmptyRangeRetainsHistory(c *tc.C) {
	for _, alive := range [][]coreunit.Name{nil, {}} {
		err := application.ValidateK8sUnitRange("foo", alive, 5)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *k8sUnitRangeSuite) TestSparseRange(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/0", "foo/2"}, 2)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	c.Check(err, tc.ErrorMatches, "unit range is not contiguous between ordinals 0 and 2: .*")
}

func (s *k8sUnitRangeSuite) TestDuplicateOrdinal(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/1", "foo/1"}, 1)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	c.Check(err, tc.ErrorMatches, "unit range is not contiguous between ordinals 1 and 1: .*")
}

func (s *k8sUnitRangeSuite) TestInvalidUnitNames(c *tc.C) {
	for _, name := range []coreunit.Name{"", "foo", "foo/invalid", "foo/-1"} {
		err := application.ValidateK8sUnitRange("foo", []coreunit.Name{name}, 0)
		c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent, tc.Commentf("%q", name))
	}
}

func (s *k8sUnitRangeSuite) TestUnitFromAnotherApplication(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/0", "bar/1"}, 1)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	c.Check(err, tc.ErrorMatches, `invalid unit name "bar/1": .*`)
}

func (s *k8sUnitRangeSuite) TestSequenceAheadOfRange(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/0", "foo/1"}, 5)
	c.Assert(err, tc.ErrorIs, applicationerrors.ScalingStateInconsistent)
	c.Check(err, tc.ErrorMatches, "unit sequence 5 exceeds highest Alive ordinal 1: .*")
}

func (s *k8sUnitRangeSuite) TestSequenceBelowRange(c *tc.C) {
	err := application.ValidateK8sUnitRange("foo", []coreunit.Name{"foo/2", "foo/3"}, 1)
	c.Assert(err, tc.ErrorIsNil)
}
