package charm_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"os"
	"os/exec"
	"path/filepath"
)

type BundleSuite struct {
	bundlePath string
}

var _ = Suite(&BundleSuite{})

func (s *BundleSuite) SetUpSuite(c *C) {
	s.bundlePath = bundleDir(c, repoDir("dummy"))
}

func (s *BundleSuite) TestReadBundle(c *C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, IsNil)
	checkDummy(c, bundle, s.bundlePath)
}

func (s *BundleSuite) TestReadBundleWithoutConfig(c *C) {
	path := bundleDir(c, repoDir("varnish"))
	bundle, err := charm.ReadBundle(path)
	c.Assert(err, IsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(len(bundle.Config().Options), Equals, 0)
}

func (s *BundleSuite) TestReadBundleBytes(c *C) {
	data, err := ioutil.ReadFile(s.bundlePath)
	c.Assert(err, IsNil)

	bundle, err := charm.ReadBundleBytes(data)
	c.Assert(err, IsNil)
	checkDummy(c, bundle, "")
}

func (s *BundleSuite) TestExpandTo(c *C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, IsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, IsNil)

	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	checkDummy(c, dir, path)
}

func (s *BundleSuite) TestBundleRevisionFile(c *C) {
	charmDir := c.MkDir()
	copyCharmDir(charmDir, repoDir("dummy"))
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, IsNil)

	bundle, err := charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, IsNil)
	c.Assert(bundle.Revision(), Equals, 0)

	// Missing revision file with old revision in metadata
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, IsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, IsNil)

	bundle, err = charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, IsNil)
	c.Assert(bundle.Revision(), Equals, 1234)

	// Revision file with bad content
	err = ioutil.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, IsNil)

	bundle, err = charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, ErrorMatches, "invalid revision file")
	c.Assert(bundle, IsNil)
}

func (s *BundleSuite) TestBundleSetRevision(c *C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, IsNil)

	c.Assert(bundle.Revision(), Equals, 1)
	bundle.SetRevision(42)
	c.Assert(bundle.Revision(), Equals, 42)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, IsNil)

	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, 42)
}

func (s *BundleSuite) TestExpandToWithBadLink(c *C) {
	charmDir := c.MkDir()
	copyCharmDir(charmDir, repoDir("dummy"))
	badLink := filepath.Join(charmDir, "hooks", "badlink")

	// Symlink targetting a path outside of the charm.
	err := os.Symlink("../../target", badLink)
	c.Assert(err, IsNil)

	bundle, err := charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, IsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, ErrorMatches, `symlink "hooks/badlink" links out of charm: "../../target"`)

	// Symlink targetting an absolute path.
	os.Remove(badLink)
	err = os.Symlink("/target", badLink)
	c.Assert(err, IsNil)

	bundle, err = charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, IsNil)

	path = filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, ErrorMatches, `symlink "hooks/badlink" is absolute: "/target"`)
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

func extBundleDir(c *C, dirpath string) (path string) {
	path = filepath.Join(c.MkDir(), "bundle.charm")
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %s; zip --fifo --symlinks -r %s .", dirpath, path))
	output, err := cmd.CombinedOutput()
	c.Assert(err, IsNil, Bug("Command output: %s", output))
	return path
}
