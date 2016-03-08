// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuversion_test

import (
	"runtime"

	"github.com/juju/version"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

var isDevTests = []struct {
	num version.Number
	dev bool
}{{
	num: version.Number{},
}, {
	num: version.Number{Major: 0, Minor: 0, Patch: 1},
}, {
	num: version.Number{Major: 0, Minor: 0, Patch: 2},
}, {
	num: version.Number{Major: 0, Minor: 1, Patch: 0},
	dev: true,
}, {
	num: version.Number{Major: 0, Minor: 2, Patch: 3},
}, {
	num: version.Number{Major: 1, Minor: 0, Patch: 0},
}, {
	num: version.Number{Major: 10, Minor: 234, Patch: 3456},
}, {
	num: version.Number{Major: 10, Minor: 234, Patch: 3456, Build: 1},
	dev: true,
}, {
	num: version.Number{Major: 10, Minor: 234, Patch: 3456, Build: 64},
	dev: true,
}, {
	num: version.Number{Major: 10, Minor: 235, Patch: 3456},
}, {
	num: version.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha"},
	dev: true,
}, {
	num: version.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha", Build: 1},
	dev: true,
}, {
	num: version.Number{Major: 1, Minor: 21},
}}

func (*suite) TestIsDev(c *gc.C) {
	for i, test := range isDevTests {
		c.Logf("test %d", i)
		c.Check(test.num.IsDev(), gc.Equals, test.dev)
	}
}

func (s *suite) TestCompiler(c *gc.C) {
	c.Assert(version.Compiler, gc.Equals, runtime.Compiler)
}
