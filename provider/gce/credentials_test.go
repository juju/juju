// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

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
	s.provider, err = environs.Provider("gce")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 2)
	_, ok := schemas["oauth2"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected access-key auth-type schema"))
	_, ok = schemas["jsonfile"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected jsonfile auth-type schema"))
}

var sampleCredentialAttributes = map[string]string{
	"client-id":    "123",
	"client-email": "test@example.com",
	"project-id":   "fourfivesix",
	"private-key":  "sewen",
}

func (s *credentialsSuite) TestOAuth2CredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "oauth2")

	err := schema.Validate(sampleCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(schema["client-id"].Hidden, jc.IsFalse)
	c.Assert(schema["client-email"].Hidden, jc.IsFalse)
	c.Assert(schema["project-id"].Hidden, jc.IsFalse)
	c.Assert(schema["private-key"].Hidden, jc.IsTrue)
}

func (s *credentialsSuite) TestOAuth2CredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.credentialSchema(c, "oauth2")

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

func (s *credentialsSuite) TestJSONFileCredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "jsonfile")
	err := schema.Validate(map[string]string{"file": "whatever"})
	// For now at least, the contents of the file are not validated
	// by the credentials schema. That is left to the provider.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestJSONFileCredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.credentialSchema(c, "jsonfile")
	err := schema.Validate(nil)
	c.Assert(err, gc.ErrorMatches, "file: expected string, got nothing")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(credentials, gc.HasLen, 0)
}

func (s *credentialsSuite) credentialSchema(c *gc.C, authType cloud.AuthType) cloud.CredentialSchema {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	return providerCredentials.CredentialSchemas()[authType]
}
