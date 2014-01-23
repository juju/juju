// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	gc "launchpad.net/gocheck"

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

func (s *configSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
}

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
	restore := local.SetRootCheckFunction(func() bool { return true })
	defer restore()
	_, err := local.Provider.Prepare(minimalConfig(c))
	c.Assert(err, gc.ErrorMatches, "bootstrapping a local environment must not be done as root")
}

type configRootSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&configRootSuite{})

func (s *configRootSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	// Skip if not linux
	if runtime.GOOS != "linux" {
		c.Skip("not running linux")
	}
	// Skip if not running as root.
	if os.Getuid() != 0 {
		c.Skip("not running as root")
	}
}

func (s *configRootSuite) TestCreateDirsNoUserJustRoot(c *gc.C) {
	defer os.Setenv("SUDO_UID", os.Getenv("SUDO_UID"))
	defer os.Setenv("SUDO_GID", os.Getenv("SUDO_GID"))

	os.Setenv("SUDO_UID", "")
	os.Setenv("SUDO_GID", "")

	testConfig := minimalConfig(c)
	err := local.CreateDirs(c, testConfig)
	c.Assert(err, gc.IsNil)
	// Check that the dirs are owned by root.
	for _, dir := range local.CheckDirs(c, testConfig) {
		info, err := os.Stat(dir)
		c.Assert(err, gc.IsNil)
		// This call is linux specific, but then so is sudo
		c.Assert(info.Sys().(*syscall.Stat_t).Uid, gc.Equals, uint32(0))
		c.Assert(info.Sys().(*syscall.Stat_t).Gid, gc.Equals, uint32(0))
	}
}

func (s *configRootSuite) TestCreateDirsAsUser(c *gc.C) {
	defer os.Setenv("SUDO_UID", os.Getenv("SUDO_UID"))
	defer os.Setenv("SUDO_GID", os.Getenv("SUDO_GID"))

	os.Setenv("SUDO_UID", "1000")
	os.Setenv("SUDO_GID", "1000")

	testConfig := minimalConfig(c)
	err := local.CreateDirs(c, testConfig)
	c.Assert(err, gc.IsNil)
	// Check that the dirs are owned by the UID/GID set above..
	for _, dir := range local.CheckDirs(c, testConfig) {
		info, err := os.Stat(dir)
		c.Assert(err, gc.IsNil)
		// This call is linux specific, but then so is sudo
		c.Assert(info.Sys().(*syscall.Stat_t).Uid, gc.Equals, uint32(1000))
		c.Assert(info.Sys().(*syscall.Stat_t).Gid, gc.Equals, uint32(1000))
	}
}
