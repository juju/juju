// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/juju/internal/packaging/manager"
)

var _ = gc.Suite(&AptSuite{})

type AptSuite struct {
	testing.IsolationSuite
	paccmder commands.PackageCommander
	pacman   manager.PackageManager
}

func (s *AptSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.paccmder = commands.NewAptPackageCommander()
	s.pacman = manager.NewAptPackageManager()
}

func (s *AptSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *AptSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
}

func (s *AptSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
}
