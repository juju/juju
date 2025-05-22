// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing_test

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/fs"
	"github.com/juju/juju/internal/testhelpers"
)

type CharmDirSuite struct {
	testhelpers.IsolationSuite
}

func TestCharmDirSuite(t *testing.T) {
	tc.Run(t, &CharmDirSuite{})
}

func checkDummy(c *tc.C, f charm.Charm) {
	c.Assert(f.Revision(), tc.Equals, 1)
	c.Assert(f.Meta().Name, tc.Equals, "dummy")
	c.Assert(f.Config().Options["title"].Default, tc.Equals, "My Title")
	c.Assert(f.Actions(), tc.DeepEquals,
		&charm.Actions{
			ActionSpecs: map[string]charm.ActionSpec{
				"snapshot": {
					Description: "Take a snapshot of the database.",
					Params: map[string]interface{}{
						"type":        "object",
						"description": "Take a snapshot of the database.",
						"title":       "snapshot",
						"properties": map[string]interface{}{
							"outfile": map[string]interface{}{
								"description": "The file to write out to.",
								"type":        "string",
								"default":     "foo.bz2",
							}},
						"additionalProperties": false}}}})
	lpc, ok := f.(charm.LXDProfiler)
	c.Assert(ok, tc.IsTrue)
	c.Assert(lpc.LXDProfile(), tc.DeepEquals, &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting":    "true",
			"security.privileged": "true",
		},
		Description: "sample lxdprofile for testing",
		Devices: map[string]map[string]string{
			"tun": {
				"path": "/dev/net/tun",
				"type": "unix-char",
			},
		},
	})
}

func (s *CharmDirSuite) TestReadCharmDir(c *tc.C) {
	path := charmDirPath(c, "dummy")
	dir, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)
	checkDummy(c, dir)
}

func (s *CharmDirSuite) TestReadCharmDirWithoutConfig(c *tc.C) {
	path := charmDirPath(c, "varnish")
	dir, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)

	// A lacking config.yaml file still causes a proper
	// Config value to be returned.
	c.Assert(dir.Config().Options, tc.HasLen, 0)
}

func (s *CharmDirSuite) TestReadCharmDirWithoutActions(c *tc.C) {
	path := charmDirPath(c, "wordpress")
	dir, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)

	// A lacking actions.yaml file still causes a proper
	// Actions value to be returned.
	c.Assert(dir.Actions().ActionSpecs, tc.HasLen, 0)
}

func (s *CharmDirSuite) TestReadCharmDirWithActions(c *tc.C) {
	path := charmDirPath(c, "dummy-actions")
	dir, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir.Actions().ActionSpecs, tc.HasLen, 1)
}

func (s *CharmDirSuite) TestReadCharmDirWithJujuActions(c *tc.C) {
	path := charmDirPath(c, "juju-charm")
	dir, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir.Actions().ActionSpecs, tc.HasLen, 1)
}

func (s *CharmDirSuite) TestReadCharmDirManifest(c *tc.C) {
	path := charmDirPath(c, "dummy")
	dir, err := charmtesting.ReadCharmDir(path)
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

func (s *CharmDirSuite) TestReadCharmDirWithoutManifest(c *tc.C) {
	path := charmDirPath(c, "no-manifest")
	_, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIs, charm.FileNotFound)
}

func (s *CharmDirSuite) TestArchiveTo(c *tc.C) {
	baseDir := c.MkDir()
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	s.assertArchiveTo(c, baseDir, charmDir)
}

func (s *CharmDirSuite) TestReadCharmDirNoLogging(c *tc.C) {
	var tw loggo.TestWriter
	err := loggo.RegisterWriter("versionstring-test", &tw)
	c.Assert(err, tc.ErrorIsNil)
	defer loggo.RemoveWriter("versionstring-test")

	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	dir, err := charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir.Version(), tc.Equals, "")

	c.Assert(tw.Log(), tc.HasLen, 0)
}

