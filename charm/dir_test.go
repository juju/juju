package charm_test

import (
	"archive/zip"
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"os"
	"path/filepath"
)

func repoDir(name string) (path string) {
	return filepath.Join("testrepo", "series", name)
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

	var metaf, instf, emptyf, revf *zip.File
	for _, f := range zipr.File {
		c.Logf("Bundled file: %s", f.Name)
		switch f.Name {
		case "revision":
			revf = f
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

	c.Assert(revf, NotNil)
	reader, err := revf.Open()
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(reader)
	reader.Close()
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "1")

	c.Assert(metaf, NotNil)
	reader, err = metaf.Open()
	c.Assert(err, IsNil)
	meta, err := charm.ReadMeta(reader)
	reader.Close()
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

func copyCharmDir(dst, src string) {
	dir, err := charm.ReadDir(src)
	if err != nil {
		panic(err)
	}
	var b bytes.Buffer
	err = dir.BundleTo(&b)
	if err != nil {
		panic(err)
	}
	bundle, err := charm.ReadBundleBytes(b.Bytes())
	if err != nil {
		panic(err)
	}
	err = bundle.ExpandTo(dst)
	if err != nil {
		panic(err)
	}
}

func (s *S) TestDirRevisionFile(c *C) {
	charmDir := c.MkDir()
	copyCharmDir(charmDir, repoDir("dummy"))
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, IsNil)

	dir, err := charm.ReadDir(charmDir)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, 0)

	// Missing revision file with old revision in metadata
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, IsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, IsNil)

	dir, err = charm.ReadDir(charmDir)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, 1234)

	// Revision file with bad content
	err = ioutil.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, IsNil)

	dir, err = charm.ReadDir(charmDir)
	c.Assert(err, Matches, "invalid revision file")
	c.Assert(dir, IsNil)
}

func (s *S) TestDirSetRevision(c *C) {
	dir, err := charm.ReadDir(repoDir("dummy"))
	c.Assert(err, IsNil)

	c.Assert(dir.Revision(), Equals, 1)
	dir.SetRevision(42)
	c.Assert(dir.Revision(), Equals, 42)

	var b bytes.Buffer
	err = dir.BundleTo(&b)
	c.Assert(err, IsNil)

	bundle, err := charm.ReadBundleBytes(b.Bytes())
	c.Assert(bundle.Revision(), Equals, 42)
}

func (s *S) TestDirSetDiskRevision(c *C) {
	charmDir := c.MkDir()
	copyCharmDir(charmDir, repoDir("dummy"))

	dir, err := charm.ReadDir(charmDir)
	c.Assert(err, IsNil)

	c.Assert(dir.Revision(), Equals, 1)
	dir.SetDiskRevision(42)
	c.Assert(dir.Revision(), Equals, 42)

	dir, err = charm.ReadDir(charmDir)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, 42)
}
