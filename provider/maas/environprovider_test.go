// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type EnvironProviderSuite struct {
	providerSuite
}

var _ = gc.Suite(&EnvironProviderSuite{})

func (s *EnvironProviderSuite) cloudSpec() environs.CloudSpec {
	credential := cloud.NewCredential(
		cloud.OAuth1AuthType,
		map[string]string{
			"maas-oauth": "aa:bb:cc",
		},
	)
	return environs.CloudSpec{
		Type:       "maas",
		Endpoint:   "http://maas.testing.invalid/maas/",
		Credential: &credential,
	}
}

func (suite *EnvironProviderSuite) TestSecretAttrsReturnsSensitiveMAASAttributes(c *gc.C) {
	const oauth = "aa:bb:cc"
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  oauth,
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	secretAttrs, err := providerInstance.SecretAttrs(config)
	c.Assert(err, jc.ErrorIsNil)

	expectedAttrs := map[string]string{"maas-oauth": oauth}
	c.Check(secretAttrs, gc.DeepEquals, expectedAttrs)
}

func (suite *EnvironProviderSuite) TestCredentialsSetup(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "maas",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := providerInstance.BootstrapConfig(environs.BootstrapConfigParams{
		Config: config,
		Cloud:  suite.cloudSpec(),
	})
	c.Assert(err, jc.ErrorIsNil)

	attrs = cfg.UnknownAttrs()
	server, ok := attrs["maas-server"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(server, gc.Equals, "http://maas.testing.invalid/maas/")
	oauth, ok := attrs["maas-oauth"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(oauth, gc.Equals, "aa:bb:cc")
}

func (suite *EnvironProviderSuite) TestUnknownAttrsContainAgentName(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "maas",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := providerInstance.BootstrapConfig(environs.BootstrapConfigParams{
		Config: config,
		Cloud:  suite.cloudSpec(),
	})
	c.Assert(err, jc.ErrorIsNil)

	unknownAttrs := cfg.UnknownAttrs()
	c.Assert(unknownAttrs["maas-server"], gc.Equals, "http://maas.testing.invalid/maas/")

	uuid, ok := unknownAttrs["maas-agent-name"]

	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, jc.Satisfies, utils.IsValidUUIDString)
}

func (suite *EnvironProviderSuite) TestMAASServerFromEndpoint(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type": "maas",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cloudSpec := suite.cloudSpec()
	cloudSpec.Endpoint = "maas.testing"

	cfg, err := providerInstance.BootstrapConfig(environs.BootstrapConfigParams{
		Config: config,
		Cloud:  cloudSpec,
	})
	c.Assert(err, jc.ErrorIsNil)

	unknownAttrs := cfg.UnknownAttrs()
	c.Assert(unknownAttrs["maas-server"], gc.Equals, "http://maas.testing/MAAS")
}

func (suite *EnvironProviderSuite) TestPrepareSetsAgentName(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  "aa:bb:cc",
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	config, err = providerInstance.PrepareForCreateEnvironment(suite.controllerUUID, config)
	c.Assert(err, jc.ErrorIsNil)

	uuid, ok := config.UnknownAttrs()["maas-agent-name"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, jc.Satisfies, utils.IsValidUUIDString)
}

func (suite *EnvironProviderSuite) TestPrepareExistingAgentName(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":            "maas",
		"maas-oauth":      "aa:bb:cc",
		"maas-server":     "http://maas.testing.invalid/maas/",
		"maas-agent-name": "foobar",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	_, err = providerInstance.PrepareForCreateEnvironment(suite.controllerUUID, config)
	c.Assert(err, gc.Equals, errAgentNameAlreadySet)
}

func (suite *EnvironProviderSuite) TestAgentNameShouldNotBeSetByHand(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":            "maas",
		"maas-agent-name": "foobar",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	_, err = providerInstance.BootstrapConfig(environs.BootstrapConfigParams{
		Config: config,
		Cloud:  suite.cloudSpec(),
	})
	c.Assert(err, gc.Equals, errAgentNameAlreadySet)
}

// create a temporary file with the given content.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *gc.C, content []byte) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, jc.ErrorIsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}

func (suite *EnvironProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	const oauth = "wrongly-formatted-oauth-string"
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  oauth,
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := providerInstance.Open(environs.OpenParams{
		Cloud:  suite.cloudSpec(),
		Config: config,
	})
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}
