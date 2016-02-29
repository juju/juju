// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

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
	testing.IsolationSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("openstack")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "access-key", "userpass")
}

func (s *credentialsSuite) TestAccessKeyCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "access-key", map[string]string{
		"access-key":  "key",
		"secret-key":  "secret",
		"tenant-name": "gary",
	})
}

func (s *credentialsSuite) TestAccessKeyHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "access-key", "secret-key")
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "userpass", map[string]string{
		"username":    "bob",
		"password":    "dobbs",
		"tenant-name": "gary",
	})
}

func (s *credentialsSuite) TestUserPassHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "userpass", "password")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	// No environment variables set, so no credentials should be found.
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *credentialsSuite) TestDetectCredentialsAccessKeyEnvironmentVariables(c *gc.C) {
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("OS_TENANT_NAME", "gary")
	s.PatchEnvironment("OS_ACCESS_KEY", "key-id")
	s.PatchEnvironment("OS_SECRET_KEY", "secret-access-key")

	credentials, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"access-key":  "key-id",
			"secret-key":  "secret-access-key",
			"tenant-name": "gary",
		},
	))
}

func (s *credentialsSuite) TestDetectCredentialsUserPassEnvironmentVariables(c *gc.C) {
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("OS_TENANT_NAME", "gary")
	s.PatchEnvironment("OS_USERNAME", "bob")
	s.PatchEnvironment("OS_PASSWORD", "dobbs")

	credentials, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, cloud.NewCredential(
		cloud.UserPassAuthType, map[string]string{
			"username":    "bob",
			"password":    "dobbs",
			"tenant-name": "gary",
		},
	))
}
