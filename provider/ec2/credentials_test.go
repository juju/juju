// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
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
	s.provider, err = environs.Provider("ec2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "access-key")
}

func (s *credentialsSuite) TestAccessKeyCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "access-key", map[string]string{
		"access-key": "key",
		"secret-key": "secret",
	})
}

func (s *credentialsSuite) TestAccessKeyHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "access-key", "secret-key")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	// No environment variables set, so no credentials should be found.
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *credentialsSuite) TestDetectCredentialsEnvironmentVariables(c *gc.C) {
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		err := utils.SetHome(home)
		c.Assert(err, jc.ErrorIsNil)
	})
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "key-id")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret-access-key")

	credentials, err := s.provider.DetectCredentials()
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

func (s *credentialsSuite) assertDetectCredentialsKnownLocation(c *gc.C, dir string) {
	location := filepath.Join(dir, ".aws")
	err := os.MkdirAll(location, 0700)
	c.Assert(err, jc.ErrorIsNil)
	path := filepath.Join(location, "credentials")
	credData := `
[fred]
aws_access_key_id=aws-key-id
aws_secret_access_key=aws-secret-access-key
`[1:]
	err = ioutil.WriteFile(path, []byte(credData), 0600)
	c.Assert(err, jc.ErrorIsNil)

	path = filepath.Join(location, "config")
	regionData := `
[default]
region=region
`[1:]
	err = ioutil.WriteFile(path, []byte(regionData), 0600)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure any env vars are ignored.
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "key-id")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret-access-key")

	credentials, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, gc.Equals, "region")
	expected := cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"access-key": "aws-key-id",
			"secret-key": "aws-secret-access-key",
		},
	)
	expected.Label = `aws credential "fred"`
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsKnownLocationUnix(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("skipping on Windows")
	}
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		err := utils.SetHome(home)
		c.Assert(err, jc.ErrorIsNil)
	})
	s.assertDetectCredentialsKnownLocation(c, dir)
}

func (s *credentialsSuite) TestDetectCredentialsKnownLocationWindows(c *gc.C) {
	if runtime.GOOS != "windows" {
		c.Skip("skipping on non-Windows platform")
	}
	dir := c.MkDir()
	s.PatchEnvironment("USERPROFILE", dir)
	s.assertDetectCredentialsKnownLocation(c, dir)
}
