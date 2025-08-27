// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	stdcontext "context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.BaseSuite

	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
	config   *config.Config
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Check(err, jc.ErrorIsNil)

	s.spec = gce.MakeTestCloudSpec()

	uuid := utils.MustNewUUID().String()
	s.config = testing.CustomModelConfig(c, testing.Attrs{
		"uuid": uuid,
		"type": "gce",
	})
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	c.Assert(s.provider, gc.Equals, gce.Provider)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: s.config,
	})
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testmodel")
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *gc.C) {
	s.spec.Name = ""
	s.testOpenError(c, s.spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *gc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: s.config,
	})
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *providerSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Config: s.config,
		Cloud:  gce.MakeTestCloudSpec(),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.provider.Validate(s.config, nil)
	c.Check(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(s.config.AllAttrs(), gc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestUpgradeConfig(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ModelConfigUpgrader))
	upgrader := s.provider.(environs.ModelConfigUpgrader)

	_, ok := s.config.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsFalse)

	outConfig, err := upgrader.UpgradeConfig(s.config)
	c.Assert(err, jc.ErrorIsNil)
	source, ok := outConfig.StorageDefaultBlockSource()
	c.Assert(ok, jc.IsTrue)
	c.Assert(source, gc.Equals, "gce")
}
