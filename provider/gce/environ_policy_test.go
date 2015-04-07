// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/testing"
)

type environPolSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environPolSuite{})

func (s *environPolSuite) TestPrecheckInstance(c *gc.C) {
	cons := constraints.Value{}
	placement := ""
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAPI(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
	}

	cons := constraints.Value{}
	placement := ""
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 0)
}

func (s *environPolSuite) TestPrecheckInstanceFullAPI(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp, "", ""),
	}

	cons := constraints.MustParse("instance-type=n1-standard-1 arch=amd64 root-disk=1G")
	placement := "zone=home-zone"
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, gc.Equals, "home")
}

func (s *environPolSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-1")
	placement := ""
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-1.invalid")
	placement := ""
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, gc.ErrorMatches, `.*invalid GCE instance type.*`)
}

func (s *environPolSuite) TestPrecheckInstanceDiskSize(c *gc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-1 root-disk=1G")
	placement := ""
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	cons := constraints.MustParse("instance-type=n1-standard-1 arch=i386")
	placement := ""
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
	}

	cons := constraints.Value{}
	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneUnavailable(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusDown, "", ""),
	}

	cons := constraints.Value{}
	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, gc.ErrorMatches, `.*availability zone "a-zone" is DOWN`)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp, "", ""),
	}

	cons := constraints.Value{}
	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(testing.FakeDefaultSeries, cons, placement)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *environPolSuite) TestSupportedArchitectures(c *gc.C) {
	s.FakeCommon.Arches = []string{arch.AMD64}

	archList, err := s.Env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(archList, jc.SameContents, []string{arch.AMD64})
}

func (s *environPolSuite) TestConstraintsValidator(c *gc.C) {
	s.FakeCommon.Arches = []string{arch.AMD64}

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorEmpty(c *gc.C) {
	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	s.FakeCommon.Arches = []string{arch.AMD64}

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, jc.DeepEquals, []string{"tags"})
}

func (s *environPolSuite) TestConstraintsValidatorVocabArch(c *gc.C) {
	s.FakeCommon.Arches = []string{arch.AMD64}

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorVocabInstType(c *gc.C) {
	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorVocabContainer(c *gc.C) {
	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("container=lxc")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: container=lxc\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorConflicts(c *gc.C) {
	s.FakeCommon.Arches = []string{arch.AMD64}

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=n1-standard-1")
	// We do not check arch or container since there is only one valid
	// value for each and will always match.
	consFallback := constraints.MustParse("cpu-cores=2 cpu-power=1000 mem=10000 tags=bar")
	merged, err := validator.Merge(consFallback, cons)
	c.Assert(err, jc.ErrorIsNil)

	// tags is not supported, but we're not validating here...
	expected := constraints.MustParse("instance-type=n1-standard-1 tags=bar")
	c.Check(merged, jc.DeepEquals, expected)
}

func (s *environPolSuite) TestSupportNetworks(c *gc.C) {
	isSupported := s.Env.SupportNetworks()

	c.Check(isSupported, jc.IsFalse)
}

func (s *environPolSuite) TestSupportAddressAllocation(c *gc.C) {
	isSupported, err := s.Env.SupportAddressAllocation("some-network")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(isSupported, jc.IsFalse)
}

func (s *environPolSuite) TestSupportAddressAllocationEmpty(c *gc.C) {
	isSupported, err := s.Env.SupportAddressAllocation("")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(isSupported, jc.IsFalse)
}
