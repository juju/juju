// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/testing"
)

type credentialsSuite struct {
	testing.BaseSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider, _ = newProviders(c, azure.ProviderConfig{})
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 1)
	_, ok := schemas["userpass"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected userpass auth-type schema"))
}

var sampleCredentialAttributes = map[string]string{
	"application-id":       "application",
	"application-password": "password",
	"subscription-id":      "subscription",
	"tenant-id":            "tenant",
}

func (s *credentialsSuite) TestUserPassCredentialSchema(c *gc.C) {
	schema := s.userpassCredentialSchema(c)

	err := schema.Validate(sampleCredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	// Only application-password is expected to be hidden during input.
	for key := range sampleCredentialAttributes {
		if key == "application-password" {
			c.Assert(schema[key].Hidden, jc.IsTrue)
		} else {
			c.Assert(schema[key].Hidden, jc.IsFalse)
		}
	}
}

func (s *credentialsSuite) TestUserPassCredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.userpassCredentialSchema(c)

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

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	credentials, err := providerCredentials.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(credentials, gc.HasLen, 0)
}

func (s *credentialsSuite) userpassCredentialSchema(c *gc.C) cloud.CredentialSchema {
	providerCredentials := s.provider.(environs.ProviderCredentials)
	return providerCredentials.CredentialSchemas()["userpass"]
}
