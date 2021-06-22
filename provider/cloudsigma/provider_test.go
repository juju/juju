// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	stdcontext "context"
	stdtesting "testing"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

func TestCloudSigma(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testing.IsolationSuite

	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	provider, err := environs.Provider("cloudsigma")
	c.Assert(err, jc.ErrorIsNil)
	s.provider = provider
	s.spec = fakeCloudSpec()
}

func (s *providerSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: newConfig(c, nil),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
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
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "oauth1" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(stdcontext.TODO(), s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: newConfig(c, nil),
	})
	c.Assert(err, gc.ErrorMatches, expect)
}
