package charm_test

import (
	"archive/zip"
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/charm"
	"launchpad.net/juju-core/juju/testing"
	"os"
	"path/filepath"
	"syscall"
)

type DirSuite struct{}

var _ = Suite(&DirSuite{})

func (s *DirSuite) TestReadDir(c *C) {
	path := testing.Charms.DirPath("dummy")
	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	checkDummy(c, dir, path)
}

func (s *DirSuite) TestReadDirWithoutConfig(c *C) {
	path := testing.Charms.DirPath("varnish")
	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(dir.Config().Options, HasLen, 0)
}

func (s *DirSuite) TestBundleTo(c *C) {
	dir := testing.Charms.Dir("dummy")
	path := filepath.Join(c.MkDir(), "bundle.charm")
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	err = dir.BundleTo(file)
	file.Close()
	c.Assert(err, IsNil)

	zipr, err := zip.OpenReader(path)
	c.Assert(err, IsNil)
	defer zipr.Close()

	var metaf, instf, emptyf, revf, symf *zip.File
	for _, f := range zipr.File {
		c.Logf("Bundled file: %s", f.Name)
		switch f.Name {
		case "revision":
			revf = f
		case "metadata.yaml":
			metaf = f
		case "hooks/install":
			instf = f
		case "hooks/symlink":
			symf = f
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
	// Despite it being 0751, we pack and unpack it as 0755.
	c.Assert(instf.Mode()&0777, Equals, os.FileMode(0755))

	c.Assert(symf, NotNil)
	c.Assert(symf.Mode()&0777, Equals, os.FileMode(0777))
	reader, err = symf.Open()
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(reader)
	reader.Close()
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "../target")

	c.Assert(emptyf, NotNil)
	c.Assert(emptyf.Mode()&os.ModeType, Equals, os.ModeDir)
	// Despite it being 0750, we pack and unpack it as 0755.
	c.Assert(emptyf.Mode()&0777, Equals, os.FileMode(0755))
}

func (s *DirSuite) TestBundleToWithBadType(c *C) {
	charmDir := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
	badFile := filepath.Join(charmDir, "hooks", "badfile")

	// Symlink targeting a path outside of the charm.
	err := os.Symlink("../../target", badFile)
	c.Assert(err, IsNil)

	dir, err := charm.ReadDir(charmDir)
	c.Assert(err, IsNil)

	err = dir.BundleTo(&bytes.Buffer{})
	c.Assert(err, ErrorMatches, `symlink "hooks/badfile" links out of charm: "../../target"`)

	// Symlink targeting an absolute path.
	os.Remove(badFile)
	err = os.Symlink("/target", badFile)
	c.Assert(err, IsNil)

	dir, err = charm.ReadDir(charmDir)
	c.Assert(err, IsNil)

	err = dir.BundleTo(&bytes.Buffer{})
	c.Assert(err, ErrorMatches, `symlink "hooks/badfile" is absolute: "/target"`)

	// Can't bundle special files either.
	os.Remove(badFile)
	err = syscall.Mkfifo(badFile, 0644)
	c.Assert(err, IsNil)

	dir, err = charm.ReadDir(charmDir)
	c.Assert(err, IsNil)

	err = dir.BundleTo(&bytes.Buffer{})
	c.Assert(err, ErrorMatches, `file is a named pipe: "hooks/badfile"`)
}

func (s *DirSuite) TestDirRevisionFile(c *C) {
	charmDir := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
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
	c.Assert(err, ErrorMatches, "invalid revision file")
	c.Assert(dir, IsNil)
}

func (s *DirSuite) TestDirSetRevision(c *C) {
	dir := testing.Charms.ClonedDir(c.MkDir(), "dummy")
	c.Assert(dir.Revision(), Equals, 1)
	dir.SetRevision(42)
	c.Assert(dir.Revision(), Equals, 42)

	var b bytes.Buffer
	err := dir.BundleTo(&b)
	c.Assert(err, IsNil)

	bundle, err := charm.ReadBundleBytes(b.Bytes())
	c.Assert(bundle.Revision(), Equals, 42)
}

func (s *DirSuite) TestDirSetDiskRevision(c *C) {
	charmDir := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
	dir, err := charm.ReadDir(charmDir)
	c.Assert(err, IsNil)

	c.Assert(dir.Revision(), Equals, 1)
	dir.SetDiskRevision(42)
	c.Assert(dir.Revision(), Equals, 42)

	dir, err = charm.ReadDir(charmDir)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, 42)
}
