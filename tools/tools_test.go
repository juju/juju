// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type ToolsSuite struct{}

var _ = gc.Suite(&ToolsSuite{})

func (s *ToolsSuite) TestUseZipOn126(c *gc.C) {
	use := tools.UseZipToolsWindows(version.MustParseBinary("1.26.3-win2012r2-amd64"))
	c.Assert(use, jc.IsTrue)
	use = tools.UseZipToolsWindows(version.MustParseBinary("2.0.0-win2012r2-amd64"))
	c.Assert(use, jc.IsTrue)
	use = tools.UseZipToolsWindows(version.MustParseBinary("2.26.15-win2012r2-amd64"))
	c.Assert(use, jc.IsTrue)
}

func (s *ToolsSuite) TestDoNotUseZipUnder125(c *gc.C) {
	use := tools.UseZipToolsWindows(version.MustParseBinary("1.25.3-win2012r2-amd64"))
	c.Assert(use, jc.IsFalse)
	use = tools.UseZipToolsWindows(version.MustParseBinary("1.21.1-win2012r2-amd64"))
	c.Assert(use, jc.IsFalse)
}