func (s *CharmDirSuite) TestArchiveToWithSymlinkedRootDir(c *tc.C) {
	path := cloneDir(c, charmDirPath(c, "dummy"))
	baseDir := filepath.Dir(path)
	err := os.Symlink(filepath.Join("dummy"), filepath.Join(baseDir, "newdummy"))
	c.Assert(err, tc.ErrorIsNil)
	charmDir := filepath.Join(baseDir, "newdummy")

	s.assertArchiveTo(c, baseDir, charmDir)
}

func (s *CharmDirSuite) assertArchiveTo(c *tc.C, baseDir, charmDir string) {
	haveSymlinks := true
	if err := os.Symlink("../target", filepath.Join(charmDir, "hooks/symlink")); err != nil {
		haveSymlinks = false
	}

	dir, err := charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)

	path := filepath.Join(baseDir, "archive.charm")

	file, err := os.Create(path)
	c.Assert(err, tc.ErrorIsNil)

	err = dir.ArchiveTo(file)
	c.Assert(err, tc.ErrorIsNil)

	err = file.Close()
	c.Assert(err, tc.ErrorIsNil)

	zipr, err := zip.OpenReader(path)
	c.Assert(err, tc.ErrorIsNil)
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
		case ".ignored", ".dir/ignored":
			c.Errorf("archive includes .* entries: %s", f.Name)
		}
	}

	c.Assert(revf, tc.NotNil)
	reader, err := revf.Open()
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(reader)
	reader.Close()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, "1")

	c.Assert(metaf, tc.NotNil)
	reader, err = metaf.Open()
	c.Assert(err, tc.ErrorIsNil)
	meta, err := charm.ReadMeta(reader)
	reader.Close()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta.Name, tc.Equals, "dummy")

	c.Assert(instf, tc.NotNil)
	// Despite it being 0751, we pack and unpack it as 0755.
	c.Assert(instf.Mode()&0777, tc.Equals, os.FileMode(0755))

	if haveSymlinks {
		c.Assert(symf, tc.NotNil)
		c.Assert(symf.Mode()&0777, tc.Equals, os.FileMode(0777))
		reader, err = symf.Open()
		c.Assert(err, tc.ErrorIsNil)
		data, err = io.ReadAll(reader)
		reader.Close()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(data), tc.Equals, "../target")
	} else {
		c.Assert(symf, tc.IsNil)
	}

	c.Assert(emptyf, tc.NotNil)
	c.Assert(emptyf.Mode()&os.ModeType, tc.Equals, os.ModeDir)
	// Despite it being 0750, we pack and unpack it as 0755.
	c.Assert(emptyf.Mode()&0777, tc.Equals, os.FileMode(0755))
}

// Bug #864164: Must complain if charm hooks aren't executable
func (s *CharmDirSuite) TestArchiveToWithNonExecutableHooks(c *tc.C) {
	hooks := []string{"install", "start", "config-changed", "upgrade-charm", "stop"}
	for _, relName := range []string{"foo", "bar", "self"} {
		for _, kind := range []string{"joined", "changed", "departed", "broken"} {
			hooks = append(hooks, relName+"-relation-"+kind)
		}
	}

	dir := readCharmDir(c, "all-hooks")
	path := filepath.Join(c.MkDir(), "archive.charm")
	file, err := os.Create(path)
	c.Assert(err, tc.ErrorIsNil)
	err = dir.ArchiveTo(file)
	file.Close()
	c.Assert(err, tc.ErrorIsNil)

	// Expand it and check the hooks' permissions
	// (But do not use ExpandTo(), just use the raw zip)
	f, err := os.Open(path)
	c.Assert(err, tc.ErrorIsNil)
	defer f.Close()
	fi, err := f.Stat()
	c.Assert(err, tc.ErrorIsNil)
	size := fi.Size()
	zipr, err := zip.NewReader(f, size)
	c.Assert(err, tc.ErrorIsNil)
	allhooks := dir.Meta().Hooks()
	for _, zfile := range zipr.File {
		cleanName := filepath.Clean(zfile.Name)
		if strings.HasPrefix(cleanName, "hooks") {
			hookName := filepath.Base(cleanName)
			if _, ok := allhooks[hookName]; ok {
				perms := zfile.Mode()
				c.Assert(perms&0100 != 0, tc.Equals, true, tc.Commentf("hook %q is not executable", hookName))
			}
		}
	}
}

