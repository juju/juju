// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type credentialsSuite struct {
	testhelpers.FakeHomeSuite
	provider environs.EnvironProvider
}

func TestCredentialsSuite(t *stdtesting.T) {
	tc.Run(t, &credentialsSuite{})
}

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.FakeHomeSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("maas")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "oauth1")
}

func (s *credentialsSuite) TestOAuth1CredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "oauth1", map[string]string{
		"maas-oauth": "123:456:789",
	})
}

func (s *credentialsSuite) TestOAuth1HiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth1", "maas-oauth")
}

func (s *credentialsSuite) TestDetectCredentials(c *tc.C) {
	s.Home.AddFiles(c, testhelpers.TestFile{
		Name: ".maasrc",
		Data: `{"Server": "http://10.0.0.1/MAAS", "OAuth": "key"}`,
	})
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(creds.DefaultRegion, tc.Equals, "")
	expected := cloud.NewCredential(
		cloud.OAuth1AuthType, map[string]string{
			"maas-oauth": "key",
		},
	)
	expected.Label = "MAAS credential for http://10.0.0.1/MAAS"
	c.Assert(creds.AuthCredentials["default"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsNoServer(c *tc.C) {
	s.Home.AddFiles(c, testhelpers.TestFile{
		Name: ".maasrc",
		Data: `{"OAuth": "key"}`,
	})
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(creds.DefaultRegion, tc.Equals, "")
	expected := cloud.NewCredential(
		cloud.OAuth1AuthType, map[string]string{
			"maas-oauth": "key",
		},
	)
	expected.Label = "MAAS credential for unspecified server"
	c.Assert(creds.AuthCredentials["default"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsNoFile(c *tc.C) {
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
