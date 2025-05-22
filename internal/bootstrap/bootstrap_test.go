// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type BootstrapSuite struct {
	testhelpers.IsolationSuite
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &BootstrapSuite{})
}

func (s *BootstrapSuite) TestBootstrapParamsPath(c *tc.C) {
	// Note: I'm hard coding the path here, because I don't know the
	// consequences of changing the params file name. So recording it
	// here should be enough warning to be careful about changing it.
	path := BootstrapParamsPath("/var/lib/juju")
	c.Assert(path, tc.Equals, "/var/lib/juju/bootstrap-params")
}

func (s *BootstrapSuite) TestIsBootstrapController(c *tc.C) {
	dir := c.MkDir()
	_, err := os.Create(filepath.Join(dir, "bootstrap-params"))
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(IsBootstrapController(dir), tc.Equals, true)
}

func (s *BootstrapSuite) TestIsBootstrapControllerIsFalse(c *tc.C) {
	dir := c.MkDir()
	c.Assert(IsBootstrapController(dir), tc.Equals, false)
}
