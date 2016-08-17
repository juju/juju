// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
)

type credentialsSuite struct {
	testing.FakeHomeSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("maas")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "oauth1")
}

func (s *credentialsSuite) TestOAuth1CredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "oauth1", map[string]string{
		"maas-oauth": "123:456:789",
	})
}

func (s *credentialsSuite) TestOAuth1HiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth1", "maas-oauth")
}

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".maasrc",
		Data: `{"Server": "http://10.0.0.1/MAAS", "OAuth": "key"}`,
	})
	creds, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds.DefaultRegion, gc.Equals, "")
	expected := cloud.NewCredential(
		cloud.OAuth1AuthType, map[string]string{
			"maas-oauth": "key",
		},
	)
	expected.Label = "MAAS credential for http://10.0.0.1/MAAS"
	c.Assert(creds.AuthCredentials["default"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsNoServer(c *gc.C) {
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".maasrc",
		Data: `{"OAuth": "key"}`,
	})
	creds, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds.DefaultRegion, gc.Equals, "")
	expected := cloud.NewCredential(
		cloud.OAuth1AuthType, map[string]string{
			"maas-oauth": "key",
		},
	)
	expected.Label = "MAAS credential for unspecified server"
	c.Assert(creds.AuthCredentials["default"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsNoFile(c *gc.C) {
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
