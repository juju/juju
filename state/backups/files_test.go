// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package backups_test

import (
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&filesSuite{})

type filesSuite struct {
	testing.BaseSuite
	root string
}

func (s *filesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	// Set the process' umask to 0 so tests that check permission bits don't
	// fail due to the users umask being an unexpected value.
	oldUmask := syscall.Umask(0)
	s.AddCleanup(func(_ *gc.C) {
		syscall.Umask(oldUmask)
	})

	s.root = c.MkDir()
}

func (s *filesSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

// createFiles preps the fake FS. The files are all created relative to
// the given root.
func (s *filesSuite) createFiles(c *gc.C, paths corebackups.Paths, root, machineID string, snapPaths bool) {
	mkdir := func(path string) string {
		dirname := filepath.Join(root, path)
		os.MkdirAll(dirname, 0777)
		return dirname
	}
	touch := func(dirname, name string) {
		path := filepath.Join(dirname, name)
		file, err := os.Create(path)
		c.Assert(err, jc.ErrorIsNil)
		file.Close()
	}

	dirname := mkdir(paths.DataDir)
	touch(dirname, "system-identity")
	touch(dirname, "nonce.txt")
	touch(dirname, "server.pem")
	if snapPaths {
		snapDirname := mkdir("/var/snap/juju-db/common")
		touch(snapDirname, "shared-secret")
	} else {
		touch(dirname, "shared-secret")
	}
	mkdir(filepath.Join(paths.DataDir, "tools"))

	dirname = mkdir(filepath.Join(paths.DataDir, "agents"))
	touch(dirname, "machine-"+machineID+".conf")

	dirname = mkdir("/home/ubuntu/.ssh")
	touch(dirname, "authorized_keys")

	dirname = mkdir(filepath.Join(paths.DataDir, "init", "juju-db"))
	touch(dirname, "juju-db.service")
}

func (s *filesSuite) checkSameStrings(c *gc.C, actual, expected []string) {
	sActual := set.NewStrings(actual...)
	sExpected := set.NewStrings(expected...)

	sActualOnly := sActual.Difference(sExpected)
	sExpectedOnly := sExpected.Difference(sActual)

	if !sActualOnly.IsEmpty() || !sExpectedOnly.IsEmpty() {
		c.Error("strings mismatch")
		onlyActual := sActualOnly.Values()
		onlyExpected := sExpectedOnly.Values()
		sort.Strings(onlyActual)
		sort.Strings(onlyExpected)

		if !sActualOnly.IsEmpty() {
			c.Log("...unexpected values:")
			for _, str := range onlyActual {
				c.Log(" " + str)
			}
		}
		if !sExpectedOnly.IsEmpty() {
			c.Log("...missing values:")
			for _, str := range onlyExpected {
				c.Log(" " + str)
			}
		}
	}
}

func (s *filesSuite) TestGetFilesToBackUp(c *gc.C) {
	paths := corebackups.Paths{
		DataDir: "/var/lib/juju",
		LogsDir: "/var/log/juju",
	}
	s.createFiles(c, paths, s.root, "0", false)
	s.createFiles(c, paths, s.root, "1", false)

	files, err := backups.GetFilesToBackUp(s.root, &paths)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-0.conf"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-1.conf"),
		filepath.Join(s.root, "/var/lib/juju/nonce.txt"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/lib/juju/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
		filepath.Join(s.root, "/var/lib/juju/init/juju-db"),
	}
	c.Check(files, jc.SameContents, expected)
	s.checkSameStrings(c, files, expected)
}

func (s *filesSuite) TestGetFilesToBackUpMissing(c *gc.C) {
	paths := corebackups.Paths{
		DataDir: "/var/lib/juju",
		LogsDir: "/var/log/juju",
	}
	s.createFiles(c, paths, s.root, "0", false)

	missing := []string{
		"/var/lib/juju/nonce.txt",
		"/home/ubuntu/.ssh/authorized_keys",
	}
	for _, filename := range missing {
		err := os.Remove(filepath.Join(s.root, filename))
		c.Assert(err, jc.ErrorIsNil)
	}

	files, err := backups.GetFilesToBackUp(s.root, &paths)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		filepath.Join(s.root, "/var/lib/juju/agents/machine-0.conf"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/lib/juju/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
		filepath.Join(s.root, "/var/lib/juju/init/juju-db"),
	}
	// This got re-created.
	expected = append(expected, filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"))
	c.Check(files, jc.SameContents, expected)
	s.checkSameStrings(c, files, expected)
}

func (s *filesSuite) TestGetFilesToBackUpSnap(c *gc.C) {
	paths := corebackups.Paths{
		DataDir: "/var/lib/juju",
		LogsDir: "/var/log/juju",
	}
	s.createFiles(c, paths, s.root, "0", true)

	files, err := backups.GetFilesToBackUp(s.root, &paths)
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-0.conf"),
		filepath.Join(s.root, "/var/lib/juju/nonce.txt"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/snap/juju-db/common/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
		filepath.Join(s.root, "/var/lib/juju/init/juju-db"),
	}
	c.Check(files, jc.SameContents, expected)
	s.checkSameStrings(c, files, expected)
}
