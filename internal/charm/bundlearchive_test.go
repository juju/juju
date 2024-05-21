// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm"
)

var _ = gc.Suite(&BundleArchiveSuite{})

type BundleArchiveSuite struct {
	archivePath string
}

const bundleName = "wordpress-simple"

func (s *BundleArchiveSuite) SetUpSuite(c *gc.C) {
	s.archivePath = archivePath(c, readBundleDir(c, bundleName))
}

func (s *BundleArchiveSuite) TestReadBundleArchive(c *gc.C) {
	archive, err := charm.ReadBundleArchive(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)
	checkWordpressBundle(c, archive, s.archivePath, bundleName)
}

func (s *BundleArchiveSuite) TestReadBundleArchiveBytes(c *gc.C) {
	data, err := os.ReadFile(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadBundleArchiveBytes(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archive.ContainsOverlays(), jc.IsFalse)
	checkWordpressBundle(c, archive, "", bundleName)
}

func (s *BundleArchiveSuite) TestReadMultiDocBundleArchiveBytes(c *gc.C) {
	path := archivePath(c, readBundleDir(c, "wordpress-simple-multidoc"))
	data, err := os.ReadFile(path)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadBundleArchiveBytes(data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archive.ContainsOverlays(), jc.IsTrue)
	checkWordpressBundle(c, archive, "", "wordpress-simple-multidoc")
}

func (s *BundleArchiveSuite) TestReadBundleArchiveFromReader(c *gc.C) {
	f, err := os.Open(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	info, err := f.Stat()
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadBundleArchiveFromReader(f, info.Size())
	c.Assert(err, jc.ErrorIsNil)
	checkWordpressBundle(c, archive, "", bundleName)
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
	c.Assert(err, jc.ErrorIsNil)

	// Remove the file from the bundle directory.
	// ArchiveTo just zips the contents of the directory as-is,
	// so the resulting bundle archive not contain the
	// file.
	err = os.Remove(filepath.Join(dir.Path, fileToRemove))
	c.Assert(err, jc.ErrorIsNil)

	archivePath := filepath.Join(c.MkDir(), "out.bundle")
	dstf, err := os.Create(archivePath)
	c.Assert(err, jc.ErrorIsNil)

	err = dir.ArchiveTo(dstf)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	err = archive.ExpandTo(dir)
	c.Assert(err, jc.ErrorIsNil)
	bdir, err := charm.ReadBundleDir(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bdir.ReadMe(), gc.Equals, archive.ReadMe())
	c.Assert(bdir.Data(), gc.DeepEquals, archive.Data())
}
