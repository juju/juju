// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
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
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 1)
	_, ok := schemas["access-key"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected access-key auth-type schema"))
}

var sampleCredentialAttributes = map[string]string{
	"access-key": "key",
	"secret-key": "secret",
}

func (s *credentialsSuite) TestAccessKeyCredentialSchema(c *gc.C) {
	schema := s.accessKeyCredentialSchema(c)

	err := schema.Validate(sampleCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(schema["access-key"].Hidden, jc.IsFalse)
	c.Assert(schema["secret-key"].Hidden, jc.IsTrue)
}

func (s *credentialsSuite) TestAccessKeyCredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.accessKeyCredentialSchema(c)

	// If any one of the attributes is missing, it's an error.
	for excludedKey := range sampleCredentialAttributes {
		attrs := make(map[string]string)
		for key, value := range sampleCredentialAttributes {
			if key != excludedKey {
				attrs[key] = value
			}
		}
		err := schema.Validate(attrs)
		c.Assert(err, gc.ErrorMatches, excludedKey+": expected string, got nothing")
	}
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	// No environment variables set, so no credentials should be found.
	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(credentials, gc.HasLen, 0)
}

func (s *credentialsSuite) TestDetectCredentialsEnvironmentVariables(c *gc.C) {
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "key-id")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret-access-key")

	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.HasLen, 1)
	c.Assert(credentials[0], jc.DeepEquals, cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"access-key": "key-id",
			"secret-key": "secret-access-key",
		},
	))
}

func (s *credentialsSuite) accessKeyCredentialSchema(c *gc.C) cloud.CredentialSchema {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	return providerCredentials.CredentialSchemas()["access-key"]
}
