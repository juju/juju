// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/vsphere"
)

type environPolSuite struct {
	vsphere.BaseSuite
}

var _ = gc.Suite(&environPolSuite{})

func (s *environPolSuite) TestSupportedArchitectures(c *gc.C) {
	archList, err := s.Env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(archList, jc.SameContents, []string{arch.AMD64})
}

func (s *environPolSuite) TestConstraintsValidator(c *gc.C) {
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
	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, jc.DeepEquals, []string{"tags"})
}

func (s *environPolSuite) TestConstraintsValidatorVocabArch(c *gc.C) {
	validator, err := s.Env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)

	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
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
