// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/gce"
)

type providerSuite struct {
	gce.BaseSuite

	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
}

var _ = tc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Check(err, tc.ErrorIsNil)

	s.spec = gce.MakeTestCloudSpec()
}

func (s *providerSuite) TestRegistered(c *tc.C) {
	c.Assert(s.provider, tc.Equals, gce.Provider)
}

func (s *providerSuite) TestOpen(c *tc.C) {
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: s.Config,
	}, environs.NoopCredentialInvalidator())
	c.Check(err, tc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), tc.Equals, "testmodel")
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *tc.C) {
	s.spec.Name = ""
	s.testOpenError(c, s.spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *tc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *tc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *tc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: s.Config,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *providerSuite) TestValidateCloud(c *tc.C) {
	err := s.provider.ValidateCloud(context.Background(), gce.MakeTestCloudSpec())
	c.Check(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestValidate(c *tc.C) {
	validCfg, err := s.provider.Validate(context.Background(), s.Config, nil)
	c.Check(err, tc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(s.Config.AllAttrs(), tc.DeepEquals, validAttrs)
}
