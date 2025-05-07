// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/testhelpers"
)

type credentialsSuite struct {
	testhelpers.IsolationSuite
	provider environs.EnvironProvider
}

var _ = tc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "oauth2", "jsonfile")
}

var sampleCredentialAttributes = map[string]string{
	"GCE_CLIENT_ID":    "123",
	"GCE_CLIENT_EMAIL": "test@example.com",
	"GCE_PROJECT_ID":   "fourfivesix",
	"GCE_PRIVATE_KEY":  "sewen",
}

func (s *credentialsSuite) TestOAuth2CredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "oauth2", map[string]string{
		"client-id":    "123",
		"client-email": "test@example.com",
		"project-id":   "fourfivesix",
		"private-key":  "sewen",
	})
}

func (s *credentialsSuite) TestOAuth2HiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth2", "private-key")
}

func (s *credentialsSuite) TestJSONFileCredentialsValid(c *tc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "somefile")
	err := os.WriteFile(filename, []byte("contents"), 0600)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.AssertProviderCredentialsValid(c, s.provider, "jsonfile", map[string]string{
		// For now at least, the contents of the file are not validated
		// by the credentials schema. That is left to the provider.
		// The file does need to be an absolute path though and exist.
		"file": filename,
	})
}

func createCredsFile(c *tc.C, path string) string {
	if path == "" {
		dir := c.MkDir()
		path = filepath.Join(dir, "creds.json")
	}
	creds, err := google.NewCredentials(sampleCredentialAttributes)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(path, creds.JSONKey, 0644)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

func (s *credentialsSuite) TestDetectCredentialsFromEnvVar(c *tc.C) {
	jsonpath := createCredsFile(c, "")
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("GOOGLE_APPLICATION_CREDENTIALS", jsonpath)
	s.PatchEnvironment("CLOUDSDK_COMPUTE_REGION", "region")
	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "region")
	expected := cloud.NewCredential(cloud.JSONFileAuthType, map[string]string{"file": jsonpath})
	expected.Label = `google credential "test@example.com"`
	c.Assert(credentials.AuthCredentials["fred"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) assertDetectCredentialsKnownLocation(c *tc.C, jsonpath string) {
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("CLOUDSDK_COMPUTE_REGION", "region")
	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, tc.Equals, "region")
	expected := cloud.NewCredential(cloud.JSONFileAuthType, map[string]string{"file": jsonpath})
	expected.Label = `google credential "test@example.com"`
	c.Assert(credentials.AuthCredentials["fred"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsKnownLocationUnix(c *tc.C) {
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		err := utils.SetHome(home)
		c.Assert(err, tc.ErrorIsNil)
	})
	path := filepath.Join(dir, ".config", "gcloud")
	err = os.MkdirAll(path, 0700)
	c.Assert(err, tc.ErrorIsNil)
	jsonpath := createCredsFile(c, filepath.Join(path, "application_default_credentials.json"))
	s.assertDetectCredentialsKnownLocation(c, jsonpath)
}
