// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
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
	s.provider = newProvider(c, azure.ProviderConfig{})
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider,
		"interactive",
		"service-principal-secret",
		"userpass",
	)
}

var sampleCredentialAttributes = map[string]string{
	"application-id":       "application",
	"application-password": "password",
	"subscription-id":      "subscription",
}

func (s *credentialsSuite) TestUserPassCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "userpass", map[string]string{
		"application-id":       "application",
		"application-password": "password",
		"subscription-id":      "subscription",
		"tenant-id":            "tenant",
	})
}

func (s *credentialsSuite) TestServicePrincipalSecretCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "service-principal-secret", map[string]string{
		"application-id":       "application",
		"application-password": "password",
		"subscription-id":      "subscription",
	})
}

func (s *credentialsSuite) TestServicePrincipalSecretHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "service-principal-secret", "application-password")
}

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *credentialsSuite) TestFinalizeCredentialUserPass(c *gc.C) {
	in := cloud.NewCredential("userpass", map[string]string{
		"application-id":       "application",
		"application-password": "password",
		"subscription-id":      "subscription",
		"tenant-id":            "tenant",
	})
	ctx := testing.Context(c)
	out, err := s.provider.FinalizeCredential(ctx, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.AuthType(), gc.Equals, cloud.AuthType("service-principal-secret"))
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"application-id":       "application",
		"application-password": "password",
		"subscription-id":      "subscription",
	})
	stderr := testing.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
WARNING: The "userpass" auth-type is deprecated, and will be removed soon.

Please update the credential in ~/.local/share/juju/credentials.yaml,
changing auth-type to "service-principal-secret", and dropping the tenant-id field.

`[1:])
}
