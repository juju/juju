// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

type OpenSuite struct {
	envtesting.ToolsFixture
}

var _ = gc.Suite(&OpenSuite{})

func (OpenSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
}

func (OpenSuite) TestNewDummyEnviron(c *gc.C) {
	// matches *Settings.Map()
	cfg, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	c.Assert(bootstrap.Bootstrap(env, constraints.Value{}), gc.IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"type": "wondercloud",
	})
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.ErrorMatches, "no registered provider for.*")
	c.Assert(env, gc.IsNil)
}

func (OpenSuite) TestNewFromName(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()
	e, err := environs.NewFromName("erewhemos")
	c.Assert(err, gc.IsNil)
	c.Assert(e.Name(), gc.Equals, "erewhemos")
	c.Assert(func() { e.Storage() }, gc.PanicMatches, "environment .* is not prepared")
}

func (OpenSuite) TestNewFromNameNoDefault(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()

	e, err := environs.NewFromName("")
	c.Assert(err, gc.ErrorMatches, "no default environment found")
	c.Assert(e, gc.IsNil)
}

func (OpenSuite) TestNewFromNameDefault(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig, testing.SampleCertName).Restore()
	e, err := environs.NewFromName("")
	c.Assert(err, gc.IsNil)
	c.Assert(e.Name(), gc.Equals, "erewhemos")
}

func (OpenSuite) TestPrepareFromName(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()
	e, err := environs.PrepareFromName("erewhemos")
	c.Assert(err, gc.IsNil)
	c.Assert(e.Name(), gc.Equals, "erewhemos")
	// Check we can access storage ok, which implies the environment has been prepared.
	c.Assert(e.Storage(), gc.NotNil)
}

func (OpenSuite) TestConfigForName(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()
	cfg, err := environs.ConfigForName("erewhemos")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "erewhemos")
}

func (OpenSuite) TestConfigForNameNoDefault(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault, testing.SampleCertName).Restore()
	_, err := environs.ConfigForName("")
	c.Assert(err, gc.ErrorMatches, "no default environment found")
}

func (OpenSuite) TestConfigForNameDefault(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig, testing.SampleCertName).Restore()
	cfg, err := environs.ConfigForName("")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "erewhemos")
}

func (OpenSuite) TestNew(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
		},
	))
	c.Assert(err, gc.IsNil)
	e, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(func() { e.Storage() }, gc.PanicMatches, "environment .* is not prepared")
}

func (OpenSuite) TestPrepare(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
		},
	))
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, configstore.NewMem())
	c.Assert(err, gc.IsNil)
	// Check we can access storage ok, which implies the environment has been prepared.
	c.Assert(e.Storage(), gc.NotNil)
}

func (OpenSuite) TestNewFromAttrs(c *gc.C) {
	e, err := environs.NewFromAttrs(dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server": false,
			"name":         "erewhemos",
		},
	))
	c.Assert(err, gc.IsNil)
	c.Assert(func() { e.Storage() }, gc.PanicMatches, "environment .* is not prepared")
}

const checkEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

type checkEnvironmentSuite struct{}

var _ = gc.Suite(&checkEnvironmentSuite{})

func (s *checkEnvironmentSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
}

func (s *checkEnvironmentSuite) TestCheckEnvironment(c *gc.C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.PrepareFromName("test")
	c.Assert(err, gc.IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.IsNil)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentFileNotFound(c *gc.C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.PrepareFromName("test")
	c.Assert(err, gc.IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)

	// When the bootstrap-verify file does not exist, it still believes
	// the environment is a juju-core one because earlier versions
	// did not create that file.
	err = stor.Remove("bootstrap-verify")
	c.Assert(err, gc.IsNil)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.IsNil)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentGetFails(c *gc.C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.PrepareFromName("test")
	c.Assert(err, gc.IsNil)

	// VerifyStorage is sufficient for our tests and much simpler
	// than Bootstrap which calls it.
	stor := environ.Storage()
	err = environs.VerifyStorage(stor)
	c.Assert(err, gc.IsNil)

	// When fetching the verification file from storage fails,
	// we get an InvalidEnvironmentError.
	someError := errors.Unauthorizedf("you shall not pass")
	dummy.Poison(stor, "bootstrap-verify", someError)
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.Equals, someError)
}

func (s *checkEnvironmentSuite) TestCheckEnvironmentBadContent(c *gc.C) {
	defer testing.MakeFakeHome(c, checkEnv, "existing").Restore()

	environ, err := environs.PrepareFromName("test")
	c.Assert(err, gc.IsNil)

	// We mock a bad (eg. from a Python-juju environment) bootstrap-verify.
	stor := environ.Storage()
	content := "bad verification content"
	reader := strings.NewReader(content)
	err = stor.Put("bootstrap-verify", reader, int64(len(content)))
	c.Assert(err, gc.IsNil)

	// When the bootstrap-verify file contains unexpected content,
	// we get an InvalidEnvironmentError.
	err = environs.CheckEnvironment(environ)
	c.Assert(err, gc.Equals, environs.InvalidEnvironmentError)
}