func (s *CharmDirSuite) TestArchiveToWithBadType(c *tc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	badFile := filepath.Join(charmDir, "hooks", "badfile")

	// Symlink targeting a path outside of the charm.
	err := os.Symlink("../../target", badFile)
	c.Assert(err, tc.ErrorIsNil)

	dir, err := charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)

	err = dir.ArchiveTo(&bytes.Buffer{})
	c.Assert(err, tc.ErrorMatches, `.*symlink "hooks/badfile" links out of charm: "../../target"`)

	// Symlink targeting an absolute path.
	os.Remove(badFile)
	err = os.Symlink("/target", badFile)
	c.Assert(err, tc.ErrorIsNil)

	dir, err = charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)

	err = dir.ArchiveTo(&bytes.Buffer{})
	c.Assert(err, tc.ErrorMatches, `.*symlink "hooks/badfile" is absolute: "/target"`)

	// Can't archive special files either.
	os.Remove(badFile)
	err = syscall.Mkfifo(badFile, 0644)
	c.Assert(err, tc.ErrorIsNil)

	dir, err = charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)

	err = dir.ArchiveTo(&bytes.Buffer{})
	c.Assert(err, tc.ErrorMatches, `.*file is a named pipe: "hooks/badfile"`)
}

func (s *CharmDirSuite) TestDirRevisionFile(c *tc.C) {
	charmDir := cloneDir(c, charmDirPath(c, "dummy"))
	revPath := filepath.Join(charmDir, "revision")

	// Missing revision file
	err := os.Remove(revPath)
	c.Assert(err, tc.ErrorIsNil)

	dir, err := charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir.Revision(), tc.Equals, 0)

	// Missing revision file with obsolete old revision in metadata ignores
	// the old revision field.
	file, err := os.OpenFile(filepath.Join(charmDir, "metadata.yaml"), os.O_WRONLY|os.O_APPEND, 0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = file.Write([]byte("\nrevision: 1234\n"))
	c.Assert(err, tc.ErrorIsNil)

	dir, err = charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir.Revision(), tc.Equals, 0)

	// Revision file with bad content
	err = os.WriteFile(revPath, []byte("garbage"), 0666)
	c.Assert(err, tc.ErrorIsNil)

	dir, err = charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorMatches, `parsing "revision" file: expected integer`)
	c.Assert(dir, tc.IsNil)
}

// charmDirPath returns the path to the charm with the
// given name in the testing repository.
func charmDirPath(c *tc.C, name string) string {
	path := filepath.Join("../internal/test-charm-repo/quantal", name)
	assertIsDir(c, path)
	return path
}

func assertIsDir(c *tc.C, path string) {
	info, err := os.Stat(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.IsDir(), tc.Equals, true)
}

// readCharmDir returns the charm with the given
// name from the testing repository.
func readCharmDir(c *tc.C, name string) *charmtesting.CharmDir {
	path := charmDirPath(c, name)
	ch, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, tc.ErrorIsNil)
	return ch
}

// cloneDir recursively copies the path directory
// into a new directory and returns the path
// to it.
func cloneDir(c *tc.C, path string) string {
	newPath := filepath.Join(c.MkDir(), filepath.Base(path))
	err := fs.Copy(path, newPath)
	c.Assert(err, tc.ErrorIsNil)
	return newPath
}
