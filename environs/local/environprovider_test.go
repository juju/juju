// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing"
)

type baseProviderSuite struct {
	testing.LoggingSuite
	restoreDataDir func()
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	loggo.GetLogger("juju.environs.local").SetLogLevel(loggo.TRACE)
	s.restoreDataDir = envtesting.PatchDataDir(c.MkDir())
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.restoreDataDir()
	s.LoggingSuite.TearDownTest(c)
}
