// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

var _ = gc.Suite(&BundleSuite{})

type BundleSuite struct {
	testing.IsolationSuite
}

func (*BundleSuite) TestReadBundleDir(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple")
	b, err := charm.ReadBundle(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsFalse)
	c.Assert(b, gc.FitsTypeOf, (*charm.BundleDir)(nil))
	checkWordpressBundle(c, b, path)
}

func (*BundleSuite) TestReadMultiDocBundleDir(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple-multidoc")
	b, err := charm.ReadBundle(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsTrue)
	c.Assert(b, gc.FitsTypeOf, (*charm.BundleDir)(nil))
	checkWordpressBundle(c, b, path)
}

func (*BundleSuite) TestReadBundleArchive(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple")
	b, err := charm.ReadBundle(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsFalse)
	c.Assert(b, gc.FitsTypeOf, (*charm.BundleDir)(nil))
	checkWordpressBundle(c, b, path)
}

func (*BundleSuite) TestReadMultiDocBundleArchive(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple-multidoc")
	b, err := charm.ReadBundle(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsTrue)
	c.Assert(b, gc.FitsTypeOf, (*charm.BundleDir)(nil))
	checkWordpressBundle(c, b, path)
}

func checkWordpressBundle(c *gc.C, b charm.Bundle, path string) {
	// Load the charms required by the bundle.
	wordpressCharm := readCharmDir(c, "wordpress")
	mysqlCharm := readCharmDir(c, "mysql")

	bd := b.Data()
	c.Assert(bd.RequiredCharms(), jc.DeepEquals, []string{"ch:mysql", "ch:wordpress"})

	charms := map[string]charm.Charm{
		"ch:wordpress": wordpressCharm,
		"ch:mysql":     mysqlCharm,
	}
	err := bd.VerifyWithCharms(verifyOk, nil, nil, charms)
	c.Assert(err, gc.IsNil)

	c.Assert(bd.Applications, jc.DeepEquals, map[string]*charm.ApplicationSpec{
		"wordpress": {
			Charm: "ch:wordpress",
		},
		"mysql": {
			Charm:    "ch:mysql",
			NumUnits: 1,
		},
	})
	c.Assert(bd.Relations, jc.DeepEquals, [][]string{
		{"wordpress:db", "mysql:server"},
	})
	c.Assert(b.ReadMe(), gc.Equals, "A dummy bundle\n")
	switch b := b.(type) {
	case *charm.BundleArchive:
		c.Assert(b.Path, gc.Equals, path)
	case *charm.BundleDir:
		c.Assert(b.Path, gc.Equals, path)
	}
}

func verifyOk(string) error {
	return nil
}
