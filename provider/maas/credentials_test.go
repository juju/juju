// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
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
	envtesting.AssertProviderAuthTypes(c, s.provider, "oauth1")
}

func (s *credentialsSuite) TestOAuth1CredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "oauth1", map[string]string{
		"maas-oauth": "123:456:789",
	})
}

func (s *credentialsSuite) TestOAuth1HiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth1", "maas-oauth")
}

func (s *credentialsSuite) TestDetectCredentialsNotFound(c *gc.C) {
	credentials, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(credentials, gc.HasLen, 0)
}
