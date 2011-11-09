package charm_test

import (
	//"archive/zip"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"os"
	"path/filepath"
)

type BundleSuite struct {
	bundlePath string
}

var _ = Suite(&BundleSuite{})

func (s *BundleSuite) SetUpSuite(c *C) {
	s.bundlePath = bundleDir(c, repoDir("dummy"))
}

func (s BundleSuite) TestReadBundle(c *C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, IsNil)
	checkDummy(c, bundle, s.bundlePath)
}

func (s BundleSuite) TestReadBundleBytes(c *C) {
	data, err := ioutil.ReadFile(s.bundlePath)
	c.Assert(err, IsNil)

	bundle, err := charm.ReadBundleBytes(data)
	c.Assert(err, IsNil)
	checkDummy(c, bundle, "")
}

func (s BundleSuite) TestExpandTo(c *C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	path := filepath.Join(c.MkDir(), "charm")

	err = bundle.ExpandTo(path)
	c.Assert(err, IsNil)

	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	checkDummy(c, dir, path)

}

func bundleDir(c *C, dirpath string) (path string) {
	dir, err := charm.ReadDir(dirpath)
	c.Assert(err, IsNil)

	path = filepath.Join(c.MkDir(), "bundle.charm")

	file, err := os.Create(path)
	c.Assert(err, IsNil)

	err = dir.BundleTo(file)
	c.Assert(err, IsNil)
	file.Close()

	return path
}
