// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/arch"
)

var _ environs.Environ = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)
var _ simplestreams.MetadataValidator = (*environ)(nil)

type environSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestBase(c *gc.C) {
	s.PatchValue(&newClient, func(*environConfig) (*environClient, error) {
		return nil, nil
	})

	baseConfig := newConfig(c, validAttrs().Merge(testing.Attrs{"name": "testname"}))
	env, err := environs.New(baseConfig)
	c.Assert(err, gc.IsNil)
	env.(*environ).supportedArchitectures = []string{arch.AMD64}

	cfg := env.Config()
	c.Assert(cfg, gc.NotNil)
	c.Check(cfg.Name(), gc.Equals, "testname")

	c.Check(env.PrecheckInstance("", constraints.Value{}, ""), gc.IsNil)

	hasRegion, ok := env.(simplestreams.HasRegion)
	c.Check(ok, gc.Equals, true)
	c.Assert(hasRegion, gc.NotNil)

	cloudSpec, err := hasRegion.Region()
	c.Check(err, gc.IsNil)
	c.Check(cloudSpec.Region, gc.Not(gc.Equals), "")
	c.Check(cloudSpec.Endpoint, gc.Not(gc.Equals), "")

	archs, err := env.SupportedArchitectures()
	c.Check(err, gc.IsNil)
	c.Assert(archs, gc.NotNil)
	c.Assert(archs, gc.HasLen, 1)
	c.Check(archs[0], gc.Equals, arch.AMD64)

	validator, err := env.ConstraintsValidator()
	c.Check(validator, gc.NotNil)
	c.Check(err, gc.IsNil)

	c.Check(env.SupportsUnitPlacement(), gc.ErrorMatches, "SupportsUnitPlacement not implemented")

	c.Check(env.OpenPorts(nil), gc.IsNil)
	c.Check(env.ClosePorts(nil), gc.IsNil)

	ports, err := env.Ports()
	c.Check(ports, gc.IsNil)
	c.Check(err, gc.IsNil)
}

func (s *environSuite) TestUnsupportedConstraints(c *gc.C) {
	s.PatchValue(&newClient, func(*environConfig) (*environClient, error) {
		return nil, nil
	})

	baseConfig := newConfig(c, validAttrs().Merge(testing.Attrs{"name": "testname"}))
	env, err := environs.New(baseConfig)
	c.Assert(err, gc.IsNil)
	env.(*environ).supportedArchitectures = []string{arch.AMD64}

	validator, err := env.ConstraintsValidator()
	c.Check(validator, gc.NotNil)
	c.Check(err, gc.IsNil)

	unsupported, err := validator.Validate(constraints.MustParse(
		"arch=amd64 tags=foo cpu-power=100 virt-type=kvm",
	))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}
