// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
)

func TestBundleArchiveSuite(t *stdtesting.T) { tc.Run(t, &BundleArchiveSuite{}) }

type BundleArchiveSuite struct {
	archivePath string
}

const bundleName = "wordpress-simple"

func (s *BundleArchiveSuite) SetUpSuite(c *tc.C) {
	s.archivePath = archivePath(c, readBundleDir(c, bundleName))
}

func (s *BundleArchiveSuite) TestReadBundleArchive(c *tc.C) {
	archive, err := charm.ReadBundleArchive(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	checkWordpressBundle(c, archive, s.archivePath, bundleName)
}

func (s *BundleArchiveSuite) TestReadBundleArchiveBytes(c *tc.C) {
	data, err := os.ReadFile(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)

	archive, err := charm.ReadBundleArchiveBytes(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(archive.ContainsOverlays(), tc.IsFalse)
	checkWordpressBundle(c, archive, "", bundleName)
}

func (s *BundleArchiveSuite) TestReadMultiDocBundleArchiveBytes(c *tc.C) {
	path := archivePath(c, readBundleDir(c, "wordpress-simple-multidoc"))
	data, err := os.ReadFile(path)
	c.Assert(err, tc.ErrorIsNil)

	archive, err := charm.ReadBundleArchiveBytes(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(archive.ContainsOverlays(), tc.IsTrue)
	checkWordpressBundle(c, archive, "", "wordpress-simple-multidoc")
}

func (s *BundleArchiveSuite) TestReadBundleArchiveFromReader(c *tc.C) {
	f, err := os.Open(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	defer f.Close()
	info, err := f.Stat()
	c.Assert(err, tc.ErrorIsNil)

	archive, err := charm.ReadBundleArchiveFromReader(f, info.Size())
	c.Assert(err, tc.ErrorIsNil)
	checkWordpressBundle(c, archive, "", bundleName)
}

func (s *BundleArchiveSuite) TestReadBundleArchiveWithoutBundleYAML(c *tc.C) {
	testReadBundleArchiveWithoutFile(c, "bundle.yaml")
}

func (s *BundleArchiveSuite) TestReadBundleArchiveWithoutREADME(c *tc.C) {
	testReadBundleArchiveWithoutFile(c, "README.md")
}

func testReadBundleArchiveWithoutFile(c *tc.C, fileToRemove string) {
	path := cloneDir(c, bundleDirPath(c, "wordpress-simple"))
	dir, err := charmtesting.ReadBundleDir(path)
	c.Assert(err, tc.ErrorIsNil)

	// Remove the file from the bundle directory.
	// ArchiveTo just zips the contents of the directory as-is,
	// so the resulting bundle archive not contain the
	// file.
	err = os.Remove(filepath.Join(dir.Path, fileToRemove))
	c.Assert(err, tc.ErrorIsNil)

	archivePath := filepath.Join(c.MkDir(), "out.bundle")
	dstf, err := os.Create(archivePath)
	c.Assert(err, tc.ErrorIsNil)

	err = dir.ArchiveTo(dstf)
	c.Assert(err, tc.ErrorIsNil)
	dstf.Close()

	archive, err := charm.ReadBundleArchive(archivePath)
	// Slightly dubious assumption: the quoted file name has no
	// regexp metacharacters worth worrying about.
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("archive file %q not found", fileToRemove))
	c.Assert(archive, tc.IsNil)
}

func (s *BundleArchiveSuite) TestExpandTo(c *tc.C) {
	dir := c.MkDir()
	archive, err := charm.ReadBundleArchive(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	err = archive.ExpandTo(dir)
	c.Assert(err, tc.ErrorIsNil)
	bdir, err := charmtesting.ReadBundleDir(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bdir.ReadMe(), tc.Equals, archive.ReadMe())
	c.Assert(bdir.Data(), tc.DeepEquals, archive.Data())
}
