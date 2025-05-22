// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type CharmArchiveSuite struct {
	testhelpers.IsolationSuite
	archivePath string
}

func TestCharmArchiveSuite(t *stdtesting.T) {
	tc.Run(t, &CharmArchiveSuite{})
}

func (s *CharmArchiveSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.archivePath = archivePath(c, readCharmDir(c, "dummy"))
}

var dummyArchiveMembersCommon = []string{
	"config.yaml",
	"empty",
	"empty/.gitkeep",
	"hooks",
	"hooks/install",
	"lxd-profile.yaml",
	"manifest.yaml",
	"metadata.yaml",
	"revision",
	"src",
	"src/hello.c",
	".notignored",
	"build",
	"build/ignored",
}

var dummyArchiveMembers = append(dummyArchiveMembersCommon, "actions.yaml")
var dummyArchiveMembersActions = append(dummyArchiveMembersCommon, []string{
	"actions.yaml",
	"actions/snapshot",
	"actions",
}...)

func (s *CharmArchiveSuite) TestReadCharmArchive(c *tc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	checkDummy(c, archive)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithoutConfig(c *tc.C) {
	// Technically varnish has no config AND no actions.
	// Perhaps we should make this more orthogonal?
	path := archivePath(c, readCharmDir(c, "varnish"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(archive.Config().Options, tc.HasLen, 0)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveManifest(c *tc.C) {
	path := archivePath(c, readCharmDir(c, "dummy"))
	dir, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(dir.Manifest().Bases, tc.DeepEquals, []charm.Base{{
		Name: "ubuntu",
		Channel: charm.Channel{

			Track: "18.04",
			Risk:  "stable",
		},
	}, {
		Name: "ubuntu",
		Channel: charm.Channel{
			Track: "20.04",
			Risk:  "stable",
		},
	}})
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithoutActions(c *tc.C) {
	// Wordpress has config but no actions.
	path := archivePath(c, readCharmDir(c, "wordpress"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)

	// A lacking actions.yaml file still causes a proper
	// Actions value to be returned.
	c.Assert(archive.Actions().ActionSpecs, tc.HasLen, 0)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithActions(c *tc.C) {
	path := archivePath(c, readCharmDir(c, "dummy-actions"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(archive.Actions().ActionSpecs, tc.HasLen, 1)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithJujuActions(c *tc.C) {
	path := archivePath(c, readCharmDir(c, "juju-charm"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(archive.Actions().ActionSpecs, tc.HasLen, 1)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveBytes(c *tc.C) {
	data, err := os.ReadFile(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(data)
	c.Assert(err, tc.ErrorIsNil)
	checkDummy(c, archive)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveFromReader(c *tc.C) {
	f, err := os.Open(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	c.Assert(err, tc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveFromReader(f, info.Size())
	c.Assert(err, tc.ErrorIsNil)
	checkDummy(c, archive)
}

func (s *CharmArchiveSuite) TestArchiveMembers(c *tc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manifest, tc.DeepEquals, set.NewStrings(dummyArchiveMembers...))
}

func (s *CharmArchiveSuite) TestArchiveMembersActions(c *tc.C) {
	path := archivePath(c, readCharmDir(c, "dummy-actions"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manifest, tc.DeepEquals, set.NewStrings(dummyArchiveMembersActions...))
}

func (s *CharmArchiveSuite) TestArchiveMembersNoRevision(c *tc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)
	dirPath := c.MkDir()
	err = archive.ExpandTo(dirPath)
	c.Assert(err, tc.ErrorIsNil)
	err = os.Remove(filepath.Join(dirPath, "revision"))
	c.Assert(err, tc.ErrorIsNil)

	archive = extCharmArchiveDir(c, dirPath)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manifest, tc.DeepEquals, set.NewStrings(dummyArchiveMembers...))
}

func (s *CharmArchiveSuite) TestArchiveMembersSymlink(c *tc.C) {
	srcPath := cloneDir(c, charmDirPath(c, "dummy"))
	if err := os.Symlink("../target", filepath.Join(srcPath, "hooks/symlink")); err != nil {
		c.Skip("cannot symlink")
	}
	expected := append([]string{"hooks/symlink"}, dummyArchiveMembers...)

	archive := archiveDir(c, srcPath)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(manifest, tc.DeepEquals, set.NewStrings(expected...))
}

func (s *CharmArchiveSuite) TestExpandTo(c *tc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, tc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, tc.ErrorIsNil)

	dir, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)
	checkDummy(c, dir)
}

func (s *CharmArchiveSuite) TestExpandToSetsHooksExecutable(c *tc.C) {
	archivePath := archivePath(c, readCharmDir(c, "all-hooks"))

	chArchive, err := charm.ReadCharmArchive(archivePath)
	c.Assert(err, tc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = chArchive.ExpandTo(path)
	c.Assert(err, tc.ErrorIsNil)

	_, err = charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)

	for name := range chArchive.Meta().Hooks() {
		info, err := os.Stat(filepath.Join(path, "hooks", name))
		if _, ok := err.(*os.PathError); ok {
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		perm := info.Mode() & 0777
		c.Assert(perm&0100 != 0, tc.Equals, true, tc.Commentf("hook %q is not executable", name))
	}
}

func (s *CharmArchiveSuite) TestCharmArchiveFileModes(c *tc.C) {
	// Apply subtler mode differences than can be expressed in Bazaar.
	srcPath := cloneDir(c, charmDirPath(c, "dummy"))
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
		c.Assert(err, tc.ErrorIsNil)
	}
	var haveSymlinks = true
	if err := os.Symlink("../target", filepath.Join(srcPath, "hooks/symlink")); err != nil {
		haveSymlinks = false
	}

	// CharmArchive and extract the charm to a new directory.
	archive := archiveDir(c, srcPath)
	path := c.MkDir()
	err := archive.ExpandTo(path)
	c.Assert(err, tc.ErrorIsNil)

	// Check sensible file modes once round-tripped.
	info, err := os.Stat(filepath.Join(path, "src", "hello.c"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Mode()&0777, tc.Equals, os.FileMode(0644))
	c.Assert(info.Mode()&os.ModeType, tc.Equals, os.FileMode(0))

	info, err = os.Stat(filepath.Join(path, "hooks", "install"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Mode()&0777, tc.Equals, os.FileMode(0755))
	c.Assert(info.Mode()&os.ModeType, tc.Equals, os.FileMode(0))

	info, err = os.Stat(filepath.Join(path, "empty"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Mode()&0777, tc.Equals, os.FileMode(0755))

	if haveSymlinks {
		target, err := os.Readlink(filepath.Join(path, "hooks", "symlink"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(target, tc.Equals, "../target")
	}
}

func (s *CharmArchiveSuite) TestCharmArchiveRevisionFile(c *tc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, tc.ErrorIsNil)

	archive := extCharmArchiveDir(c, charmDir)
	c.Assert(archive.Revision(), tc.Equals, 0)

	// Missing revision file with obsolete old revision in metadata;
	// the revision is ignored.
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, tc.ErrorIsNil)

	archive = extCharmArchiveDir(c, charmDir)
	c.Assert(archive.Revision(), tc.Equals, 0)

	// Revision file with bad content
	err = os.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, tc.ErrorIsNil)

	path := extCharmArchiveDirPath(c, charmDir)
	archive, err = charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorMatches, "invalid revision file")
	c.Assert(archive, tc.IsNil)
}

func (s *CharmArchiveSuite) TestExpandToWithBadLink(c *tc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	badLink := filepath.Join(charmDir, "hooks", "badlink")

	// Symlink targeting a path outside of the charm.
	err := os.Symlink("../../target", badLink)
	c.Assert(err, tc.ErrorIsNil)

	archive := extCharmArchiveDir(c, charmDir)
	c.Assert(err, tc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, tc.ErrorMatches, `cannot extract "hooks/badlink": symlink "../../target" leads out of scope`)

	// Symlink targeting an absolute path.
	os.Remove(badLink)
	err = os.Symlink("/target", badLink)
	c.Assert(err, tc.ErrorIsNil)

	archive = extCharmArchiveDir(c, charmDir)
	c.Assert(err, tc.ErrorIsNil)

	path = filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, tc.ErrorMatches, `cannot extract "hooks/badlink": symlink "/target" is absolute`)
}

func extCharmArchiveDirPath(c *tc.C, dirpath string) string {
	path := filepath.Join(c.MkDir(), "archive.charm")
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %s; zip --fifo --symlinks -r %s .", dirpath, path))
	output, err := cmd.CombinedOutput()
	c.Assert(err, tc.IsNil, tc.Commentf("Command output: %s", output))
	return path
}

func extCharmArchiveDir(c *tc.C, dirpath string) *charm.CharmArchive {
	path := extCharmArchiveDirPath(c, dirpath)
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, tc.ErrorIsNil)
	return archive
}

func archiveDir(c *tc.C, dirpath string) *charm.CharmArchive {
	dir, err := charmtesting.ReadCharmDir(dirpath)
	c.Assert(err, tc.ErrorIsNil)

	buf := new(bytes.Buffer)
	err = dir.ArchiveTo(buf)
	c.Assert(err, tc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(buf.Bytes())
	c.Assert(err, tc.ErrorIsNil)

	return archive
}
