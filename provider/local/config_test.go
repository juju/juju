// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/provider/local"
	"launchpad.net/juju-core/testing"
)

type configSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&configSuite{})

func minimalConfigValues() map[string]interface{} {
	return testing.FakeConfig().Merge(testing.Attrs{
		"name": "test",
		"type": provider.Local,
	})
}

func minimalConfig(c *gc.C) *config.Config {
	minimal := minimalConfigValues()
	testConfig, err := config.New(config.NoDefaults, minimal)
	c.Assert(err, gc.IsNil)
	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	return valid
}

func localConfig(c *gc.C, extra map[string]interface{}) *config.Config {
	values := minimalConfigValues()
	for key, value := range extra {
		values[key] = value
	}
	testConfig, err := config.New(config.NoDefaults, values)
	c.Assert(err, gc.IsNil)
	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	return valid
}

func (s *configSuite) TestDefaultNetworkBridge(c *gc.C) {
	config := minimalConfig(c)
	unknownAttrs := config.UnknownAttrs()
	c.Assert(unknownAttrs["network-bridge"], gc.Equals, "lxcbr0")
}

func (s *configSuite) TestSetNetworkBridge(c *gc.C) {
	config := localConfig(c, map[string]interface{}{
		"network-bridge": "br0",
	})
	unknownAttrs := config.UnknownAttrs()
	c.Assert(unknownAttrs["network-bridge"], gc.Equals, "br0")
}

func (s *configSuite) TestValidateConfig(c *gc.C) {
	valid := minimalConfig(c)
	expectedRootDir := filepath.Join(osenv.Home(), ".juju", "test")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithRootDir(c *gc.C) {
	root := c.MkDir()
	valid := localConfig(c, map[string]interface{}{
		"root-dir": root,
	})
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, root)
}

func (s *configSuite) TestValidateConfigWithTildeInRootDir(c *gc.C) {
	valid := localConfig(c, map[string]interface{}{
		"root-dir": "~/.juju/foo",
	})
	expectedRootDir := filepath.Join(osenv.Home(), ".juju", "foo")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithFloatPort(c *gc.C) {
	// When the config values get serialized through JSON, the integers
	// get coerced to float64 values.  The parsing needs to handle this.
	valid := localConfig(c, map[string]interface{}{
		"storage-port": float64(8040),
	})
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["storage-port"], gc.Equals, int(8040))
}

func (s *configSuite) TestNamespace(c *gc.C) {
	testConfig := minimalConfig(c)
	s.PatchEnvironment("USER", "tester")
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "tester-test")
}

func (s *configSuite) TestBootstrapAsRoot(c *gc.C) {
	s.PatchValue(local.CheckIfRoot, func() bool { return true })
	env, err := local.Provider.Prepare(testing.Context(c), minimalConfig(c))
	c.Assert(err, gc.IsNil)
	err = env.Bootstrap(testing.Context(c), environs.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "bootstrapping a local environment must not be done as root")
}
