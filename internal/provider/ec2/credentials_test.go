// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
)

type credentialsSuite struct {
	testing.IsolationSuite
	provider environs.EnvironProvider
}

var _ = tc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("ec2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "access-key", "instance-role")
}

func (s *credentialsSuite) TestAccessKeyCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "access-key", map[string]string{
		"access-key": "key",
		"secret-key": "secret",
	})
}

func (s *credentialsSuite) TestAccessKeyHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "access-key", "secret-key")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *tc.C) {
	// No environment variables set, so no credentials should be found.
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *credentialsSuite) TestDetectCredentialsEnvironmentVariables(c *tc.C) {
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		err := utils.SetHome(home)
		c.Assert(err, jc.ErrorIsNil)
	})
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "key-id")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret-access-key")

	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	expected := cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"access-key": "key-id",
			"secret-key": "secret-access-key",
		},
	)
	expected.Label = `aws credential "fred"`
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) assertDetectCredentialsKnownLocation(c *tc.C, dir string) {
	location := filepath.Join(dir, ".aws")
	err := os.MkdirAll(location, 0700)
	c.Assert(err, jc.ErrorIsNil)
	path := filepath.Join(location, "credentials")
	credData := `
[fred]
aws_access_key_id=aws-key-id
aws_secret_access_key=aws-secret-access-key
`[1:]
	err = os.WriteFile(path, []byte(credData), 0600)
	c.Assert(err, jc.ErrorIsNil)

	path = filepath.Join(location, "config")
	regionData := `
[default]
region=region
`[1:]
	err = os.WriteFile(path, []byte(regionData), 0600)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure any env vars are ignored.
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "key-id")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret-access-key")

	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "region")
	expected := cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"access-key": "aws-key-id",
			"secret-key": "aws-secret-access-key",
		},
	)
	expected.Label = `aws credential "fred"`
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsKnownLocationUnix(c *tc.C) {
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		err := utils.SetHome(home)
		c.Assert(err, jc.ErrorIsNil)
	})
	s.assertDetectCredentialsKnownLocation(c, dir)
}
