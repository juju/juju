// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/ecs"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

func fakeConfig(c *gc.C, attrs ...coretesting.Attrs) *config.Config {
	cfg, err := coretesting.ModelConfig(c).Apply(fakeConfigAttrs(attrs...))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func fakeConfigAttrs(attrs ...coretesting.Attrs) coretesting.Attrs {
	merged := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type": "ecs",
		"uuid": utils.MustNewUUID().String(),
	})
	for _, attrs := range attrs {
		merged = merged.Merge(attrs)
	}
	return merged
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	cred := fakeCredential()
	return environscloudspec.CloudSpec{
		Type:       "ecs",
		Name:       "ecs1",
		Endpoint:   "https://ec2.ap-southeast-2.amazonaws.com",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"access-key":   "access-key",
		"secret-key":   "secret-key",
		"cluster-name": "cluster-name",
		"region":       "ap-southeast-2",
	})
}

type providerSuite struct {
	testing.IsolationSuite
	dialStub testing.Stub
	provider caas.ContainerEnvironProvider
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dialStub.ResetCalls()
	s.provider = ecs.NewProvider()
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	provider, err := environs.Provider("ecs")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider, gc.NotNil)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	config := fakeConfig(c)
	broker, err := s.provider.Open(environs.OpenParams{
		ControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		Cloud:          fakeCloudSpec(),
		Config:         config,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(broker, gc.NotNil)
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *gc.C) {
	spec := fakeCloudSpec()
	spec.Name = ""
	s.testOpenError(c, spec, `cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *gc.C) {
	spec := fakeCloudSpec()
	spec.Credential = nil
	s.testOpenError(c, spec, `missing credential not valid`)
}

func (s *providerSuite) TestOpenMissingControllerUUID(c *gc.C) {
	spec := fakeCloudSpec()
	s.testOpenError(c, spec, `controllerUUID is required not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	spec := fakeCloudSpec()
	spec.Credential = &credential
	s.testOpenError(c, spec, `"oauth1" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := s.provider.Open(environs.OpenParams{
		Cloud:  spec,
		Config: fakeConfig(c),
	})
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *providerSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Config: fakeConfig(c),
		Cloud:  fakeCloudSpec(),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	config := fakeConfig(c)
	validCfg, err := s.provider.Validate(config, nil)
	c.Check(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(config.AllAttrs(), gc.DeepEquals, validAttrs)
}
