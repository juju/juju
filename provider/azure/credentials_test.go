// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"io"

	"github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/azure"
	coretesting "github.com/juju/juju/testing"
)

type credentialsSuite struct {
	testing.IsolationSuite
	interactiveCreateServicePrincipalCreator
	provider environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.interactiveCreateServicePrincipalCreator = interactiveCreateServicePrincipalCreator{}
	s.provider = newProvider(c, azure.ProviderConfig{
		InteractiveCreateServicePrincipal: s.interactiveCreateServicePrincipalCreator.InteractiveCreateServicePrincipal,
	})
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
	ctx := coretesting.Context(c)
	out, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{Credential: in})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.NotNil)
	c.Assert(out.AuthType(), gc.Equals, cloud.AuthType("service-principal-secret"))
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"application-id":       "application",
		"application-password": "password",
		"subscription-id":      "subscription",
	})
	stderr := coretesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
WARNING: The "userpass" auth-type is deprecated, and will be removed soon.

Please update the credential in ~/.local/share/juju/credentials.yaml,
changing auth-type to "service-principal-secret", and dropping the tenant-id field.

`[1:])
}

func (s *credentialsSuite) TestFinalizeCredentialInteractive(c *gc.C) {
	in := cloud.NewCredential("interactive", map[string]string{"subscription-id": "subscription"})
	ctx := coretesting.Context(c)
	out, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.NotNil)
	c.Assert(out.AuthType(), gc.Equals, cloud.AuthType("service-principal-secret"))
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"application-id":       "appid",
		"application-password": "service-principal-password",
		"subscription-id":      "subscription",
	})

	s.interactiveCreateServicePrincipalCreator.CheckCallNames(c, "InteractiveCreateServicePrincipal")
	args := s.interactiveCreateServicePrincipalCreator.Calls()[0].Args
	c.Assert(args[3], gc.Equals, "https://arm.invalid")
	c.Assert(args[4], gc.Equals, "https://graph.invalid")
	c.Assert(args[5], gc.Equals, "subscription")
}

func (s *credentialsSuite) TestFinalizeCredentialInteractiveError(c *gc.C) {
	in := cloud.NewCredential("interactive", map[string]string{"subscription-id": "subscription"})
	s.interactiveCreateServicePrincipalCreator.SetErrors(errors.New("blargh"))
	ctx := coretesting.Context(c)
	_, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, gc.ErrorMatches, "blargh")
}

type interactiveCreateServicePrincipalCreator struct {
	testing.Stub
}

func (c *interactiveCreateServicePrincipalCreator) InteractiveCreateServicePrincipal(
	stderr io.Writer,
	sender autorest.Sender,
	requestInspector autorest.PrepareDecorator,
	resourceManagerEndpoint string,
	graphEndpoint string,
	subscriptionId string,
	clock clock.Clock,
	newUUID func() (utils.UUID, error),
) (appId, password string, _ error) {
	c.MethodCall(
		c, "InteractiveCreateServicePrincipal",
		stderr, sender, requestInspector, resourceManagerEndpoint,
		graphEndpoint, subscriptionId, clock, newUUID,
	)
	return "appid", "service-principal-password", c.NextErr()
}
