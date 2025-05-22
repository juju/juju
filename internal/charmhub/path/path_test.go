// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package path

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type PathSuite struct {
	testhelpers.IsolationSuite
}

func TestPathSuite(t *stdtesting.T) {
	tc.Run(t, &PathSuite{})
}

func (s *PathSuite) TestJoin(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path/")

	path := MakePath(rawURL)
	appPath, err := path.Join("entity", "app")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(appPath.String(), tc.Equals, "http://foobar/v1/path/entity/app")
}

func (s *PathSuite) TestJoinMultipleTimes(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path/")

	path := MakePath(rawURL)
	entityPath, err := path.Join("entity")
	c.Assert(err, tc.ErrorIsNil)

	appPath, err := entityPath.Join("app")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(appPath.String(), tc.Equals, "http://foobar/v1/path/entity/app")
}

func (s *PathSuite) TestQuery(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path")

	path := MakePath(rawURL)

	newPath, err := path.Query("q", "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path.String(), tc.Equals, "http://foobar/v1/path")
	c.Assert(newPath.String(), tc.Equals, "http://foobar/v1/path?q=foo")
}

func (s *PathSuite) TestQueryEmptyValue(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path")

	path := MakePath(rawURL)

	newPath, err := path.Query("q", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path.String(), tc.Equals, newPath.String())
	c.Assert(newPath.String(), tc.Equals, "http://foobar/v1/path")
}

func (s *PathSuite) TestJoinQuery(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path")

	path := MakePath(rawURL)
	entityPath, err := path.Join("entity")
	c.Assert(err, tc.ErrorIsNil)

	appPath, err := entityPath.Join("app")
	c.Assert(err, tc.ErrorIsNil)

	newPath, err := appPath.Query("q", "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(appPath.String(), tc.Equals, "http://foobar/v1/path/entity/app")
	c.Assert(newPath.String(), tc.Equals, "http://foobar/v1/path/entity/app?q=foo")
}

func (s *PathSuite) TestMultipleQueries(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path")

	path := MakePath(rawURL)

	newPath, err := path.Query("q", "foo1")
	c.Assert(err, tc.ErrorIsNil)

	newPath, err = newPath.Query("q", "foo2")
	c.Assert(err, tc.ErrorIsNil)

	newPath, err = newPath.Query("x", "bar")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(path.String(), tc.Equals, "http://foobar/v1/path")
	c.Assert(newPath.String(), tc.Equals, "http://foobar/v1/path?q=foo1&q=foo2&x=bar")
}

func (s *PathSuite) TestQueries(c *tc.C) {
	rawURL := MustParseURL(c, "http://foobar/v1/path")

	path := MakePath(rawURL)

	newPath0, err := path.Query("a", "foo1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newPath0.String(), tc.Equals, "http://foobar/v1/path?a=foo1")

	newPath1, err := newPath0.Query("b", "foo2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newPath1.String(), tc.Equals, "http://foobar/v1/path?a=foo1&b=foo2")

	newPath1, err = newPath1.Query("c", "foo3")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newPath1.String(), tc.Equals, "http://foobar/v1/path?a=foo1&b=foo2&c=foo3")
}
