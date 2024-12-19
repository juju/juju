// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
)

type BundleDirSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BundleDirSuite{})

func (*BundleDirSuite) TestReadBundleDir(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple")
	b, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsFalse)
	checkWordpressBundle(c, b, path, "wordpress-simple")
}

func (*BundleDirSuite) TestReadMultiDocBundleDir(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple-multidoc")
	b, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsTrue)
	checkWordpressBundle(c, b, path, "wordpress-simple-multidoc")
}

func (*BundleDirSuite) TestReadBundleArchive(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple")
	b, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsFalse)
	checkWordpressBundle(c, b, path, "wordpress-simple")
}

func (*BundleDirSuite) TestReadMultiDocBundleArchive(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple-multidoc")
	b, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, gc.IsNil)
	c.Assert(b.ContainsOverlays(), jc.IsTrue)
	checkWordpressBundle(c, b, path, "wordpress-simple-multidoc")
}

func (s *BundleDirSuite) TestReadBundleDirWithoutREADME(c *gc.C) {
	path := cloneDir(c, bundleDirPath(c, "wordpress-simple"))
	err := os.Remove(filepath.Join(path, "README.md"))
	c.Assert(err, gc.IsNil)
	dir, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, gc.ErrorMatches, "cannot read README file: .*")
	c.Assert(dir, gc.IsNil)
}

// bundleDirPath returns the path to the bundle with the
// given name in the testing repository.
func bundleDirPath(c *gc.C, name string) string {
	path := filepath.Join("../internal/test-charm-repo/bundle", name)
	assertIsDir(c, path)
	return path
}

func checkWordpressBundle(c *gc.C, b *charmtesting.BundleDir, path string, bundleName string) {
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
	c.Assert(b.Path, gc.Equals, path)

	bundlePath := filepath.Join("../internal/test-charm-repo/bundle", bundleName, "bundle.yaml")
	raw, err := os.ReadFile(bundlePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b.BundleBytes()), gc.Equals, string(raw))
}

func verifyOk(string) error {
	return nil
}
