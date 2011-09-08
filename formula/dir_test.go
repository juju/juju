package formula_test

import (
	"archive/zip"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"path/filepath"
)

func repoDir(name string) (path string) {
	return filepath.Join("testrepo", name)
}

func (s *S) TestReadDir(c *C) {
	path := repoDir("dummy")
	dir, err := formula.ReadDir(path)
	c.Assert(err, IsNil)
	c.Assert(dir.Path(), Equals, path)
	c.Assert(dir.Meta().Name, Equals, "dummy")
	c.Assert(dir.Config().Options["title"].Default, Equals, "My Title")
	c.Assert(dir.IsExpanded(), Equals, true)
}

func (s *S) TestBundleTo(c *C) {
	tmpdir := c.MkDir()
	dir, err := formula.ReadDir(repoDir("dummy"))
	c.Assert(err, IsNil)

	path := filepath.Join(tmpdir, "bundle.charm")
	err = dir.BundleTo(path)
	c.Assert(err, IsNil)

	zipr, err := zip.OpenReader(path)
	c.Assert(err, IsNil)
	defer zipr.Close()

	var metaf *zip.File
	for _, f := range zipr.File {
		c.Logf("Bundled file: %s", f.Name)
		switch f.Name {
		case "metadata.yaml":
			metaf = f
		case "build/ignored":
			c.Fatal("bundle includes build/*")
		}
	}

	c.Assert(metaf, NotNil)
	file, err := metaf.Open()
	c.Assert(err, IsNil)
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)

	meta, err := formula.ParseMeta(data)
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "dummy")
}
