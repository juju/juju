package formula_test

import (
	//"archive/zip"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"os"
	"path/filepath"
)

func bundleDir(c *C, dirpath string) (path string) {
	dir, err := formula.ReadDir(dirpath)
	c.Assert(err, IsNil)

	path = filepath.Join(c.MkDir(), "bundle.charm")

	file, err := os.Create(path)
	c.Assert(err, IsNil)

	err = dir.BundleTo(file)
	c.Assert(err, IsNil)
	file.Close()

	return path
}

func (s *S) TestReadBundle(c *C) {
	path := bundleDir(c, repoDir("dummy"))

	bundle, err := formula.ReadBundle(path)
	c.Assert(err, IsNil)
	c.Assert(bundle.Path, Equals, path)
	c.Assert(bundle.Meta().Name, Equals, "dummy")
	c.Assert(bundle.Config().Options["title"].Default, Equals, "My Title")
}

func (s *S) TestReadBundleBytes(c *C) {
	path := bundleDir(c, repoDir("dummy"))

	data, err := ioutil.ReadFile(path)
	c.Assert(err, IsNil)

	bundle, err := formula.ReadBundleBytes(data)
	c.Assert(err, IsNil)
	c.Assert(bundle.Path, Equals, "")
	c.Assert(bundle.Meta().Name, Equals, "dummy")
	c.Assert(bundle.Config().Options["title"].Default, Equals, "My Title")
}
