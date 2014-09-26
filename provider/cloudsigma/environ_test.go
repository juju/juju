// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

type environSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *environSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) TestBase(c *gc.C) {
	var emptyStorage environStorage

	s.PatchValue(&newClient, func(*environConfig) (*environClient, error) {
		return nil, nil
	})
	s.PatchValue(&newStorage, func(*environConfig, *environClient) (*environStorage, error) {
		return &emptyStorage, nil
	})

	baseConfig := newConfig(c, validAttrs().Merge(testing.Attrs{"name": "testname"}))
	environ, err := environs.New(baseConfig)
	c.Assert(err, gc.IsNil)

	cfg := environ.Config()
	c.Assert(cfg, gc.NotNil)
	c.Check(cfg.Name(), gc.Equals, "testname")

	environstrage, ok := environ.(environs.EnvironStorage)
	c.Check(environstrage.Storage(), gc.Equals, &emptyStorage)

	c.Check(environ.PrecheckInstance("", constraints.Value{}, ""), gc.IsNil)

	hasRegion, ok := environ.(simplestreams.HasRegion)
	c.Check(ok, gc.Equals, true)
	c.Assert(hasRegion, gc.NotNil)

	cloudSpec, err := hasRegion.Region()
	c.Check(err, gc.IsNil)
	c.Check(cloudSpec.Region, gc.Not(gc.Equals), "")
	c.Check(cloudSpec.Endpoint, gc.Not(gc.Equals), "")

	archs, err := environ.SupportedArchitectures()
	c.Check(err, gc.IsNil)
	c.Assert(archs, gc.NotNil)
	c.Assert(archs, gc.HasLen, 1)
	c.Check(archs[0], gc.Equals, arch.AMD64)

	validator, err := environ.ConstraintsValidator()
	c.Check(validator, gc.NotNil)
	c.Check(err, gc.IsNil)

	c.Check(environ.SupportNetworks(), gc.Equals, false)
	c.Check(environ.SupportsUnitPlacement(), gc.IsNil)

	c.Check(environ.OpenPorts(nil), gc.IsNil)
	c.Check(environ.ClosePorts(nil), gc.IsNil)

	ports, err := environ.Ports()
	c.Check(ports, gc.IsNil)
	c.Check(err, gc.IsNil)
}
