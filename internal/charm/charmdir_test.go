// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type CharmDirSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmDirSuite{})

func (s *CharmDirSuite) TestIsCharmDirGoodCharm(c *gc.C) {
	path := charmDirPath(c, "dummy")
	c.Assert(charm.IsCharmDir(path), jc.IsTrue)
}

func (s *CharmDirSuite) TestIsCharmDirBundle(c *gc.C) {
	path := bundleDirPath(c, "wordpress-simple")
	c.Assert(charm.IsCharmDir(path), jc.IsFalse)
}

func (s *CharmDirSuite) TestIsCharmDirNoMetadataYaml(c *gc.C) {
	path := charmDirPath(c, "bad")
	c.Assert(charm.IsCharmDir(path), jc.IsFalse)
}

func (s *CharmDirSuite) TestReadCharmDir(c *gc.C) {
	path := charmDirPath(c, "dummy")
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	checkDummy(c, dir, path)
}

func (s *CharmDirSuite) TestReadCharmDirWithoutConfig(c *gc.C) {
	path := charmDirPath(c, "varnish")
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(dir.Config().Options, gc.HasLen, 0)
}

func (s *CharmDirSuite) TestReadCharmDirWithoutActions(c *gc.C) {
	path := charmDirPath(c, "wordpress")
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)

	// A lacking actions.yaml file still causes a proper
	// Actions value to be returned.
	c.Assert(dir.Actions().ActionSpecs, gc.HasLen, 0)
}

func (s *CharmDirSuite) TestReadCharmDirWithActions(c *gc.C) {
	path := charmDirPath(c, "dummy-actions")
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Actions().ActionSpecs, gc.HasLen, 1)
}

func (s *CharmDirSuite) TestReadCharmDirWithJujuActions(c *gc.C) {
	path := charmDirPath(c, "juju-charm")
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Actions().ActionSpecs, gc.HasLen, 1)
}

func (s *CharmDirSuite) TestReadCharmDirManifest(c *gc.C) {
	path := charmDirPath(c, "dummy")
	dir, err := charm.ReadCharmDir(path)
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

func (s *CharmDirSuite) TestReadCharmDirWithoutManifest(c *gc.C) {
	path := charmDirPath(c, "mysql")
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Manifest(), gc.IsNil)
}

func (s *CharmDirSuite) TestArchiveTo(c *gc.C) {
	baseDir := c.MkDir()
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	s.assertArchiveTo(c, baseDir, charmDir)
}

func (s *CharmDirSuite) TestArchiveToWithIgnoredFiles(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	// Add a directory/files that should be ignored
	nestedGitDir := filepath.Join(dir.Path, ".git/nested")
	err = os.MkdirAll(nestedGitDir, 0700)
	c.Assert(err, jc.ErrorIsNil)

	f, err := os.Create(filepath.Join(nestedGitDir, "foo"))
	c.Assert(err, jc.ErrorIsNil)
	_ = f.Close()

	// Ensure that we cannot spoof the version or revision files
	err = os.WriteFile(filepath.Join(dir.Path, "version"), []byte("spoofed version"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dir.Path, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	var b bytes.Buffer
	err = dir.ArchiveTo(&b)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(b.Bytes())
	c.Assert(err, jc.ErrorIsNil)

	manifest, err := archive.ArchiveMembers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifest, jc.DeepEquals, set.NewStrings(dummyArchiveMembers...))

	c.Assert(archive.Version(), gc.Not(gc.Equals), "spoofed version")
	c.Assert(archive.Revision(), gc.Not(gc.Equals), 42)
}

func (s *CharmDirSuite) TestArchiveToWithJujuignore(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))

	jujuignore := `
# Ignore directory named "bar" anywhere in the charm
bar/

# Retain "tox" but ignore everything inside it EXCEPT "keep"
tox/**
!tox/keep
`
	// Add .jujuignore
	err := os.WriteFile(filepath.Join(charmDir, ".jujuignore"), []byte(jujuignore), 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Add directory/files that should be ignored based on jujuignore rules
	nestedDir := filepath.Join(charmDir, "foo/bar/baz")
	err = os.MkdirAll(nestedDir, 0700)
	c.Assert(err, jc.ErrorIsNil)

	toxDir := filepath.Join(charmDir, "tox")
	err = os.MkdirAll(filepath.Join(toxDir, "data"), 0700)
	c.Assert(err, jc.ErrorIsNil)

	f, err := os.Create(filepath.Join(toxDir, "keep"))
	c.Assert(err, jc.ErrorIsNil)
	_ = f.Close()

	f, err = os.Create(filepath.Join(toxDir, "ignore"))
	c.Assert(err, jc.ErrorIsNil)
	_ = f.Close()

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	var b bytes.Buffer
	err = dir.ArchiveTo(&b)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(b.Bytes())
	c.Assert(err, jc.ErrorIsNil)

	// Based on the .jujuignore rules, we should retain "foo/bar" and
	// "tox/keep" but nothing else
	retained := []string{"foo", "tox", "tox/keep"}
	expContents := set.NewStrings(append(retained, dummyArchiveMembers...)...)

	manifest, err := archive.ArchiveMembers()
	c.Log(manifest.Difference(expContents))
	c.Log(expContents.Difference(manifest))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manifest, jc.DeepEquals, expContents)
}

