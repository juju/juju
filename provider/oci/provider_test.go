// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	ocitesting "github.com/juju/juju/provider/oci/testing"
	jujutesting "github.com/juju/juju/testing"

	"github.com/juju/juju/provider/oci"
)

type credentialsSuite struct {
	testing.FakeHomeSuite

	provider environs.EnvironProvider
	spec     environs.CloudSpec
}

var _ = gc.Suite(&credentialsSuite{})

var singleSectionTemplate = `[%s]
user=fake
fingerprint=%s
key_file=%s
tenancy=fake
region=%s
pass_phrase=%s
`

func newConfig(c *gc.C, attrs jujutesting.Attrs) *config.Config {
	attrs = jujutesting.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	return cfg
}

func fakeCloudSpec() environs.CloudSpec {
	cred := fakeCredential()
	return environs.CloudSpec{
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
		"region":      "us-ashburn-1",
	})
}

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)

	s.provider = &oci.EnvironProvider{}
	s.spec = fakeCloudSpec()
}

func (s *credentialsSuite) writeOCIConfig(c *gc.C, sections map[string]map[string]string) {
	home := utils.Home()
	sectionList := []string{}
	for k, v := range sections {
		pem_name := fmt.Sprintf(".oci/oci_api_key_%s.pem", k)
		pem := filepath.Join(home, pem_name)
		s.Home.AddFiles(c, testing.TestFile{
			Name: pem_name,
			Data: v["key"],
		})
		cfg := fmt.Sprintf(
			singleSectionTemplate, k, v["fingerprint"],
			pem, v["region"], v["pass-phrase"])
		sectionList = append(sectionList, cfg)
	}
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".oci/config",
		Data: strings.Join(sectionList, "\n")})
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "httpsig")
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "httpsig", map[string]string{
		"user":        "fake",
		"tenancy":     "fake",
		"key":         ocitesting.PrivateKeyEncrypted,
		"pass-phrase": ocitesting.PrivateKeyPassphrase,
		"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
		"region":      "us-phoenix-1",
	})
}

func (s *credentialsSuite) TestPassphraseHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "httpsig", "pass-phrase")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}
	creds, err := s.provider.DetectCredentials()
	c.Assert(err, gc.IsNil)
	c.Assert(creds, gc.NotNil)
	c.Assert(*creds, jc.DeepEquals, result)
}

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	cfg := map[string]map[string]string{
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": ocitesting.PrivateKeyPassphrase,
			"key":         ocitesting.PrivateKeyEncrypted,
			"region":      "us-phoenix-1",
		},
	}
	s.writeOCIConfig(c, cfg)
	creds, err := s.provider.DetectCredentials()
	c.Assert(err, gc.IsNil)
	c.Assert(len(creds.AuthCredentials), gc.Equals, 1)
	c.Assert(creds.DefaultRegion, gc.Equals, "us-phoenix-1")
}

func (s *credentialsSuite) TestDetectCredentialsWrongPassphrase(c *gc.C) {
	cfg := map[string]map[string]string{
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": "bogus",
			"key":         ocitesting.PrivateKeyEncrypted,
			"region":      "us-phoenix-1",
		},
	}
	s.writeOCIConfig(c, cfg)
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *credentialsSuite) TestDetectCredentialsMultiSection(c *gc.C) {
	cfg := map[string]map[string]string{
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": ocitesting.PrivateKeyPassphrase,
			"key":         ocitesting.PrivateKeyEncrypted,
			"region":      "us-ashburn-1",
		},
		"SECONDARY": {
			"fingerprint": ocitesting.PrivateKeyUnencryptedFingerprint,
			"pass-phrase": "",
			"key":         ocitesting.PrivateKeyUnencrypted,
			"region":      "us-phoenix-1",
		},
	}
	s.writeOCIConfig(c, cfg)
	creds, err := s.provider.DetectCredentials()
	c.Assert(err, gc.IsNil)
	c.Assert(len(creds.AuthCredentials), gc.Equals, 2)
	c.Assert(creds.DefaultRegion, gc.Equals, "us-ashburn-1")
}

func (s *credentialsSuite) TestDetectCredentialsMultiSectionInvalidConfig(c *gc.C) {
	cfg := map[string]map[string]string{
		// The default section is invalid, due to incorrect password
		// This section should be skipped by DetectCredentials()
		"DEFAULT": {
			"fingerprint": ocitesting.PrivateKeyEncryptedFingerprint,
			"pass-phrase": "bogus",
			"key":         ocitesting.PrivateKeyEncrypted,
			"region":      "us-ashburn-1",
		},
		"SECONDARY": {
			"fingerprint": ocitesting.PrivateKeyUnencryptedFingerprint,
			"pass-phrase": "",
			"key":         ocitesting.PrivateKeyUnencrypted,
			"region":      "us-phoenix-1",
		},
	}
	s.writeOCIConfig(c, cfg)
	creds, err := s.provider.DetectCredentials()
	c.Assert(err, gc.IsNil)
	c.Assert(len(creds.AuthCredentials), gc.Equals, 1)
	c.Assert(creds.DefaultRegion, gc.Equals, "")
}

func (s *credentialsSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: newConfig(c, jujutesting.Attrs{"compartment-id": "fake"}),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)

	env, err = environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: newConfig(c, nil),
	})
	c.Check(err, gc.ErrorMatches, "compartment-id may not be empty")
	c.Assert(env, gc.IsNil)
}
