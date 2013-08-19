// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
)

type BundleSuite struct {
	repo       *testing.Repo
	bundlePath string
}

var _ = gc.Suite(&BundleSuite{})

func (s *BundleSuite) SetUpSuite(c *gc.C) {
	s.bundlePath = testing.Charms.BundlePath(c.MkDir(), "dummy")
}

func (s *BundleSuite) TestReadBundle(c *gc.C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, gc.IsNil)
	checkDummy(c, bundle, s.bundlePath)
}

func (s *BundleSuite) TestReadBundleWithoutConfig(c *gc.C) {
	path := testing.Charms.BundlePath(c.MkDir(), "varnish")
	bundle, err := charm.ReadBundle(path)
	c.Assert(err, gc.IsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(bundle.Config().Options, gc.HasLen, 0)
}

func (s *BundleSuite) TestReadBundleBytes(c *gc.C) {
	data, err := ioutil.ReadFile(s.bundlePath)
	c.Assert(err, gc.IsNil)

	bundle, err := charm.ReadBundleBytes(data)
	c.Assert(err, gc.IsNil)
	checkDummy(c, bundle, "")
}

func (s *BundleSuite) TestExpandTo(c *gc.C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, gc.IsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, gc.IsNil)

	dir, err := charm.ReadDir(path)
	c.Assert(err, gc.IsNil)
	checkDummy(c, dir, path)
}

func (s *BundleSuite) prepareBundle(c *gc.C, charmDir *charm.Dir, bundlePath string) {
	file, err := os.Create(bundlePath)
	c.Assert(err, gc.IsNil)
	defer file.Close()
	zipw := zip.NewWriter(file)
	defer zipw.Close()

	h := &zip.FileHeader{Name: "revision"}
	h.SetMode(syscall.S_IFREG | 0644)
	w, err := zipw.CreateHeader(h)
	c.Assert(err, gc.IsNil)
	_, err = w.Write([]byte(strconv.Itoa(charmDir.Revision())))

	h = &zip.FileHeader{Name: "metadata.yaml", Method: zip.Deflate}
	h.SetMode(0644)
	w, err = zipw.CreateHeader(h)
	c.Assert(err, gc.IsNil)
	data, err := goyaml.Marshal(charmDir.Meta())
	c.Assert(err, gc.IsNil)
	_, err = w.Write(data)
	c.Assert(err, gc.IsNil)

	for name := range charmDir.Meta().Hooks() {
		hookName := filepath.Join("hooks", name)
		h = &zip.FileHeader{
			Name:   hookName,
			Method: zip.Deflate,
		}
		// Force it non-executable
		h.SetMode(0644)
		w, err := zipw.CreateHeader(h)
		c.Assert(err, gc.IsNil)
		_, err = w.Write([]byte("not important"))
		c.Assert(err, gc.IsNil)
	}
}

func (s *BundleSuite) TestExpandToSetsHooksExecutable(c *gc.C) {
	charmDir := testing.Charms.ClonedDir(c.MkDir(), "all-hooks")
	// Bundle manually, so we can check ExpandTo(), unaffected
	// by BundleTo()'s behavior
	bundlePath := filepath.Join(c.MkDir(), "bundle.charm")
	s.prepareBundle(c, charmDir, bundlePath)
	bundle, err := charm.ReadBundle(bundlePath)
	c.Assert(err, gc.IsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, gc.IsNil)

	_, err = charm.ReadDir(path)
	c.Assert(err, gc.IsNil)

	for name := range bundle.Meta().Hooks() {
		hookName := string(name)
		info, err := os.Stat(filepath.Join(path, "hooks", hookName))
		c.Assert(err, gc.IsNil)
		perm := info.Mode() & 0777
		c.Assert(perm&0100 != 0, gc.Equals, true, gc.Commentf("hook %q is not executable", hookName))
	}
}

