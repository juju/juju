// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups_test

import (
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

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

	s.root = c.MkDir()
}

// createFiles preps the fake FS. The files are all created relative to
// the given root.
func (s *filesSuite) createFiles(c *gc.C, paths backups.Paths, root, machineID string) {
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
	touch(dirname, "shared-secret")
	mkdir(filepath.Join(paths.DataDir, "tools"))

	dirname = mkdir(filepath.Join(paths.DataDir, "agents"))
	touch(dirname, "machine-"+machineID+".conf")

	dirname = mkdir(paths.LogsDir)
	touch(dirname, "all-machines.log")
	touch(dirname, "machine-"+machineID+".log")

	dirname = mkdir("/etc/rsyslog.d")
	touch(dirname, "spam-juju.conf")

	dirname = mkdir("/home/ubuntu/.ssh")
	touch(dirname, "authorized_keys")
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

func (s *filesSuite) TestGetFilesToBackUpMachine0(c *gc.C) {
	paths := backups.Paths{
		DataDir: "/var/lib/juju",
		LogsDir: "/var/log/juju",
	}
	s.createFiles(c, paths, s.root, "0")

	files, err := backups.GetFilesToBackUp(s.root, &paths, "0")
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		filepath.Join(s.root, "/etc/rsyslog.d/spam-juju.conf"),
		filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-0.conf"),
		filepath.Join(s.root, "/var/lib/juju/nonce.txt"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/lib/juju/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
		filepath.Join(s.root, "/var/log/juju/all-machines.log"),
		filepath.Join(s.root, "/var/log/juju/machine-0.log"),
	}
	c.Check(files, jc.SameContents, expected)
	s.checkSameStrings(c, files, expected)
}

func (s *filesSuite) TestDirectoriesCleaned(c *gc.C) {
	recreatableFolder := filepath.Join(s.root, "recreate_me")
	os.MkdirAll(recreatableFolder, os.FileMode(0755))
	recreatableFolderInfo, err := os.Stat(recreatableFolder)
	c.Assert(err, jc.ErrorIsNil)

	recreatableFolder1 := filepath.Join(recreatableFolder, "recreate_me_too")
	os.MkdirAll(recreatableFolder1, os.FileMode(0755))
	recreatableFolder1Info, err := os.Stat(recreatableFolder1)
	c.Assert(err, jc.ErrorIsNil)

	deletableFolder := filepath.Join(recreatableFolder, "dont_recreate_me")
	os.MkdirAll(deletableFolder, os.FileMode(0755))

	deletableFile := filepath.Join(recreatableFolder, "delete_me")
	fh, err := os.Create(deletableFile)
	c.Assert(err, jc.ErrorIsNil)
	defer fh.Close()

	deletableFile1 := filepath.Join(recreatableFolder1, "delete_me.too")
	fhr, err := os.Create(deletableFile1)
	c.Assert(err, jc.ErrorIsNil)
	defer fhr.Close()

	s.PatchValue(backups.ReplaceableFolders, func() (map[string]os.FileMode, error) {
		replaceables := map[string]os.FileMode{}
		for _, replaceable := range []string{
			recreatableFolder,
			recreatableFolder1,
		} {
			dirStat, err := os.Stat(replaceable)
			if err != nil {
				return map[string]os.FileMode{}, errors.Annotatef(err, "cannot stat %q", replaceable)
			}
			replaceables[replaceable] = dirStat.Mode()
		}
		return replaceables, nil
	})

	err = backups.PrepareMachineForRestore()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(deletableFolder)
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(os.IsNotExist(err), gc.Equals, true)

	recreatedFolderInfo, err := os.Stat(recreatableFolder)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recreatableFolderInfo.Mode(), gc.Equals, recreatedFolderInfo.Mode())
	c.Assert(recreatableFolderInfo.Sys().(*syscall.Stat_t).Ino, gc.Not(gc.Equals), recreatedFolderInfo.Sys().(*syscall.Stat_t).Ino)

	recreatedFolder1Info, err := os.Stat(recreatableFolder1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recreatableFolder1Info.Mode(), gc.Equals, recreatedFolder1Info.Mode())
	c.Assert(recreatableFolder1Info.Sys().(*syscall.Stat_t).Ino, gc.Not(gc.Equals), recreatedFolder1Info.Sys().(*syscall.Stat_t).Ino)
}

func (s *filesSuite) TestGetFilesToBackUpMachine10(c *gc.C) {
	paths := backups.Paths{
		DataDir: "/var/lib/juju",
		LogsDir: "/var/log/juju",
	}
	s.createFiles(c, paths, s.root, "10")

	files, err := backups.GetFilesToBackUp(s.root, &paths, "10")
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		filepath.Join(s.root, "/etc/rsyslog.d/spam-juju.conf"),
		filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-10.conf"),
		filepath.Join(s.root, "/var/lib/juju/nonce.txt"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/lib/juju/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
		filepath.Join(s.root, "/var/log/juju/all-machines.log"),
		filepath.Join(s.root, "/var/log/juju/machine-10.log"),
	}
	c.Check(files, jc.SameContents, expected)
	s.checkSameStrings(c, files, expected)
}

func (s *filesSuite) TestGetFilesToBackUpMissing(c *gc.C) {
	paths := backups.Paths{
		DataDir: "/var/lib/juju",
		LogsDir: "/var/log/juju",
	}
	s.createFiles(c, paths, s.root, "0")

	missing := []string{
		"/var/lib/juju/nonce.txt",
		"/home/ubuntu/.ssh/authorized_keys",
		"/var/log/juju/all-machines.log",
		"/var/log/juju/machine-0.log",
	}
	for _, filename := range missing {
		err := os.Remove(filepath.Join(s.root, filename))
		c.Assert(err, jc.ErrorIsNil)
	}

	files, err := backups.GetFilesToBackUp(s.root, &paths, "0")
	c.Assert(err, jc.ErrorIsNil)

	expected := []string{
		filepath.Join(s.root, "/etc/rsyslog.d/spam-juju.conf"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-0.conf"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/lib/juju/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
	}
	// This got re-created.
	expected = append(expected, filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"))
	c.Check(files, jc.SameContents, expected)
	s.checkSameStrings(c, files, expected)
}