func (s *CharmSuite) TestArchiveToWithVersionString(c *gc.C) {
	baseDir := c.MkDir()
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	testing.PatchExecutableAsEchoArgs(c, s, "git")

	// create an empty .execName file inside tempDir
	vcsPath := filepath.Join(charmDir, ".git")
	_, err := os.Create(vcsPath)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	path := filepath.Join(baseDir, "archive.charm")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)

	err = dir.ArchiveTo(file)
	_ = file.Close()
	c.Assert(err, jc.ErrorIsNil)

	args := []string{"describe", "--dirty", "--always"}
	testing.AssertEchoArgs(c, "git", args...)

	zipr, err := zip.OpenReader(path)
	c.Assert(err, jc.ErrorIsNil)
	defer zipr.Close()

	var verf *zip.File
	for _, f := range zipr.File {
		if f.Name == "version" {
			verf = f
		}
	}

	c.Assert(verf, gc.NotNil)
	reader, err := verf.Open()
	c.Assert(err, jc.ErrorIsNil)
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	c.Assert(err, jc.ErrorIsNil)

	obtainedData := string(data)
	obtainedData = strings.TrimSuffix(obtainedData, "\n")

	expectedArg := "git"
	for _, arg := range args {
		expectedArg = fmt.Sprintf("%s %s", expectedArg, utils.ShQuote(arg))
	}
	c.Assert(obtainedData, gc.Equals, expectedArg)
}

func (s *CharmSuite) TestMaybeGenerateVersionStringHasAVersionFile(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	versionFile := filepath.Join(charmDir, "version")
	f, err := os.Create(versionFile)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()

	expectedVersionNumber := "123456789abc"
	_, err = f.WriteString(expectedVersionNumber)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	version, vcsType, err := dir.MaybeGenerateVersionString()
	c.Assert(version, gc.Equals, expectedVersionNumber)

	c.Assert(vcsType, gc.Equals, "versionFile")

	c.Assert(err, jc.ErrorIsNil)
}

func (s *CharmSuite) TestReadCharmDirNoLogging(c *gc.C) {
	var tw loggo.TestWriter
	err := loggo.RegisterWriter("versionstring-test", &tw)
	c.Assert(err, jc.ErrorIsNil)
	defer loggo.RemoveWriter("versionstring-test")

	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Version(), gc.Equals, "")

	noLogging := jc.SimpleMessages{}
	c.Assert(tw.Log(), jc.LogMatches, noLogging)
}

func (s *CharmSuite) TestArchiveToWithVersionStringError(c *gc.C) {
	baseDir := c.MkDir()
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))

	// create an empty .execName file inside tempDir
	vcsPath := filepath.Join(charmDir, ".git")
	_, err := os.Create(vcsPath)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	path := filepath.Join(baseDir, "archive.charm")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)

	testing.PatchExecutableThrowError(c, s, "git", 128)
	var tw loggo.TestWriter
	err = loggo.RegisterWriter("versionstring-test", &tw)
	c.Assert(err, jc.ErrorIsNil)
	defer loggo.RemoveWriter("versionstring-test")

	msg := fmt.Sprintf("%q version string generation failed : exit status 128\nThis means that the charm version won't show in juju status. Charm path %q", "git", dir.Path)

	_, _, err = dir.MaybeGenerateVersionString()
	c.Assert(err, gc.ErrorMatches, msg)

	err = dir.ArchiveTo(file)
	_ = file.Close()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		Level: loggo.WARNING, Message: msg,
	}})

	zipr, err := zip.OpenReader(path)
	c.Assert(err, jc.ErrorIsNil)
	defer zipr.Close()

	for _, f := range zipr.File {
		if f.Name == "version" {
			c.Fatal("unexpected version in charm archive")
		}
	}
}

