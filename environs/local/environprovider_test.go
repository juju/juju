// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs/local"
	"launchpad.net/juju-core/testing"
)

type baseProviderSuite struct {
	testing.LoggingSuite
	public     string
	private    string
	oldPublic  string
	oldPrivate string
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	loggo.GetLogger("juju.environs.local").SetLogLevel(loggo.TRACE)
	s.public = filepath.Join(c.MkDir(), "%s", "public")
	s.private = filepath.Join(c.MkDir(), "%s", "private")
	s.oldPublic, s.oldPrivate = local.SetDefaultStorageDirs(s.public, s.private)
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	local.SetDefaultStorageDirs(s.oldPublic, s.oldPrivate)
	s.LoggingSuite.TearDownTest(c)
}
