// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

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
	s.provider, err = environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 1)
	_, ok := schemas["userpass"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected userpass auth-type schema"))
}

var sampleUserPassCredentialAttributes = map[string]string{
	"user":     "bob",
	"password": "dobbs",
}

func (s *credentialsSuite) TestUserPassCredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "userpass")

	err := schema.Validate(sampleUserPassCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(schema["user"].Hidden, jc.IsFalse)
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
	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(credentials, gc.HasLen, 0)
}

func (s *credentialsSuite) credentialSchema(c *gc.C, authType cloud.AuthType) cloud.CredentialSchema {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	return providerCredentials.CredentialSchemas()[authType]
}
