// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"fmt"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/loggo"
)

type providerSuite struct {
	testing.LoggingSuite
	public     string
	private    string
	oldPublic  string
	oldPrivate string
}

var _ = Suite(&providerSuite{})

var _ = local.Provider

func (s *providerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	loggo.GetLogger("juju.environs.local").SetLogLevel(loggo.TRACE)
	s.public = filepath.Join(c.MkDir(), "%s", "public")
	s.private = filepath.Join(c.MkDir(), "%s", "private")
	s.oldPublic, s.oldPrivate = local.SetDefaultStorageDirs(s.public, s.private)
}

func (s *providerSuite) TearDownTest(c *C) {
	local.SetDefaultStorageDirs(s.oldPublic, s.oldPrivate)
	s.LoggingSuite.TearDownTest(c)
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

func (s *providerSuite) TestValidateConfig(c *C) {
	minimal := minimalConfigValues()
	testConfig, err := config.New(minimal)
	c.Assert(err, IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, IsNil)
	unknownAttrs := valid.UnknownAttrs()

	publicDir := fmt.Sprintf(s.public, "test")
	c.Assert(publicDir, IsDirectory)
	c.Assert(unknownAttrs["public-storage"], Equals, publicDir)

	privateDir := fmt.Sprintf(s.private, "test")
	c.Assert(privateDir, IsDirectory)
	c.Assert(unknownAttrs["private-storage"], Equals, privateDir)
}

func (s *providerSuite) TestValidateConfigWithStorage(c *C) {
	values := minimalConfigValues()
	public := c.MkDir()
	private := c.MkDir()
	values["public-storage"] = public
	values["private-storage"] = private
	testConfig, err := config.New(values)
	c.Assert(err, IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, IsNil)
	unknownAttrs := valid.UnknownAttrs()

	c.Assert(unknownAttrs["public-storage"], Equals, public)
	c.Assert(unknownAttrs["private-storage"], Equals, private)
}
