// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
)

type baseProviderSuite struct {
	testing.LoggingSuite
	lxc.TestSuite
	home    *testing.FakeHome
	restore func()
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeNoEnvironments(c, "test")
	loggo.GetLogger("juju.environs.local").SetLogLevel(loggo.TRACE)
	s.restore = local.MockAddressForInterface()
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.restore()
	s.home.Restore()
	s.TestSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}
