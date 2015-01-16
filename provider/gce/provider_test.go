// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
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
	env, err := s.provider.Open(s.Config)
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testenv")
}

func (s *providerSuite) TestPrepare(c *gc.C) {
	env, err := s.provider.Prepare(envtesting.BootstrapContext(c), s.Config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(env, gc.NotNil)
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

	expectedAttrs := map[string]string{"private-key": "seekrit"}
	c.Assert(obtainedAttrs, gc.DeepEquals, expectedAttrs)

}

func (s *providerSuite) TestBoilerplateConfig(c *gc.C) {
	// (wwitzel3) purposefully duplicate here so that this test will
	// fail if someone updates gce/config.go without updating this test.
	var boilerplateConfig = `
gce:
  type: gce

  # Google Auth Info
  private-key: 
  client-email:
  client-id:

  # Google instance info
  # region: us-central1
  project-id:
  # image-endpoint: https://www.googleapis.com
`[1:]
	c.Assert(s.provider.BoilerplateConfig(), gc.Equals, boilerplateConfig)
}
