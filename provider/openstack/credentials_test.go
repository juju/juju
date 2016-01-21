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
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 2)
	_, ok := schemas["access-key"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected access-key auth-type schema"))
	_, ok = schemas["userpass"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected userpass auth-type schema"))
}

var sampleAccessKeyCredentialAttributes = map[string]string{
	"access-key":  "key",
	"secret-key":  "secret",
	"tenant-name": "gary",
}

func (s *credentialsSuite) TestAccessKeyCredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "access-key")

	err := schema.Validate(sampleAccessKeyCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(schema["access-key"].Hidden, jc.IsFalse)
	c.Assert(schema["tenant-name"].Hidden, jc.IsFalse)
	c.Assert(schema["secret-key"].Hidden, jc.IsTrue)
}

func (s *credentialsSuite) TestAccessKeyCredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.credentialSchema(c, "access-key")

	// If any one of the attributes is missing, it's an error.
	for excludedKey := range sampleAccessKeyCredentialAttributes {
		attrs := make(map[string]string)
		for key, value := range sampleAccessKeyCredentialAttributes {
			if key != excludedKey {
				attrs[key] = value
			}
		}
		err := schema.Validate(attrs)
		c.Assert(err, gc.ErrorMatches, excludedKey+": expected string, got nothing")
	}
}

var sampleUserPassCredentialAttributes = map[string]string{
	"username":    "bob",
	"password":    "dobbs",
	"tenant-name": "gary",
}

func (s *credentialsSuite) TestUserPassCredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "userpass")

	err := schema.Validate(sampleUserPassCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(schema["username"].Hidden, jc.IsFalse)
	c.Assert(schema["tenant-name"].Hidden, jc.IsFalse)
	c.Assert(schema["password"].Hidden, jc.IsTrue)
}

func (s *credentialsSuite) TestUserPassCredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.credentialSchema(c, "userpass")

	// If any one of the attributes is missing, it's an error.
	for excludedKey := range sampleUserPassCredentialAttributes {
		attrs := make(map[string]string)
		for key, value := range sampleUserPassCredentialAttributes {
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

func (s *credentialsSuite) TestDetectCredentialsAccessKeyEnvironmentVariables(c *gc.C) {
	s.PatchEnvironment("OS_TENANT_NAME", "gary")
	s.PatchEnvironment("OS_ACCESS_KEY", "key-id")
	s.PatchEnvironment("OS_SECRET_KEY", "secret-access-key")

	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.HasLen, 1)
	c.Assert(credentials[0], jc.DeepEquals, cloud.NewCredential(
		cloud.AccessKeyAuthType, map[string]string{
			"access-key":  "key-id",
			"secret-key":  "secret-access-key",
			"tenant-name": "gary",
		},
	))
}

func (s *credentialsSuite) TestDetectCredentialsUserPassEnvironmentVariables(c *gc.C) {
	s.PatchEnvironment("OS_TENANT_NAME", "gary")
	s.PatchEnvironment("OS_USERNAME", "bob")
	s.PatchEnvironment("OS_PASSWORD", "dobbs")

	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.HasLen, 1)
	c.Assert(credentials[0], jc.DeepEquals, cloud.NewCredential(
		cloud.UserPassAuthType, map[string]string{
			"username":    "bob",
			"password":    "dobbs",
			"tenant-name": "gary",
		},
	))
}

func (s *credentialsSuite) credentialSchema(c *gc.C, authType cloud.AuthType) cloud.CredentialSchema {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	return providerCredentials.CredentialSchemas()[authType]
}
