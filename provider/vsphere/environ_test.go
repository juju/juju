// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	EnvironFixture
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.PatchValue(&vsphere.Bootstrap, func(
		ctx environs.BootstrapContext,
		env environs.Environ,
		args environs.BootstrapParams,
	) (*environs.BootstrapResult, error) {
		return nil, errors.New("Bootstrap called")
	})

	_, err := s.env.Bootstrap(nil, environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	})
	c.Assert(err, gc.ErrorMatches, "Bootstrap called")

	s.dialStub.CheckNoCalls(c)
}

func (s *environSuite) TestDestroy(c *gc.C) {
	var destroyCalled bool
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ) error {
		destroyCalled = true
		s.client.CheckNoCalls(c)
		return nil
	})
	err := s.env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(destroyCalled, jc.IsTrue)
	s.client.CheckCallNames(c, "Close")
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	var destroyCalled bool
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ) error {
		destroyCalled = true
		s.client.CheckNoCalls(c)
		return nil
	})
	err := s.env.DestroyController("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(destroyCalled, jc.IsTrue)
	s.client.CheckCallNames(c, "Close")
}

func (s *environSuite) TestAdoptResources(c *gc.C) {
	err := s.env.AdoptResources("foo", version.Number{})
	c.Assert(err, jc.ErrorIsNil)

	s.dialStub.CheckNoCalls(c)
}

func (s *environSuite) TestPrepareForBootstrap(c *gc.C) {
	err := s.env.PrepareForBootstrap(envtesting.BootstrapContext(c))
	c.Check(err, jc.ErrorIsNil)
}

func (s *environSuite) TestSupportsNetworking(c *gc.C) {
	_, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, jc.IsFalse)
}