func (s *CharmDirSuite) TestArchiveToWithSymlinkedRootDir(c *gc.C) {
	path := cloneDir(c, charmDirPath(c, "dummy"))
	baseDir := filepath.Dir(path)
	err := os.Symlink(filepath.Join("dummy"), filepath.Join(baseDir, "newdummy"))
	c.Assert(err, jc.ErrorIsNil)
	charmDir := filepath.Join(baseDir, "newdummy")

	s.assertArchiveTo(c, baseDir, charmDir)
}

func (s *CharmDirSuite) assertArchiveTo(c *gc.C, baseDir, charmDir string) {
	haveSymlinks := true
	if err := os.Symlink("../target", filepath.Join(charmDir, "hooks/symlink")); err != nil {
		haveSymlinks = false
	}

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	path := filepath.Join(baseDir, "archive.charm")

	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)

	err = dir.ArchiveTo(file)
	c.Assert(err, jc.ErrorIsNil)

	err = file.Close()
	c.Assert(err, jc.ErrorIsNil)

	zipr, err := zip.OpenReader(path)
	c.Assert(err, jc.ErrorIsNil)
	defer zipr.Close()

	var metaf, instf, emptyf, revf, symf *zip.File
	for _, f := range zipr.File {
		c.Logf("Archived file: %s", f.Name)
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
			c.Errorf("archive includes build/*: %s", f.Name)
		case ".ignored", ".dir/ignored":
			c.Errorf("archive includes .* entries: %s", f.Name)
		}
	}

	c.Assert(revf, gc.NotNil)
	reader, err := revf.Open()
	c.Assert(err, jc.ErrorIsNil)
	data, err := io.ReadAll(reader)
	reader.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "1")

	c.Assert(metaf, gc.NotNil)
	reader, err = metaf.Open()
	c.Assert(err, jc.ErrorIsNil)
	meta, err := charm.ReadMeta(reader)
	reader.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta.Name, gc.Equals, "dummy")

	c.Assert(instf, gc.NotNil)
	// Despite it being 0751, we pack and unpack it as 0755.
	c.Assert(instf.Mode()&0777, gc.Equals, os.FileMode(0755))

	if haveSymlinks {
		c.Assert(symf, gc.NotNil)
		c.Assert(symf.Mode()&0777, gc.Equals, os.FileMode(0777))
		reader, err = symf.Open()
		c.Assert(err, jc.ErrorIsNil)
		data, err = io.ReadAll(reader)
		reader.Close()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(data), gc.Equals, "../target")
	} else {
		c.Assert(symf, gc.IsNil)
	}

	c.Assert(emptyf, gc.NotNil)
	c.Assert(emptyf.Mode()&os.ModeType, gc.Equals, os.ModeDir)
	// Despite it being 0750, we pack and unpack it as 0755.
	c.Assert(emptyf.Mode()&0777, gc.Equals, os.FileMode(0755))
}

// Bug #864164: Must complain if charm hooks aren't executable
func (s *CharmDirSuite) TestArchiveToWithNonExecutableHooks(c *gc.C) {
	hooks := []string{"install", "start", "config-changed", "upgrade-charm", "stop"}
	for _, relName := range []string{"foo", "bar", "self"} {
		for _, kind := range []string{"joined", "changed", "departed", "broken"} {
			hooks = append(hooks, relName+"-relation-"+kind)
		}
	}

	dir := readCharmDir(c, "all-hooks")
	path := filepath.Join(c.MkDir(), "archive.charm")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.ArchiveTo(file)
	file.Close()
	c.Assert(err, jc.ErrorIsNil)

	tlog := c.GetTestLog()
	for _, hook := range hooks {
		fullpath := filepath.Join(dir.Path, "hooks", hook)
		exp := fmt.Sprintf(`^(.|\n)*WARNING juju.charm making "%s" executable in charm(.|\n)*$`, fullpath)
		c.Assert(tlog, gc.Matches, exp, gc.Commentf("hook %q was not made executable", fullpath))
	}

	// Expand it and check the hooks' permissions
	// (But do not use ExpandTo(), just use the raw zip)
	f, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	fi, err := f.Stat()
	c.Assert(err, jc.ErrorIsNil)
	size := fi.Size()
	zipr, err := zip.NewReader(f, size)
	c.Assert(err, jc.ErrorIsNil)
	allhooks := dir.Meta().Hooks()
	for _, zfile := range zipr.File {
		cleanName := filepath.Clean(zfile.Name)
		if strings.HasPrefix(cleanName, "hooks") {
			hookName := filepath.Base(cleanName)
			if _, ok := allhooks[hookName]; ok {
				perms := zfile.Mode()
				c.Assert(perms&0100 != 0, gc.Equals, true, gc.Commentf("hook %q is not executable", hookName))
			}
		}
	}
}

