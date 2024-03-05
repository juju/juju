// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BootstrapSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) TestBootstrapParamsPath(c *gc.C) {
	// Note: I'm hard coding the path here, because I don't know the
	// consequences of changing the params file name. So recording it
	// here should be enough warning to be careful about changing it.
	path := BootstrapParamsPath("/var/lib/juju")
	c.Assert(path, gc.Equals, "/var/lib/juju/bootstrap-params")
}

func (s *BootstrapSuite) TestIsBootstrapController(c *gc.C) {
	dir := c.MkDir()
	_, err := os.Create(filepath.Join(dir, "bootstrap-params"))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(IsBootstrapController(dir), gc.Equals, true)
}

func (s *BootstrapSuite) TestIsBootstrapControllerIsFalse(c *gc.C) {
	dir := c.MkDir()
	c.Assert(IsBootstrapController(dir), gc.Equals, false)
}
