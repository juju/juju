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
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
)

type configSuite struct {
	baseProviderSuite
	oldUser string
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	s.oldUser = os.Getenv("USER")
	err := os.Setenv("USER", "tester")
	c.Assert(err, gc.IsNil)
}

func (s *configSuite) TearDownTest(c *gc.C) {
	os.Setenv("USER", s.oldUser)
	s.baseProviderSuite.TearDownTest(c)
}

func minimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name": "test",
		"type": "local",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.CAKey,
	}
}

func minimalConfig(c *gc.C) *config.Config {
	minimal := minimalConfigValues()
	testConfig, err := config.New(minimal)
	c.Assert(err, gc.IsNil)
	return testConfig
}

func (s *configSuite) TestValidateConfig(c *gc.C) {
	testConfig := minimalConfig(c)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)

	expectedRootDir := filepath.Join(s.root, "tester-test")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithRootDir(c *gc.C) {
	values := minimalConfigValues()
	root := c.MkDir()
	values["root-dir"] = root
	testConfig, err := config.New(values)
	c.Assert(err, gc.IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, root)
}

func (s *configSuite) TestNamespace(c *gc.C) {
	testConfig := minimalConfig(c)
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "tester-test")
}

func (s *configSuite) TestNamespaceRootNoSudo(c *gc.C) {
	err := os.Setenv("USER", "root")
	c.Assert(err, gc.IsNil)
	testConfig := minimalConfig(c)
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "root-test")
}

func (s *configSuite) TestNamespaceRootWithSudo(c *gc.C) {
	err := os.Setenv("USER", "root")
	c.Assert(err, gc.IsNil)
	err = os.Setenv("SUDO_USER", "tester")
	c.Assert(err, gc.IsNil)
	defer os.Setenv("SUDO_USER", "")
	testConfig := minimalConfig(c)
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "tester-test")
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
