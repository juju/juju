// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type JujuDataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&JujuDataSuite{})

func (s *JujuDataSuite) TestStandardHome(c *gc.C) {
	testJujuData := c.MkDir()
	osenv.SetJujuData(testJujuData)
	c.Assert(osenv.JujuData(), gc.Equals, testJujuData)
}

func (s *JujuDataSuite) TestErrorHome(c *gc.C) {
	// Invalid juju home leads to panic when retrieving.
	f := func() { _ = osenv.JujuData() }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
	f = func() { _ = osenv.JujuDataPath("environments.yaml") }
	c.Assert(f, gc.PanicMatches, "juju home hasn't been initialized")
}

func (s *JujuDataSuite) TestHomePath(c *gc.C) {
	testJujuData := c.MkDir()
	osenv.SetJujuData(testJujuData)
	envPath := osenv.JujuDataPath("environments.yaml")
	c.Assert(envPath, gc.Equals, filepath.Join(testJujuData, "environments.yaml"))
}

func (s *JujuDataSuite) TestIsHomeSet(c *gc.C) {
	c.Assert(osenv.IsJujuDataSet(), jc.IsFalse)
	osenv.SetJujuData(c.MkDir())
	c.Assert(osenv.IsJujuDataSet(), jc.IsTrue)
}
