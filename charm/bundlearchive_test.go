// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

var _ = gc.Suite(&BundleArchiveSuite{})

type BundleArchiveSuite struct {
	archivePath string
}

func (s *BundleArchiveSuite) SetUpSuite(c *gc.C) {
	s.archivePath = archivePath(c, readBundleDir(c, "wordpress-simple"))
}

func (s *BundleArchiveSuite) TestReadBundleArchive(c *gc.C) {
	archive, err := charm.ReadBundleArchive(s.archivePath)
	c.Assert(err, gc.IsNil)
	checkWordpressBundle(c, archive, s.archivePath)
}

func (s *BundleArchiveSuite) TestReadBundleArchiveBytes(c *gc.C) {
	data, err := ioutil.ReadFile(s.archivePath)
	c.Assert(err, gc.IsNil)

	archive, err := charm.ReadBundleArchiveBytes(data)
	c.Assert(err, gc.IsNil)
	c.Assert(archive.ContainsOverlays(), jc.IsFalse)
	checkWordpressBundle(c, archive, "")
}

func (s *BundleArchiveSuite) TestReadMultiDocBundleArchiveBytes(c *gc.C) {
	path := archivePath(c, readBundleDir(c, "wordpress-simple-multidoc"))
	data, err := ioutil.ReadFile(path)
	c.Assert(err, gc.IsNil)

	archive, err := charm.ReadBundleArchiveBytes(data)
	c.Assert(err, gc.IsNil)
	c.Assert(archive.ContainsOverlays(), jc.IsTrue)
	checkWordpressBundle(c, archive, "")
}

func (s *BundleArchiveSuite) TestReadBundleArchiveFromReader(c *gc.C) {
	f, err := os.Open(s.archivePath)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	info, err := f.Stat()
	c.Assert(err, gc.IsNil)

	archive, err := charm.ReadBundleArchiveFromReader(f, info.Size())
	c.Assert(err, gc.IsNil)
	checkWordpressBundle(c, archive, "")
}

func (s *BundleArchiveSuite) TestReadBundleArchiveWithoutBundleYAML(c *gc.C) {
	testReadBundleArchiveWithoutFile(c, "bundle.yaml")
}

func (s *BundleArchiveSuite) TestReadBundleArchiveWithoutREADME(c *gc.C) {
	testReadBundleArchiveWithoutFile(c, "README.md")
}

func testReadBundleArchiveWithoutFile(c *gc.C, fileToRemove string) {
	path := cloneDir(c, bundleDirPath(c, "wordpress-simple"))
	dir, err := charm.ReadBundleDir(path)
	c.Assert(err, gc.IsNil)

	// Remove the file from the bundle directory.
	// ArchiveTo just zips the contents of the directory as-is,
	// so the resulting bundle archive not contain the
	// file.
	err = os.Remove(filepath.Join(dir.Path, fileToRemove))
	c.Assert(err, gc.IsNil)

	archivePath := filepath.Join(c.MkDir(), "out.bundle")
	dstf, err := os.Create(archivePath)
	c.Assert(err, gc.IsNil)

	err = dir.ArchiveTo(dstf)
	dstf.Close()

	archive, err := charm.ReadBundleArchive(archivePath)
	// Slightly dubious assumption: the quoted file name has no
	// regexp metacharacters worth worrying about.
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("archive file %q not found", fileToRemove))
	c.Assert(archive, gc.IsNil)
}

func (s *BundleArchiveSuite) TestExpandTo(c *gc.C) {
	dir := c.MkDir()
	archive, err := charm.ReadBundleArchive(s.archivePath)
	c.Assert(err, gc.IsNil)
	err = archive.ExpandTo(dir)
	c.Assert(err, gc.IsNil)
	bdir, err := charm.ReadBundleDir(dir)
	c.Assert(err, gc.IsNil)
	c.Assert(bdir.ReadMe(), gc.Equals, archive.ReadMe())
	c.Assert(bdir.Data(), gc.DeepEquals, archive.Data())
}
