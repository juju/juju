// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type ConfigSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ConfigSuite{})

func (s *ConfigSuite) TestBasePath(c *tc.C) {
	path, err := basePath("http://api.foo.bar.com")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path.String(), tc.Equals, "http://api.foo.bar.com/v2/charms")
}

func (s *ConfigSuite) TestBasePathWithTrailingSlash(c *tc.C) {
	path, err := basePath("http://api.foo.bar.com/")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path.String(), tc.Equals, "http://api.foo.bar.com/v2/charms")
}
