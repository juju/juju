// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/vsphere"
)

type providerSuite struct {
	vsphere.BaseSuite

	provider environs.EnvironProvider
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("vsphere")
	c.Check(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	c.Assert(s.provider, gc.Equals, vsphere.Provider)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud:  vsphere.FakeCloudSpec(),
		Config: s.Config,
	})
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testenv")
}

func (s *providerSuite) TestBootstrapConfig(c *gc.C) {
	credential := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"user": "u", "password": "p"},
	)
	cfg, err := s.provider.BootstrapConfig(environs.BootstrapConfigParams{
		Config: s.Config,
		Cloud: environs.CloudSpec{
			Credential: &credential,
		},
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

	expectedAttrs := map[string]string{"password": "password1"}
	c.Assert(obtainedAttrs, gc.DeepEquals, expectedAttrs)

}
