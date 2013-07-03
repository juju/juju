// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
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
	unknownAttrs := valid.UnknownAttrs()

	root := filepath.Join(s.root, "tester-test")
	c.Assert(root, jc.IsDirectory)
	c.Assert(unknownAttrs["root-dir"], gc.Equals, root)
}

func (s *configSuite) TestValidateConfigWithStorage(c *gc.C) {
	values := minimalConfigValues()
	root := c.MkDir()
	values["root-dir"] = root
	testConfig, err := config.New(values)
	c.Assert(err, gc.IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)
	unknownAttrs := valid.UnknownAttrs()

	c.Assert(root, jc.IsDirectory)
	c.Assert(unknownAttrs["root-dir"], gc.Equals, root)
}
