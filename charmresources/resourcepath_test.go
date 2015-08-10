// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type resourcepathSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&resourcepathSuite{})

func (*resourcepathSuite) TestPathFromAttributes(c *gc.C) {
	for i, test := range []struct {
		attrs    charmresources.ResourceAttributes
		expected string
		err      string
	}{{
		attrs: charmresources.ResourceAttributes{},
		err:   "resource path name cannot be empty",
	}, {
		attrs: charmresources.ResourceAttributes{PathName: "path", User: "foo", Org: "bar"},
		err:   "both user and org cannot be specified together",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path"},
		expected: "/blob/base-path",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", Stream: "test"},
		expected: "/blob/c/test/base-path",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", Series: "trusty"},
		expected: "/blob/s/trusty/base-path",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", Revision: "1.2.3"},
		expected: "/blob/base-path/1.2.3",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", Type: "zip"},
		expected: "/zip/base-path",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", User: "fred"},
		expected: "/blob/u/fred/base-path",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", Org: "acme"},
		expected: "/blob/org/acme/base-path",
	}, {
		attrs:    charmresources.ResourceAttributes{PathName: "base-path", User: "fred", Stream: "test", Series: "trusty", Revision: "1.2.3"},
		expected: "/blob/u/fred/c/test/s/trusty/base-path/1.2.3",
	}} {
		c.Logf("test %d", i)
		path, err := charmresources.ResourcePath(test.attrs)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(path, gc.Equals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (*resourcepathSuite) TestAttributesFromPath(c *gc.C) {
	for i, test := range []struct {
		path     string
		expected charmresources.ResourceAttributes
		err      string
	}{{
		path: "",
		err:  `invalid resource path ""`,
	}, {
		path: "/blob/org/bar/u/foo/base-path",
		err:  "both user and org cannot be specified together",
	}, {
		path: "/blob/u/foo/org/bar/base-path", // u and org out-of-order
		err:  `invalid resource path "/blob/u/foo/org/bar/base-path"`,
	}, {
		path:     "/blob/base-path",
		expected: charmresources.ResourceAttributes{Type: "blob", PathName: "base-path"},
	}, {
		path:     "/blob/c/test/base-path",
		expected: charmresources.ResourceAttributes{Type: "blob", PathName: "base-path", Stream: "test"},
	}, {
		path:     "/blob/s/trusty/base-path",
		expected: charmresources.ResourceAttributes{Type: "blob", PathName: "base-path", Series: "trusty"},
	}, {
		path:     "/blob/base-path/1.2.3",
		expected: charmresources.ResourceAttributes{Type: "blob", PathName: "base-path", Revision: "1.2.3"},
	}, {
		path:     "/zip/base-path",
		expected: charmresources.ResourceAttributes{Type: "zip", PathName: "base-path"},
	}, {
		path:     "/blob/u/fred/base-path",
		expected: charmresources.ResourceAttributes{Type: "blob", PathName: "base-path", User: "fred"},
	}, {
		path:     "/blob/org/acme/base-path",
		expected: charmresources.ResourceAttributes{Type: "blob", PathName: "base-path", Org: "acme"},
	}, {
		path: "/blob/u/fred/c/test/s/trusty/base-path/1.2.3",
		expected: charmresources.ResourceAttributes{
			Type: "blob", PathName: "base-path", User: "fred", Stream: "test", Series: "trusty", Revision: "1.2.3",
		},
	}} {
		c.Logf("test %d", i)
		attrs, err := charmresources.ParseResourcePath(test.path)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(attrs, jc.DeepEquals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}
