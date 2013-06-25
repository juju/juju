// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
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
	public := filepath.Join(c.MkDir(), "%s", "public")
	private := filepath.Join(c.MkDir(), "%s", "private")
	s.oldPublic, s.oldPrivate = local.SetDefaultStorageDirs(public, private)
}

func (s *providerSuite) TearDownTest(c *C) {
	local.SetDefaultStorageDirs(s.oldPublic, s.oldPrivate)
	s.LoggingSuite.TearDownTest(c)
}

func (*providerSuite) TestValidateConfig(c *C) {
	minimal := map[string]interface{}{
		"name": "test",
		"type": "local",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.CAKey,
	}
	testConfig, err := config.New(minimal)
	c.Assert(err, IsNil)

	valid, err := local.Provider.Validate(testConfig, nil)
	c.Assert(err, IsNil)

	_ = valid
}
