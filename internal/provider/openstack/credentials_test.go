// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type credentialsSuite struct {
	testhelpers.IsolationSuite
	provider environs.EnvironProvider
}

func TestCredentialsSuite(t *testing.T) {
	tc.Run(t, &credentialsSuite{})
}

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("openstack")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "access-key", "userpass")
}

func (s *credentialsSuite) TestAccessKeyCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "access-key", map[string]string{
		"access-key":  "key",
		"secret-key":  "secret",
		"tenant-name": "gary",
	})
}

func (s *credentialsSuite) TestAccessKeyHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "access-key", "secret-key")
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "userpass", map[string]string{
		"username":    "bob",
		"password":    "dobbs",
		"tenant-name": "gary",
	})
}

func (s *credentialsSuite) TestUserPassHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "userpass", "password")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *tc.C) {
	// No environment variables set, so no credentials should be found.
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *credentialsSuite) TestDetectCredentialsAccessKeyEnvironmentVariables(c *tc.C) {
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("OS_AUTH_VERSION", "2")
	s.PatchEnvironment("OS_TENANT_NAME", "gary")
	s.PatchEnvironment("OS_TENANT_ID", "abcd123")
	s.PatchEnvironment("OS_ACCESS_KEY", "key-id")
	s.PatchEnvironment("OS_SECRET_KEY", "secret-access-key")
	s.PatchEnvironment("OS_REGION_NAME", "east")

	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "east")
	expected := cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"version":     "2",
			"access-key":  "key-id",
			"secret-key":  "secret-access-key",
			"tenant-name": "gary",
			"tenant-id":   "abcd123",
		},
	)
	expected.Label = `openstack region "east" project "gary" user "fred"`
	c.Assert(credentials.AuthCredentials["fred"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsUserPassEnvironmentVariables(c *tc.C) {
	s.PatchEnvironment("OS_IDENTITY_API_VERSION", "3")
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("OS_PROJECT_NAME", "gary")
	s.PatchEnvironment("OS_PROJECT_ID", "xyz")
	s.PatchEnvironment("OS_USERNAME", "bob")
	s.PatchEnvironment("OS_PASSWORD", "dobbs")
	s.PatchEnvironment("OS_REGION_NAME", "west")
	s.PatchEnvironment("OS_USER_DOMAIN_NAME", "user-domain")

	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "west")
	expected := cloud.NewCredential(
		cloud.UserPassAuthType, map[string]string{
			"version":             "3",
			"username":            "bob",
			"password":            "dobbs",
			"tenant-name":         "gary",
			"tenant-id":           "xyz",
			"domain-name":         "",
			"project-domain-name": "",
			"user-domain-name":    "user-domain",
		},
	)
	expected.Label = `openstack region "west" project "gary" user "bob"`
	c.Assert(credentials.AuthCredentials["bob"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsUserPassDefaultDomain(c *tc.C) {
	s.PatchEnvironment("OS_AUTH_VERSION", "3")
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("OS_PROJECT_NAME", "gary")
	s.PatchEnvironment("OS_USERNAME", "bob")
	s.PatchEnvironment("OS_PASSWORD", "dobbs")
	s.PatchEnvironment("OS_REGION_NAME", "west")
	s.PatchEnvironment("OS_DEFAULT_DOMAIN_NAME", "default-domain")

	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "west")
	expected := cloud.NewCredential(
		cloud.UserPassAuthType, map[string]string{
			"version":             "3",
			"username":            "bob",
			"password":            "dobbs",
			"tenant-name":         "gary",
			"tenant-id":           "",
			"domain-name":         "",
			"project-domain-name": "default-domain",
			"user-domain-name":    "default-domain",
		},
	)
	expected.Label = `openstack region "west" project "gary" user "bob"`
	c.Assert(credentials.AuthCredentials["bob"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsNovarc(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("not running linux")
	}
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		err := utils.SetHome(home)
		c.Assert(err, tc.ErrorIsNil)
	})

	content := `
# Some secrets
export OS_AUTH_VERSION=3
export OS_TENANT_NAME=gary
export OS_TENANT_ID=xyz
EXPORT OS_USERNAME=bob
  export  OS_PASSWORD = dobbs
OS_REGION_NAME=region
OS_PROJECT_DOMAIN_NAME=project-domain
`[1:]
	novarc := filepath.Join(dir, ".novarc")
	err = os.WriteFile(novarc, []byte(content), 0600)
	c.Assert(err, tc.ErrorIsNil)
	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "region")
	expected := cloud.NewCredential(
		cloud.UserPassAuthType, map[string]string{
			"version":             "3",
			"username":            "bob",
			"password":            "dobbs",
			"tenant-name":         "gary",
			"tenant-id":           "xyz",
			"domain-name":         "",
			"project-domain-name": "project-domain",
			"user-domain-name":    "",
		},
	)
	expected.Label = `openstack region "region" project "gary" user "bob"`
	c.Assert(credentials.AuthCredentials["bob"], tc.DeepEquals, expected)
}
