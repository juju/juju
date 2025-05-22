// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestConfigSuite(t *testing.T) {
	tc.Run(t, &ConfigSuite{})
}

func (s *ConfigSuite) TestBasePath(c *tc.C) {
	path, err := basePath("http://api.foo.bar.com")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path.String(), tc.Equals, "http://api.foo.bar.com/v2/charms")
}

func (s *ConfigSuite) TestBasePathWithTrailingSlash(c *tc.C) {
	path, err := basePath("http://api.foo.bar.com/")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path.String(), tc.Equals, "http://api.foo.bar.com/v2/charms")
}
