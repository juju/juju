// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/provider/lxd"
)

type environPolSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environPolSuite{})

func (s *environPolSuite) TestPrecheckInstanceOkay(c *gc.C) {
	cons := constraints.Value{}
	placement := ""
	err := s.Env.PrecheckInstance(series.LatestLts(), cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAPI(c *gc.C) {
	cons := constraints.Value{}
	placement := ""
	err := s.Env.PrecheckInstance(series.LatestLts(), cons, placement)
	c.Assert(err, jc.ErrorIsNil)

	s.CheckNoAPI(c)
}

func (s *environPolSuite) TestPrecheckInstanceHasInstanceType(c *gc.C) {
	cons := constraints.MustParse("instance-type=some-instance-type")
	placement := ""
	err := s.Env.PrecheckInstance(series.LatestLts(), cons, placement)

	c.Check(err, gc.ErrorMatches, `LXD does not support instance types.*`)
}

func (s *environPolSuite) TestPrecheckInstanceDiskSize(c *gc.C) {
	cons := constraints.MustParse("root-disk=1G")
	placement := ""
	err := s.Env.PrecheckInstance(series.LatestLts(), cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	cons := constraints.MustParse("arch=i386")
	placement := ""
	err := s.Env.PrecheckInstance(series.LatestLts(), cons, placement)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	cons := constraints.Value{}
	placement := "zone=a-zone"
	err := s.Env.PrecheckInstance(series.LatestLts(), cons, placement)

	c.Check(err, gc.ErrorMatches, `unknown placement directive: .*`)
}

func (s *environPolSuite) TestConstraintsValidatorOkay(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

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
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse(strings.Join([]string{
		"arch=amd64",
		"tags=foo",
		"mem=3",
		"instance-type=some-type",
		"cores=2",
		"cpu-power=250",
		"virt-type=kvm",
	}, " "))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		"tags",
		"instance-type",
		"cores",
		"cpu-power",
		"virt-type",
	}
	c.Check(unsupported, jc.SameContents, expected)
}

func (s *environPolSuite) TestConstraintsValidatorVocabArchKnown(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	_, err = validator.Validate(cons)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environPolSuite) TestConstraintsValidatorVocabArchUnknown(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: \\[amd64\\]")
}

func (s *environPolSuite) TestConstraintsValidatorVocabContainerUnknown(c *gc.C) {
	c.Skip("this will fail until we add a container vocabulary")
	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("container=lxd")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: container=lxd\nvalid values are:.*")
}

func (s *environPolSuite) TestConstraintsValidatorConflicts(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("instance-type=n1-standard-1")
	consFallback := constraints.MustParse("cores=2 cpu-power=1000 mem=10000 tags=bar")
	merged, err := validator.Merge(consFallback, cons)
	c.Assert(err, jc.ErrorIsNil)

	// tags is not supported, but we're not validating here...
	expected := constraints.MustParse("instance-type=n1-standard-1 tags=bar cores=2 cpu-power=1000 mem=10000")
	c.Check(merged, jc.DeepEquals, expected)
}

func (s *environPolSuite) TestSupportNetworks(c *gc.C) {
	isSupported := s.Env.SupportNetworks()

	c.Check(isSupported, jc.IsFalse)
}