func (s *CharmDirSuite) TestArchiveToWithBadType(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	badFile := filepath.Join(charmDir, "hooks", "badfile")

	// Symlink targeting a path outside of the charm.
	err := os.Symlink("../../target", badFile)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	err = dir.ArchiveTo(&bytes.Buffer{})
	c.Assert(err, gc.ErrorMatches, `.*symlink "hooks/badfile" links out of charm: "../../target"`)

	// Symlink targeting an absolute path.
	os.Remove(badFile)
	err = os.Symlink("/target", badFile)
	c.Assert(err, jc.ErrorIsNil)

	dir, err = charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	err = dir.ArchiveTo(&bytes.Buffer{})
	c.Assert(err, gc.ErrorMatches, `.*symlink "hooks/badfile" is absolute: "/target"`)

	// Can't archive special files either.
	os.Remove(badFile)
	err = syscall.Mkfifo(badFile, 0644)
	c.Assert(err, jc.ErrorIsNil)

	dir, err = charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	err = dir.ArchiveTo(&bytes.Buffer{})
	c.Assert(err, gc.ErrorMatches, `.*file is a named pipe: "hooks/badfile"`)
}

func (s *CharmDirSuite) TestDirRevisionFile(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, 0)

	// Missing revision file with obsolete old revision in metadata ignores
	// the old revision field.
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, jc.ErrorIsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, jc.ErrorIsNil)

	dir, err = charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, 0)

	// Revision file with bad content
	err = os.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, jc.ErrorIsNil)

	dir, err = charm.ReadCharmDir(charmDir)
	c.Assert(err, gc.ErrorMatches, "invalid revision file")
	c.Assert(dir, gc.IsNil)
}

func (s *CharmDirSuite) TestDirSetRevision(c *gc.C) {
	path := cloneDir(c, charmDirPath(c, "dummy"))
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, 1)
	dir.SetRevision(42)
	c.Assert(dir.Revision(), gc.Equals, 42)

	var b bytes.Buffer
	err = dir.ArchiveTo(&b)
	c.Assert(err, jc.ErrorIsNil)

	archive, err := charm.ReadCharmArchiveBytes(b.Bytes())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archive.Revision(), gc.Equals, 42)
}

func (s *CharmDirSuite) TestDirSetDiskRevision(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(dir.Revision(), gc.Equals, 1)
	dir.SetDiskRevision(42)
	c.Assert(dir.Revision(), gc.Equals, 42)

	dir, err = charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, 42)
}

func (s *CharmSuite) TestMaybeGenerateVersionStringError(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))

	testing.PatchExecutableThrowError(c, s, "git", 128)
	vcsPath := filepath.Join(charmDir, ".git")
	_, err := os.Create(vcsPath)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	version, vcsType, err := dir.MaybeGenerateVersionString()
	msg := fmt.Sprintf("%q version string generation failed : exit status 128\nThis means that the charm version won't show in juju status. Charm path %q", "git", dir.Path)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(version, gc.Equals, "")
	c.Assert(vcsType, gc.Equals, "git")
}

func (s *CharmSuite) assertGenerateVersionString(c *gc.C, execName string, args []string) {
	// Read the charmDir from the testing folder and clone all contents.
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))

	testing.PatchExecutableAsEchoArgs(c, s, execName)

	// create an empty .execName file inside tempDir
	vcsPath := filepath.Join(charmDir, "."+execName)
	_, err := os.Create(vcsPath)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	version, vcsType, err := dir.MaybeGenerateVersionString()
	c.Assert(err, jc.ErrorIsNil)

	version = strings.Trim(version, "\n")
	version = strings.Replace(version, "'", "", -1)
	expectedVersion := strings.Join(append([]string{execName}, args...), " ")
	c.Assert(version, gc.Equals, expectedVersion)
	c.Assert(vcsType, gc.Equals, execName)

	testing.AssertEchoArgs(c, execName, args...)
}

