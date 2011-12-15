package charm_test

import (
	"archive/zip"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"os"
	"path/filepath"
)

func repoDir(name string) (path string) {
	return filepath.Join("testrepo", name)
}

func (s *S) TestReadDir(c *C) {
	path := repoDir("dummy")
	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	checkDummy(c, dir, path)
}

func (s *S) TestBundleTo(c *C) {
	dir, err := charm.ReadDir(repoDir("dummy"))
	c.Assert(err, IsNil)

	path := filepath.Join(c.MkDir(), "bundle.charm")
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	err = dir.BundleTo(file)
	file.Close()
	c.Assert(err, IsNil)

	zipr, err := zip.OpenReader(path)
	c.Assert(err, IsNil)
	defer zipr.Close()

	var metaf, instf, emptyf *zip.File
	for _, f := range zipr.File {
		c.Logf("Bundled file: %s", f.Name)
		switch f.Name {
		case "metadata.yaml":
			metaf = f
		case "hooks/install":
			instf = f
		case "empty/":
			emptyf = f
		case "build/ignored":
			c.Errorf("bundle includes build/*: %s", f.Name)
		case ".ignored", ".dir/ignored":
			c.Errorf("bundle includes .* entries: %s", f.Name)
		}
	}

	c.Assert(metaf, NotNil)
	reader, err := metaf.Open()
	c.Assert(err, IsNil)
	defer reader.Close()
	meta, err := charm.ReadMeta(reader)
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "dummy")

	c.Assert(instf, NotNil)
	mode, err := instf.Mode()
	c.Assert(err, IsNil)
	c.Assert(mode&0700, Equals, os.FileMode(0700))

	c.Assert(emptyf, NotNil)
	mode, err = emptyf.Mode()
	c.Assert(err, IsNil)
	c.Assert(mode&os.ModeType, Equals, os.ModeDir)
}
