// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce"
)

type providerSuite struct {
	gce.BaseSuite

	provider environs.EnvironProvider
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Check(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	c.Assert(s.provider, gc.Equals, gce.Provider)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud:  makeTestCloudSpec(),
		Config: s.Config,
	})
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testenv")
}

func (s *providerSuite) TestBootstrapConfig(c *gc.C) {
	cfg, err := s.provider.BootstrapConfig(environs.BootstrapConfigParams{
		Config: s.Config,
		Cloud:  makeTestCloudSpec(),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.provider.Validate(s.Config, nil)
	c.Check(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestSecretAttrs(c *gc.C) {
	obtainedAttrs, err := s.provider.SecretAttrs(s.Config)
	c.Check(err, jc.ErrorIsNil)

	expectedAttrs := map[string]string{"private-key": gce.PrivateKey}
	c.Assert(obtainedAttrs, gc.DeepEquals, expectedAttrs)

}

func (s *providerSuite) TestUpgradeConfig(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ModelConfigUpgrader))
	upgrader := s.provider.(environs.ModelConfigUpgrader)

	_, ok := s.Config.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsFalse)

	outConfig, err := upgrader.UpgradeConfig(s.Config)
	c.Assert(err, jc.ErrorIsNil)
	source, ok := outConfig.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(source, gc.Equals, "gce")
}
