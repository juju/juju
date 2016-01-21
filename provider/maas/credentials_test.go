// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

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
	s.provider, err = environs.Provider("maas")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.ProviderCredentials))
	providerCredentials := s.provider.(environs.ProviderCredentials)

	schemas := providerCredentials.CredentialSchemas()
	c.Assert(schemas, gc.HasLen, 1)
	_, ok := schemas["oauth1"]
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected oauth1 auth-type schema"))
}

var sampleOAuth1CredentialAttributes = map[string]string{
	"maas-oauth": "123:456:789",
}

func (s *credentialsSuite) TestOAuth1CredentialSchema(c *gc.C) {
	schema := s.credentialSchema(c, "oauth1")

	err := schema.Validate(sampleOAuth1CredentialAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(schema["maas-oauth"].Hidden, jc.IsTrue)
}

func (s *credentialsSuite) TestOAuth1CredentialSchemaMissingAttributes(c *gc.C) {
	schema := s.credentialSchema(c, "oauth1")
	err := schema.Validate(nil)
	c.Assert(err, gc.ErrorMatches, "maas-oauth: expected string, got nothing")
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
