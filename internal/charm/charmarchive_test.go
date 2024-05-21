// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm"
)

type CharmArchiveSuite struct {
	testing.IsolationSuite
	archivePath string
}

var _ = gc.Suite(&CharmArchiveSuite{})

func (s *CharmArchiveSuite) SetUpSuite(c *gc.C) {
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
}

var dummyArchiveMembers = append(dummyArchiveMembersCommon, "actions.yaml")
var dummyArchiveMembersActions = append(dummyArchiveMembersCommon, []string{
	"actions.yaml",
	"actions/snapshot",
	"actions",
}...)

func (s *CharmArchiveSuite) TestReadCharmArchive(c *gc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)
	checkDummy(c, archive, s.archivePath)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithoutConfig(c *gc.C) {
	// Technically varnish has no config AND no actions.
	// Perhaps we should make this more orthogonal?
	path := archivePath(c, readCharmDir(c, "varnish"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(archive.Config().Options, gc.HasLen, 0)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveManifest(c *gc.C) {
	path := archivePath(c, readCharmDir(c, "dummy"))
	dir, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(dir.Manifest().Bases, gc.DeepEquals, []charm.Base{{
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

func (s *CharmArchiveSuite) TestReadCharmArchiveWithoutManifest(c *gc.C) {
	path := archivePath(c, readCharmDir(c, "mysql"))
	dir, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Manifest(), gc.IsNil)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithoutActions(c *gc.C) {
	// Wordpress has config but no actions.
	path := archivePath(c, readCharmDir(c, "wordpress"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)

	// A lacking actions.yaml file still causes a proper
	// Actions value to be returned.
	c.Assert(archive.Actions().ActionSpecs, gc.HasLen, 0)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithActions(c *gc.C) {
	path := archivePath(c, readCharmDir(c, "dummy-actions"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archive.Actions().ActionSpecs, gc.HasLen, 1)
}

func (s *CharmDirSuite) TestReadCharmArchiveWithJujuActions(c *gc.C) {
	path := archivePath(c, readCharmDir(c, "juju-charm"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archive.Actions().ActionSpecs, gc.HasLen, 1)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveBytes(c *gc.C) {
	data, err := os.ReadFile(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(data)
	c.Assert(err, jc.ErrorIsNil)
	checkDummy(c, archive, "")
}

func (s *CharmArchiveSuite) TestReadCharmArchiveFromReader(c *gc.C) {
	f, err := os.Open(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveFromReader(f, info.Size())
	c.Assert(err, jc.ErrorIsNil)
	checkDummy(c, archive, "")
}

func (s *CharmArchiveSuite) TestArchiveMembers(c *gc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifest, jc.DeepEquals, set.NewStrings(dummyArchiveMembers...))
}

func (s *CharmArchiveSuite) TestArchiveMembersActions(c *gc.C) {
	path := archivePath(c, readCharmDir(c, "dummy-actions"))
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifest, jc.DeepEquals, set.NewStrings(dummyArchiveMembersActions...))
}

func (s *CharmArchiveSuite) TestArchiveMembersNoRevision(c *gc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)
	dirPath := c.MkDir()
	err = archive.ExpandTo(dirPath)
	c.Assert(err, jc.ErrorIsNil)
	err = os.Remove(filepath.Join(dirPath, "revision"))
	c.Assert(err, jc.ErrorIsNil)

	archive = extCharmArchiveDir(c, dirPath)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifest, gc.DeepEquals, set.NewStrings(dummyArchiveMembers...))
}

func (s *CharmArchiveSuite) TestArchiveMembersSymlink(c *gc.C) {
	srcPath := cloneDir(c, charmDirPath(c, "dummy"))
	if err := os.Symlink("../target", filepath.Join(srcPath, "hooks/symlink")); err != nil {
		c.Skip("cannot symlink")
	}
	expected := append([]string{"hooks/symlink"}, dummyArchiveMembers...)

	archive := archiveDir(c, srcPath)
	manifest, err := archive.ArchiveMembers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifest, gc.DeepEquals, set.NewStrings(expected...))
}

func (s *CharmArchiveSuite) TestExpandTo(c *gc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	checkDummy(c, dir, path)
}

func (s *CharmArchiveSuite) TestReadCharmArchiveWithVersion(c *gc.C) {
	clonedPath := cloneDir(c, charmDirPath(c, "versioned"))
	_, err := os.Create(filepath.Join(clonedPath, ".git"))
	c.Assert(err, jc.ErrorIsNil)

	// NOTE(achilleasa) Initially, I tried using PatchExecutableAsEchoArgs
	// but it doesn't work as expected on my bionic box so I reverted to
	// the following less elegant approach to stubbing git output.
	var gitOutput string
	switch runtime.GOOS {
	case "windows":
		gitOutput = "@echo off\r\necho c0ffee"
	default:
		gitOutput = "#!/bin/bash -norc\necho c0ffee"
	}
	testing.PatchExecutable(c, s, "git", gitOutput)

	// Read cloned path and archive it; the archive should now include
	// the version fetched from our mocked call to git
	cd, err := charm.ReadCharmDir(clonedPath)
	c.Assert(err, jc.ErrorIsNil)
	path := archivePath(c, cd)

	// Read back the archive and verify the correct version
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archive.Version(), gc.Equals, "c0ffee")
}

func (s *CharmArchiveSuite) prepareCharmArchive(c *gc.C, charmDir *charm.CharmDir, archivePath string) {
	file, err := os.Create(archivePath)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()
	zipw := zip.NewWriter(file)
	defer zipw.Close()

	h := &zip.FileHeader{Name: "revision"}
	h.SetMode(syscall.S_IFREG | 0644)
	w, err := zipw.CreateHeader(h)
	c.Assert(err, jc.ErrorIsNil)
	_, err = w.Write([]byte(strconv.Itoa(charmDir.Revision())))
	c.Assert(err, jc.ErrorIsNil)

	h = &zip.FileHeader{Name: "metadata.yaml", Method: zip.Deflate}
	h.SetMode(0644)
	w, err = zipw.CreateHeader(h)
	c.Assert(err, jc.ErrorIsNil)
	data, err := yaml.Marshal(charmDir.Meta())
	c.Assert(err, jc.ErrorIsNil)
	_, err = w.Write(data)
	c.Assert(err, jc.ErrorIsNil)

	for name := range charmDir.Meta().Hooks() {
		hookName := filepath.Join("hooks", name)
		h = &zip.FileHeader{
			Name:   hookName,
			Method: zip.Deflate,
		}
		// Force it non-executable
		h.SetMode(0644)
		w, err := zipw.CreateHeader(h)
		c.Assert(err, jc.ErrorIsNil)
		_, err = w.Write([]byte("not important"))
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *CharmArchiveSuite) TestExpandToSetsHooksExecutable(c *gc.C) {
	charmDir, err := charm.ReadCharmDir(cloneDir(c, charmDirPath(c, "all-hooks")))
	c.Assert(err, jc.ErrorIsNil)
	// CharmArchive manually, so we can check ExpandTo(), unaffected
	// by ArchiveTo()'s behavior
	archivePath := filepath.Join(c.MkDir(), "archive.charm")
	s.prepareCharmArchive(c, charmDir, archivePath)
	archive, err := charm.ReadCharmArchive(archivePath)
	c.Assert(err, jc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, jc.ErrorIsNil)

	_, err = charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)

	for name := range archive.Meta().Hooks() {
		hookName := string(name)
		info, err := os.Stat(filepath.Join(path, "hooks", hookName))
		c.Assert(err, jc.ErrorIsNil)
		perm := info.Mode() & 0777
		c.Assert(perm&0100 != 0, gc.Equals, true, gc.Commentf("hook %q is not executable", hookName))
	}
}

func (s *CharmArchiveSuite) TestCharmArchiveFileModes(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)
	}
	var haveSymlinks = true
	if err := os.Symlink("../target", filepath.Join(srcPath, "hooks/symlink")); err != nil {
		haveSymlinks = false
	}

	// CharmArchive and extract the charm to a new directory.
	archive := archiveDir(c, srcPath)
	path := c.MkDir()
	err := archive.ExpandTo(path)
	c.Assert(err, jc.ErrorIsNil)

	// Check sensible file modes once round-tripped.
	info, err := os.Stat(filepath.Join(path, "src", "hello.c"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Mode()&0777, gc.Equals, os.FileMode(0644))
	c.Assert(info.Mode()&os.ModeType, gc.Equals, os.FileMode(0))

	info, err = os.Stat(filepath.Join(path, "hooks", "install"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Mode()&0777, gc.Equals, os.FileMode(0755))
	c.Assert(info.Mode()&os.ModeType, gc.Equals, os.FileMode(0))

	info, err = os.Stat(filepath.Join(path, "empty"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Mode()&0777, gc.Equals, os.FileMode(0755))

	if haveSymlinks {
		target, err := os.Readlink(filepath.Join(path, "hooks", "symlink"))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(target, gc.Equals, "../target")
	}
}

func (s *CharmArchiveSuite) TestCharmArchiveRevisionFile(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, jc.ErrorIsNil)

	archive := extCharmArchiveDir(c, charmDir)
	c.Assert(archive.Revision(), gc.Equals, 0)

	// Missing revision file with obsolete old revision in metadata;
	// the revision is ignored.
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, jc.ErrorIsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, jc.ErrorIsNil)

	archive = extCharmArchiveDir(c, charmDir)
	c.Assert(archive.Revision(), gc.Equals, 0)

	// Revision file with bad content
	err = os.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, jc.ErrorIsNil)

	path := extCharmArchiveDirPath(c, charmDir)
	archive, err = charm.ReadCharmArchive(path)
	c.Assert(err, gc.ErrorMatches, "invalid revision file")
	c.Assert(archive, gc.IsNil)
}

func (s *CharmArchiveSuite) TestCharmArchiveSetRevision(c *gc.C) {
	archive, err := charm.ReadCharmArchive(s.archivePath)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(archive.Revision(), gc.Equals, 1)
	archive.SetRevision(42)
	c.Assert(archive.Revision(), gc.Equals, 42)

	path := filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, 42)
}

func (s *CharmArchiveSuite) TestExpandToWithBadLink(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	badLink := filepath.Join(charmDir, "hooks", "badlink")

	// Symlink targeting a path outside of the charm.
	err := os.Symlink("../../target", badLink)
	c.Assert(err, jc.ErrorIsNil)

	archive := extCharmArchiveDir(c, charmDir)
	c.Assert(err, jc.ErrorIsNil)

	path := filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, gc.ErrorMatches, `cannot extract "hooks/badlink": symlink "../../target" leads out of scope`)

	// Symlink targeting an absolute path.
	os.Remove(badLink)
	err = os.Symlink("/target", badLink)
	c.Assert(err, jc.ErrorIsNil)

	archive = extCharmArchiveDir(c, charmDir)
	c.Assert(err, jc.ErrorIsNil)

	path = filepath.Join(c.MkDir(), "charm")
	err = archive.ExpandTo(path)
	c.Assert(err, gc.ErrorMatches, `cannot extract "hooks/badlink": symlink "/target" is absolute`)
}

func extCharmArchiveDirPath(c *gc.C, dirpath string) string {
	path := filepath.Join(c.MkDir(), "archive.charm")
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %s; zip --fifo --symlinks -r %s .", dirpath, path))
	output, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil, gc.Commentf("Command output: %s", output))
	return path
}

func extCharmArchiveDir(c *gc.C, dirpath string) *charm.CharmArchive {
	path := extCharmArchiveDirPath(c, dirpath)
	archive, err := charm.ReadCharmArchive(path)
	c.Assert(err, jc.ErrorIsNil)
	return archive
}

func archiveDir(c *gc.C, dirpath string) *charm.CharmArchive {
	dir, err := charm.ReadCharmDir(dirpath)
	c.Assert(err, jc.ErrorIsNil)

	buf := new(bytes.Buffer)
	err = dir.ArchiveTo(buf)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(buf.Bytes())
	c.Assert(err, jc.ErrorIsNil)

	return archive
}
