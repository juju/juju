// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/testing"
)

type environProviderSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&environProviderSuite{})

func (e *environProviderSuite) NewProvider(c *gc.C) environs.EnvironProvider {
	provider, err := environs.Provider("oracle")
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.NotNil)
	return provider
}

func (e *environProviderSuite) TestCloudSchma(c *gc.C) {
	provider := e.NewProvider(c)
	schema := provider.CloudSchema()
	c.Assert(schema, gc.IsNil)
}

func (e *environProviderSuite) TestPing(c *gc.C) {
	provider := e.NewProvider(c)
	err := provider.Ping(context.NewCloudCallContext(), "")
	c.Assert(err, gc.IsNil)
}

func (e *environProviderSuite) TestPrepareConfigWithErrors(c *gc.C) {
	provider := e.NewProvider(c)
	_, err := provider.PrepareConfig(environs.PrepareConfigParams{})
	c.Assert(err, gc.NotNil)

	_, err = provider.PrepareConfig(environs.PrepareConfigParams{
		Config: testing.ModelConfig(c),
	})
	c.Assert(err, gc.NotNil)
}

func (e *environProviderSuite) TestPrepareConfig(c *gc.C) {
	provider := e.NewProvider(c)
	credentials := jujucloud.NewCredential(
		jujucloud.UserPassAuthType,
		map[string]string{
			"identity-domain": "bretdd",
		},
	)
	_, err := provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environscloudspec.CloudSpec{
			Type:       "oracle",
			Name:       "oracle",
			Credential: &credentials,
		},
		Config: testing.ModelConfig(c),
	})
	c.Assert(err, gc.IsNil)
}

func (e *environProviderSuite) TestOpen(c *gc.C) {
	provider := e.NewProvider(c)
	credentials := jujucloud.NewCredential(
		jujucloud.UserPassAuthType,
		map[string]string{
			"identity-domain": "bretdd",
			"username":        "some-friendly-username",
			"password":        "some-friendly-password",
		},
	)
	_, err := environs.Open(provider, environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type:       "oracle",
			Name:       "oracle",
			Credential: &credentials,
			Endpoint:   "https://127.0.0.1/",
		},
		Config: testing.ModelConfig(c),
	})
	c.Assert(err, gc.NotNil)
}

func (e *environProviderSuite) TestValidateWithErrors(c *gc.C) {
	provider := e.NewProvider(c)
	_, err := provider.Validate(
		testing.ModelConfig(c),
		testing.ModelConfig(c),
	)
	c.Assert(err, gc.NotNil)
}

func (e *environProviderSuite) TestValidate(c *gc.C) {
	provider := e.NewProvider(c)
	_, err := provider.Validate(testing.ModelConfig(c), nil)
	c.Assert(err, gc.IsNil)
}

func (e *environProviderSuite) TestCredentialSchema(c *gc.C) {
	provider := e.NewProvider(c)
	credentials := provider.CredentialSchemas()
	c.Assert(credentials,
		jc.DeepEquals,
		oracle.OracleCredentials,
	)
}

func (e *environProviderSuite) TestDetectCredentials(c *gc.C) {
	provider := e.NewProvider(c)
	_, err := provider.DetectCredentials()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (e *environProviderSuite) TestFinalizeCredential(c *gc.C) {
	provider := e.NewProvider(c)
	cloudcred := jujucloud.NewCredential(
		jujucloud.UserPassAuthType,
		map[string]string{
			"identity-domain": "bretdd",
			"username":        "some-friendly-username",
			"password":        "some-friendly-password",
		},
	)

	credentials, err := provider.FinalizeCredential(
		nil,
		environs.FinalizeCredentialParams{
			Credential: cloudcred,
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(credentials, gc.NotNil)
	c.Assert(*credentials, jc.DeepEquals, cloudcred)

}