// TestCreateMaybeGenerateVersionString verifies if the version string can be generated
// in case of git revision control directory
func (s *CharmSuite) TestGitMaybeGenerateVersionString(c *gc.C) {
	s.assertGenerateVersionString(c, "git", []string{"describe", "--dirty", "--always"})
}

// TestBzrMaybeGenaretVersionString verifies if the version string can be generated
// in case of bazaar revision control directory.
func (s *CharmSuite) TestBazaarMaybeGenerateVersionString(c *gc.C) {
	s.assertGenerateVersionString(c, "bzr", []string{"version-info"})
}

// TestHgMaybeGenerateVersionString verifies if the version string can be generated
// in case of Mecurial revision control directory.
func (s *CharmSuite) TestHgMaybeGenerateVersionString(c *gc.C) {
	s.assertGenerateVersionString(c, "hg", []string{"id", "-n"})
}

// TestNoVCSMaybeGenerateVersionString verifies that version string not generated
// in case of not a revision control directory.
func (s *CharmSuite) TestNoVCSMaybeGenerateVersionString(c *gc.C) {
	// Read the charmDir from the testing folder and clone the contents.
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))

	dir, err := charm.ReadCharmDir(charmDir)
	c.Assert(err, jc.ErrorIsNil)

	versionString, vcsType, err := dir.MaybeGenerateVersionString()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(versionString, gc.Equals, "")
	c.Assert(vcsType, gc.Equals, "")
}

// TestMaybeGenerateVersionStringUsesAbsolutePathGitVersion verifies that using a relative path still works.
func (s *CharmSuite) TestMaybeGenerateVersionStringUsesAbsolutePathGitVersion(c *gc.C) {
	// Read the relativePath from the testing folder.
	relativePath := charmDirPath(c, "dummy")
	dir, err := charm.ReadCharmDir(relativePath)
	c.Assert(err, jc.ErrorIsNil)

	versionString, vcsType, err := dir.MaybeGenerateVersionString()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(versionString, gc.Not(gc.Equals), "")
	c.Assert(vcsType, gc.Equals, "git")
}

// TestMaybeGenerateVersionStringLogsAbsolutePath verifies that the absolute path gets logged.
func (s *CharmSuite) TestMaybeGenerateVersionStringLogsAbsolutePath(c *gc.C) {
	var tw loggo.TestWriter
	ctx := loggo.DefaultContext()
	err := ctx.AddWriter("versionstring-test", &tw)
	c.Assert(err, jc.ErrorIsNil)

	logger := ctx.GetLogger("juju.testing")
	lvl, _ := loggo.ParseLevel("TRACE")
	logger.SetLogLevel(lvl)
	defer func() { _, _ = loggo.RemoveWriter("versionstring-test") }()
	defer loggo.ResetLogging()

	testing.PatchExecutableThrowError(c, s, "git", 128)

	// Read the relativePath from the testing folder.
	relativePath := charmDirPath(c, "dummy")
	absPath, err := filepath.Abs(relativePath)
	c.Assert(err, jc.ErrorIsNil)

	dir, err := charm.ReadCharmDir(relativePath, charm.WithLogger(internallogger.WrapLoggo(logger)))
	c.Assert(err, jc.ErrorIsNil)

	expectedMsg := fmt.Sprintf("charm is not versioned, charm path %q", absPath)

	versionString, vcsType, err := dir.MaybeGenerateVersionString()
	c.Assert(len(tw.Log()), gc.Equals, 1)
	c.Assert(tw.Log()[0].Message, gc.Matches, expectedMsg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(versionString, gc.Matches, "")
	c.Assert(vcsType, gc.Equals, "")
}

// We expect it to be successful because we set the timeout to be high and the executable "git" returns error code 0
func (s *CharmSuite) TestCheckGitIsUsed(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	testing.PatchExecutableAsEchoArgs(c, s, "git")
	cmdWaitTime := 100 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), cmdWaitTime)
	isUsing := charm.UsesGit(ctx, charmDir, cancel, loggertesting.WrapCheckLog(c))
	c.Assert(isUsing, gc.Equals, true)
}

// We create the executable "git" and still expect it to "fail" because we set the timeout to be 0
func (s *CharmSuite) TestCheckGitTimeout(c *gc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	testing.PatchExecutableAsEchoArgs(c, s, "git")
	cmdWaitTime := 0 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), cmdWaitTime)
	isUsing := charm.UsesGit(ctx, charmDir, cancel, loggertesting.WrapCheckLog(c))
	c.Assert(isUsing, gc.Equals, false)
}
