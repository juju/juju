// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

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
	s.provider, err = environs.Provider("joyent")
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
	"sdc-user":     "sdc-user",
	"sdc-key-id":   "sdc-key-id",
	"manta-user":   "manta-user",
	"manta-key-id": "manta-key-id",
	"private-key":  "private-key",
	"algorithm":    "algorithm",
}

func (s *credentialsSuite) TestUserPassCredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "userpass")

	err := schema.Validate(sampleUserPassCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	// Only private-key is expected to be hidden during input.
	for key := range sampleUserPassCredentialAttributes {
		if key == "private-key" {
			c.Assert(schema[key].Hidden, jc.IsTrue)
		} else {
			c.Assert(schema[key].Hidden, jc.IsFalse)
		}
	}
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

func (s *credentialsSuite) credentialSchema(c *gc.C, authType cloud.AuthType) cloud.CredentialSchema {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	return providerCredentials.CredentialSchemas()[authType]
}
