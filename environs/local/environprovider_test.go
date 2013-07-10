// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
)

type baseProviderSuite struct {
	testing.LoggingSuite
	root    string
	oldRoot string
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	loggo.GetLogger("juju.environs.local").SetLogLevel(loggo.TRACE)
	s.root = c.MkDir()
	s.oldRoot = local.SetDefaultRootDir(s.root)
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	local.SetDefaultRootDir(s.oldRoot)
	s.LoggingSuite.TearDownTest(c)
}
