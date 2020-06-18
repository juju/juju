// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type PathSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PathSuite{})

func (s *PathSuite) TestJoin(c *gc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path/")

	path := MakePath(rawURL)
	appPath, err := path.Join("entity", "app")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(appPath.String(), gc.Equals, "http://foobar/v1/path/entity/app")
}

func (s *PathSuite) TestJoinMultipleTimes(c *gc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path/")

	path := MakePath(rawURL)
	entityPath, err := path.Join("entity")
	c.Assert(err, jc.ErrorIsNil)

	appPath, err := entityPath.Join("app")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(appPath.String(), gc.Equals, "http://foobar/v1/path/entity/app")
}

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
