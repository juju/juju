// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type EnvironProviderSuite struct {
	providerSuite
}

var _ = gc.Suite(&EnvironProviderSuite{})

func (suite *EnvironProviderSuite) TestSecretAttrsReturnsSensitiveMAASAttributes(c *gc.C) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	const oauth = "aa:bb:cc"
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  oauth,
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	secretAttrs, err := suite.makeEnviron().Provider().SecretAttrs(config)
	c.Assert(err, gc.IsNil)

	expectedAttrs := map[string]string{"maas-oauth": oauth}
	c.Check(secretAttrs, gc.DeepEquals, expectedAttrs)
}

func (suite *EnvironProviderSuite) TestUnknownAttrsContainAgentName(c *gc.C) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  "aa:bb:cc",
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	ctx := testing.Context(c)
	environ, err := suite.makeEnviron().Provider().Prepare(ctx, config)
	c.Assert(err, gc.IsNil)

	preparedConfig := environ.Config()
	unknownAttrs := preparedConfig.UnknownAttrs()

	uuid, ok := unknownAttrs["maas-agent-name"]

	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, jc.Satisfies, utils.IsValidUUIDString)
}

func (suite *EnvironProviderSuite) TestAgentNameShouldNotBeSetByHand(c *gc.C) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":            "maas",
		"maas-oauth":      "aa:bb:cc",
		"maas-server":     "http://maas.testing.invalid/maas/",
		"maas-agent-name": "foobar",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	ctx := testing.Context(c)
	_, err = suite.makeEnviron().Provider().Prepare(ctx, config)
	c.Assert(err, gc.Equals, errAgentNameAlreadySet)
}

// create a temporary file with the given content.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *gc.C, content []byte) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, gc.IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, gc.IsNil)
	return filename
}

func (suite *EnvironProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	const oauth = "wrongly-formatted-oauth-string"
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":        "maas",
		"maas-oauth":  oauth,
		"maas-server": "http://maas.testing.invalid/maas/",
	})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := suite.makeEnviron().Provider().Open(config)
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}
