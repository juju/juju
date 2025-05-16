// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"fmt"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/oci"
	ocitesting "github.com/juju/juju/internal/provider/oci/testing"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type credentialsSuite struct {
	testhelpers.FakeHomeSuite

	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
}

func TestCredentialsSuite(t *stdtesting.T) { tc.Run(t, &credentialsSuite{}) }

var singleSectionTemplate = `[%s]
user=fake
fingerprint=%s
key_file=%s
tenancy=fake
region=%s
pass_phrase=%s
`

func newConfig(c *tc.C, attrs jujutesting.Attrs) *config.Config {
	attrs = jujutesting.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, tc.IsNil)
	return cfg
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	cred := fakeCredential()
	return environscloudspec.CloudSpec{
		Type:       "oci",
		Name:       "oci",
		Region:     "us-phoenix-1",
		Endpoint:   "",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.HTTPSigAuthType, map[string]string{
		"key":         ocitesting.PrivateKeyEncrypted,
		"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
		"pass-phrase": ocitesting.PrivateKeyPassphrase,
		"tenancy":     "fake",
		"user":        "fake",
	})
}

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.FakeHomeSuite.SetUpTest(c)

	s.provider = &oci.EnvironProvider{}
	s.spec = fakeCloudSpec()
}

func (s *credentialsSuite) writeOCIConfig(c *tc.C, sections map[string]map[string]string) {
	home := utils.Home()
	sectionList := []string{}
	for k, v := range sections {
		pem_name := fmt.Sprintf(".oci/oci_api_key_%s.pem", k)
		pem := filepath.Join(home, pem_name)
		s.Home.AddFiles(c, testhelpers.TestFile{
			Name: pem_name,
			Data: v["key"],
		})
		cfg := fmt.Sprintf(
			singleSectionTemplate, k, v["fingerprint"],
			pem, v["region"], v["pass-phrase"])
		sectionList = append(sectionList, cfg)
	}
	s.Home.AddFiles(c, testhelpers.TestFile{
		Name: ".oci/config",
		Data: strings.Join(sectionList, "\n")})
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "httpsig")
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "httpsig", map[string]string{
		"user":        "fake",
		"tenancy":     "fake",
		"key":         ocitesting.PrivateKeyEncrypted,
		"pass-phrase": ocitesting.PrivateKeyPassphrase,
		"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
		"region":      "us-phoenix-1",
	})
}

func (s *credentialsSuite) TestPassphraseHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "httpsig", "pass-phrase")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *tc.C) {
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.IsNil)
	c.Assert(creds, tc.NotNil)
	c.Assert(*creds, tc.DeepEquals, result)
}

func (s *credentialsSuite) TestDetectCredentials(c *tc.C) {
	cfg := map[string]map[string]string{
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": ocitesting.PrivateKeyPassphrase,
			"key":         ocitesting.PrivateKeyEncrypted,
		},
	}
	s.writeOCIConfig(c, cfg)
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.IsNil)
	c.Assert(len(creds.AuthCredentials), tc.Equals, 1)
}

func (s *credentialsSuite) TestDetectCredentialsWrongPassphrase(c *tc.C) {
	cfg := map[string]map[string]string{
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": "bogus",
			"key":         ocitesting.PrivateKeyEncrypted,
		},
	}
	s.writeOCIConfig(c, cfg)
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *credentialsSuite) TestDetectCredentialsMultiSection(c *tc.C) {
	cfg := map[string]map[string]string{
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": ocitesting.PrivateKeyPassphrase,
			"key":         ocitesting.PrivateKeyEncrypted,
		},
		"SECONDARY": {
			"fingerprint": ocitesting.PrivateKeyUnencryptedFingerprint,
			"pass-phrase": "",
			"key":         ocitesting.PrivateKeyUnencrypted,
		},
	}
	s.writeOCIConfig(c, cfg)
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.IsNil)
	c.Assert(len(creds.AuthCredentials), tc.Equals, 2)
}

func (s *credentialsSuite) TestDetectCredentialsMultiSectionInvalidConfig(c *tc.C) {
	cfg := map[string]map[string]string{
		// The default section is invalid, due to incorrect password
		// This section should be skipped by DetectCredentials()
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": "bogus",
			"key":         ocitesting.PrivateKeyEncrypted,
		},
		"SECONDARY": {
			"fingerprint": ocitesting.PrivateKeyUnencryptedFingerprint,
			"pass-phrase": "",
			"key":         ocitesting.PrivateKeyUnencrypted,
		},
	}
	s.writeOCIConfig(c, cfg)
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.IsNil)
	c.Assert(len(creds.AuthCredentials), tc.Equals, 1)
	c.Assert(creds.DefaultRegion, tc.Equals, "")
}

func (s *credentialsSuite) TestOpen(c *tc.C) {
	env, err := environs.Open(c.Context(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: newConfig(c, jujutesting.Attrs{"compartment-id": "fake"}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env, tc.NotNil)

	env, err = environs.Open(c.Context(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: newConfig(c, nil),
	}, environs.NoopCredentialInvalidator())
	c.Check(err, tc.ErrorMatches, "compartment-id may not be empty")
	c.Assert(env, tc.IsNil)
}
