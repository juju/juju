// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) TestBasePath(c *gc.C) {
	config := Config{
		URL:     "http://api.foo.bar.com",
		Version: "v2",
		Entity:  "meshuggah",
	}
	path, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path.String(), gc.Equals, "http://api.foo.bar.com/v2/meshuggah")
}

func (s *ConfigSuite) TestBasePathWithTrailingSlash(c *gc.C) {
	config := Config{
		URL:     "http://api.foo.bar.com/",
		Version: "v2",
		Entity:  "meshuggah",
	}
	path, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path.String(), gc.Equals, "http://api.foo.bar.com/v2/meshuggah")
}
