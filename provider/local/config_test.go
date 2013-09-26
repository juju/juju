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
	return testing.FakeConfig().Merge(testing.Attrs{
		"name": "test",
		"type": provider.Local,
	})
}

func minimalConfig(c *gc.C) *config.Config {
	minimal := minimalConfigValues()
	testConfig, err := config.New(config.NoDefaults, minimal)
	c.Assert(err, gc.IsNil)
	return testConfig
}

func (s *configSuite) TestValidateConfig(c *gc.C) {
	testConfig := minimalConfig(c)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)

	expectedRootDir := filepath.Join(osenv.Home(), ".juju", "test")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithRootDir(c *gc.C) {
	values := minimalConfigValues()
	root := c.MkDir()
	values["root-dir"] = root
	testConfig, err := config.New(config.NoDefaults, values)
	c.Assert(err, gc.IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, root)
}

func (s *configSuite) TestValidateConfigWithTildeInRootDir(c *gc.C) {
	values := minimalConfigValues()
	values["root-dir"] = "~/.juju/foo"
	testConfig, err := config.New(config.NoDefaults, values)
	c.Assert(err, gc.IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)

	expectedRootDir := filepath.Join(osenv.Home(), ".juju", "foo")
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["root-dir"], gc.Equals, expectedRootDir)
}

func (s *configSuite) TestValidateConfigWithFloatPort(c *gc.C) {
	// When the config values get serialized through JSON, the integers
	// get coerced to float64 values.  The parsing needs to handle this.
	values := minimalConfigValues()
	values["storage-port"] = float64(8040)
	testConfig, err := config.New(config.NoDefaults, values)
	c.Assert(err, gc.IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	unknownAttrs := valid.UnknownAttrs()
	c.Assert(unknownAttrs["storage-port"], gc.Equals, int(8040))
}

func (s *configSuite) TestNamespace(c *gc.C) {
	testConfig := minimalConfig(c)
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "tester-test")
}

func (s *configSuite) TestNamespaceRootNoSudo(c *gc.C) {
	restore := local.SetRootCheckFunction(func() bool { return true })
	defer restore()
	err := os.Setenv("USER", "root")
	c.Assert(err, gc.IsNil)
	testConfig := minimalConfig(c)
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "root-test")
}

func (s *configSuite) TestNamespaceRootWithSudo(c *gc.C) {
	restore := local.SetRootCheckFunction(func() bool { return true })
	defer restore()
	err := os.Setenv("USER", "root")
	c.Assert(err, gc.IsNil)
	err = os.Setenv("SUDO_USER", "tester")
	c.Assert(err, gc.IsNil)
	defer os.Setenv("SUDO_USER", "")
	testConfig := minimalConfig(c)
	c.Assert(local.ConfigNamespace(testConfig), gc.Equals, "tester-test")
}

func (s *configSuite) TestSudoCallerIds(c *gc.C) {
	defer os.Setenv("SUDO_UID", os.Getenv("SUDO_UID"))
	defer os.Setenv("SUDO_GID", os.Getenv("SUDO_GID"))
	for _, test := range []struct {
		uid         string
		gid         string
		errString   string
		expectedUid int
		expectedGid int
	}{{
		uid: "",
		gid: "",
	}, {
		uid:         "1001",
		gid:         "1002",
		expectedUid: 1001,
		expectedGid: 1002,
	}, {
		uid:       "1001",
		gid:       "foo",
		errString: `invalid value "foo" for SUDO_GID`,
	}, {
		uid:       "foo",
		gid:       "bar",
		errString: `invalid value "foo" for SUDO_UID`,
	}} {
		os.Setenv("SUDO_UID", test.uid)
		os.Setenv("SUDO_GID", test.gid)
		uid, gid, err := local.SudoCallerIds()
		if test.errString == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(uid, gc.Equals, test.expectedUid)
			c.Assert(gid, gc.Equals, test.expectedGid)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errString)
			c.Assert(uid, gc.Equals, 0)
			c.Assert(gid, gc.Equals, 0)
		}
	}
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
