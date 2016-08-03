// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/testing"
)

type ProviderSuite struct {
	testing.IsolationSuite
	spec     environs.CloudSpec
	provider environs.EnvironProvider
}

var _ = gc.Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "foo",
			"secret-key": "bar",
		},
	)
	s.spec = environs.CloudSpec{
		Type:       "ec2",
		Name:       "aws",
		Region:     "us-east-1",
		Credential: &credential,
	}

	provider, err := environs.Provider("ec2")
	c.Assert(err, jc.ErrorIsNil)
	s.provider = provider
}

func (s *ProviderSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud:  s.spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *ProviderSuite) TestOpenInvalidRegion(c *gc.C) {
	s.spec.Region = "foobar"
	s.testOpenError(c, s.spec, `validating cloud spec: region name "foobar" not valid`)
}

func (s *ProviderSuite) TestOpenMissingCredential(c *gc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *ProviderSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "userpass" auth-type not supported`)
}

func (s *ProviderSuite) testOpenError(c *gc.C, spec environs.CloudSpec, expect string) {
	_, err := s.provider.Open(environs.OpenParams{
		Cloud:  spec,
		Config: coretesting.ModelConfig(c),
	})
	c.Assert(err, gc.ErrorMatches, expect)
}
