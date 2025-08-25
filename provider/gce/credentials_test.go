// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/gce/internal/google"
)

type credentialsSuite struct {
	testing.IsolationSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "oauth2", "jsonfile")
}

var sampleCredentialAttributes = map[string]string{
	"GCE_CLIENT_ID":    "123",
	"GCE_CLIENT_EMAIL": "test@example.com",
	"GCE_PROJECT_ID":   "fourfivesix",
	"GCE_PRIVATE_KEY":  "sewen",
}

func (s *credentialsSuite) TestOAuth2CredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "oauth2", map[string]string{
		"client-id":    "123",
		"client-email": "test@example.com",
		"project-id":   "fourfivesix",
		"private-key":  "sewen",
	})
}

func (s *credentialsSuite) TestOAuth2HiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth2", "private-key")
}

func (s *credentialsSuite) TestJSONFileCredentialsValid(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "somefile")
	err := os.WriteFile(filename, []byte("contents"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.AssertProviderCredentialsValid(c, s.provider, "jsonfile", map[string]string{
		// For now at least, the contents of the file are not validated
		// by the credentials schema. That is left to the provider.
		// The file does need to be an absolute path though and exist.
		"file": filename,
	})
}

func createCredsFile(c *gc.C, path string) string {
	if path == "" {
		dir := c.MkDir()
		path = filepath.Join(dir, "creds.json")
	}
	creds, err := google.NewCredentials(sampleCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(path, creds.JSONKey, 0644)
	c.Assert(err, jc.ErrorIsNil)
	return path
}

func (s *credentialsSuite) TestDetectCredentialsFromEnvVar(c *gc.C) {
	jsonpath := createCredsFile(c, "")
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("GOOGLE_APPLICATION_CREDENTIALS", jsonpath)
	s.PatchEnvironment("CLOUDSDK_COMPUTE_REGION", "region")
	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, gc.Equals, "region")
	expected := cloud.NewCredential(cloud.JSONFileAuthType, map[string]string{"file": jsonpath})
	expected.Label = `google credential "test@example.com"`
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) assertDetectCredentialsKnownLocation(c *gc.C, jsonpath string) {
	s.PatchEnvironment("USER", "fred")
	s.PatchEnvironment("CLOUDSDK_COMPUTE_REGION", "region")
	credentials, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials.DefaultRegion, gc.Equals, "region")
	expected := cloud.NewCredential(cloud.JSONFileAuthType, map[string]string{"file": jsonpath})
	expected.Label = `google credential "test@example.com"`
	c.Assert(credentials.AuthCredentials["fred"], jc.DeepEquals, expected)
}

func (s *credentialsSuite) TestDetectCredentialsKnownLocationUnix(c *gc.C) {
	home := utils.Home()
	dir := c.MkDir()
	err := utils.SetHome(dir)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		err := utils.SetHome(home)
		c.Assert(err, jc.ErrorIsNil)
	})
	path := filepath.Join(dir, ".config", "gcloud")
	err = os.MkdirAll(path, 0700)
	c.Assert(err, jc.ErrorIsNil)
	jsonpath := createCredsFile(c, filepath.Join(path, "application_default_credentials.json"))
	s.assertDetectCredentialsKnownLocation(c, jsonpath)
}