func (s *BundleSuite) TestBundleFileModes(c *gc.C) {
	// Apply subtler mode differences than can be expressed in Bazaar.
	srcPath := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
	modes := []struct {
		path string
		mode os.FileMode
	}{
		{"hooks/install", 0751},
		{"empty", 0750},
		{"src/hello.c", 0614},
	}
	for _, m := range modes {
		err := os.Chmod(filepath.Join(srcPath, m.path), m.mode)
		c.Assert(err, gc.IsNil)
	}
	var haveSymlinks = true
	if err := os.Symlink("../target", filepath.Join(srcPath, "hooks/symlink")); err != nil {
		haveSymlinks = false
	}

	// Bundle and extract the charm to a new directory.
	dir, err := charm.ReadDir(srcPath)
	c.Assert(err, gc.IsNil)
	buf := new(bytes.Buffer)
	err = dir.BundleTo(buf)
	c.Assert(err, gc.IsNil)
	bundle, err := charm.ReadBundleBytes(buf.Bytes())
	c.Assert(err, gc.IsNil)
	path := c.MkDir()
	err = bundle.ExpandTo(path)
	c.Assert(err, gc.IsNil)

	// Check sensible file modes once round-tripped.
	info, err := os.Stat(filepath.Join(path, "src", "hello.c"))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode()&0777, gc.Equals, os.FileMode(0644))
	c.Assert(info.Mode()&os.ModeType, gc.Equals, os.FileMode(0))

	info, err = os.Stat(filepath.Join(path, "hooks", "install"))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode()&0777, gc.Equals, os.FileMode(0755))
	c.Assert(info.Mode()&os.ModeType, gc.Equals, os.FileMode(0))

	info, err = os.Stat(filepath.Join(path, "empty"))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode()&0777, gc.Equals, os.FileMode(0755))

	if haveSymlinks {
		target, err := os.Readlink(filepath.Join(path, "hooks", "symlink"))
		c.Assert(err, gc.IsNil)
		c.Assert(target, gc.Equals, "../target")
	}
}

func (s *BundleSuite) TestBundleRevisionFile(c *gc.C) {
	charmDir := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, gc.IsNil)

	bundle, err := charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, gc.IsNil)
	c.Assert(bundle.Revision(), gc.Equals, 0)

	// Missing revision file with old revision in metadata
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, gc.IsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, gc.IsNil)

	bundle, err = charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, gc.IsNil)
	c.Assert(bundle.Revision(), gc.Equals, 1234)

	// Revision file with bad content
	err = ioutil.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, gc.IsNil)

	bundle, err = charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, gc.ErrorMatches, "invalid revision file")
	c.Assert(bundle, gc.IsNil)
}

func (s *BundleSuite) TestBundleSetRevision(c *gc.C) {
	bundle, err := charm.ReadBundle(s.bundlePath)
	c.Assert(err, gc.IsNil)

	c.Assert(bundle.Revision(), gc.Equals, 1)
	bundle.SetRevision(42)
	c.Assert(bundle.Revision(), gc.Equals, 42)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, gc.IsNil)

	dir, err := charm.ReadDir(path)
	c.Assert(err, gc.IsNil)
	c.Assert(dir.Revision(), gc.Equals, 42)
}

func (s *BundleSuite) TestExpandToWithBadLink(c *gc.C) {
	charmDir := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
	badLink := filepath.Join(charmDir, "hooks", "badlink")

	// Symlink targeting a path outside of the charm.
	err := os.Symlink("../../target", badLink)
	c.Assert(err, gc.IsNil)

	bundle, err := charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, gc.IsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, gc.ErrorMatches, `symlink "hooks/badlink" links out of charm: "../../target"`)

	// Symlink targeting an absolute path.
	os.Remove(badLink)
	err = os.Symlink("/target", badLink)
	c.Assert(err, gc.IsNil)

	bundle, err = charm.ReadBundle(extBundleDir(c, charmDir))
	c.Assert(err, gc.IsNil)

	path = filepath.Join(c.MkDir(), "charm")
	err = bundle.ExpandTo(path)
	c.Assert(err, gc.ErrorMatches, `symlink "hooks/badlink" is absolute: "/target"`)
}

func extBundleDir(c *gc.C, dirpath string) (path string) {
	path = filepath.Join(c.MkDir(), "bundle.charm")
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %s; zip --fifo --symlinks -r %s .", dirpath, path))
	output, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil, gc.Commentf("Command output: %s", output))
	return path
}
