// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/testing"
)

type EnvironProviderSuite struct {
	providerSuite
}

var _ = gc.Suite(&EnvironProviderSuite{})

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

func (suite *EnvironProviderSuite) TestUnknownAttrsContainAgentName(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  "aa:bb:cc",
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	ctx := envtesting.BootstrapContext(c)
	environ, err := providerInstance.PrepareForBootstrap(ctx, config)
	c.Assert(err, jc.ErrorIsNil)

	preparedConfig := environ.Config()
	unknownAttrs := preparedConfig.UnknownAttrs()

	uuid, ok := unknownAttrs["maas-agent-name"]

	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, jc.Satisfies, utils.IsValidUUIDString)
}

func (suite *EnvironProviderSuite) TestPrepareSetsAgentName(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  "aa:bb:cc",
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	config, err = providerInstance.PrepareForCreateEnvironment(config)
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

	_, err = providerInstance.PrepareForCreateEnvironment(config)
	c.Assert(err, gc.Equals, errAgentNameAlreadySet)
}

func (suite *EnvironProviderSuite) TestAgentNameShouldNotBeSetByHand(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":            "maas",
		"maas-oauth":      "aa:bb:cc",
		"maas-server":     "http://maas.testing.invalid/maas/",
		"maas-agent-name": "foobar",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	ctx := envtesting.BootstrapContext(c)
	_, err = providerInstance.PrepareForBootstrap(ctx, config)
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
	env, err := providerInstance.Open(config)
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}
