// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&mongoRestoreSuite{})

type mongoRestoreSuite struct {
	testing.BaseSuite
}

func (s *mongoRestoreSuite) TestMongoRestoreArgsForVersion121(c *gc.C) {
	dir := filepath.Join(agent.DefaultPaths.DataDir, "db")
	versionNumber := version.Number{}
	versionNumber.Major = 1
	versionNumber.Minor = 21
	args, err := backups.MongoRestoreArgsForVersion(versionNumber, "/some/fake/path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(args, gc.HasLen, 5)
	c.Assert(args[0:5], jc.DeepEquals, []string{
		"--drop",
		"--journal",
		"--dbpath",
		dir,
		"/some/fake/path",
	})
}

func (s *mongoRestoreSuite) TestMongoRestoreArgsForVersion122(c *gc.C) {
	dir := filepath.Join(agent.DefaultPaths.DataDir, "db")
	versionNumber := version.Number{}
	versionNumber.Major = 1
	versionNumber.Minor = 22
	args, err := backups.MongoRestoreArgsForVersion(versionNumber, "/some/fake/path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(args, gc.HasLen, 6)
	c.Assert(args[0:6], jc.DeepEquals, []string{
		"--drop",
		"--journal",
		"--oplogReplay",
		"--dbpath",
		dir,
		"/some/fake/path",
	})
}

func (s *mongoRestoreSuite) TestMongoRestoreArgsForVersion2(c *gc.C) {
	dir := filepath.Join(agent.DefaultPaths.DataDir, "db")
	versionNumber := version.Number{}
	versionNumber.Major = 2
	versionNumber.Minor = 0
	args, err := backups.MongoRestoreArgsForVersion(versionNumber, "/some/fake/path")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(args, gc.HasLen, 6)
	c.Assert(args[0:6], jc.DeepEquals, []string{
		"--drop",
		"--journal",
		"--oplogReplay",
		"--dbpath",
		dir,
		"/some/fake/path",
	})
}

func (s *mongoRestoreSuite) TestMongoRestoreArgsForOldVersion(c *gc.C) {
	versionNumber := version.Number{}
	versionNumber.Major = 0
	versionNumber.Minor = 0
	_, err := backups.MongoRestoreArgsForVersion(versionNumber, "/some/fake/path")
	c.Assert(err, gc.ErrorMatches, "this backup file is incompatible with the current version of juju")
}

func (s *mongoRestoreSuite) TestPlaceNewMongo(c *gc.C) {
	var argsVersion version.Number
	var newMongoDumpPath string
	ranArgs := make([][]string, 0, 3)
	ranCommands := []string{}

	restorePathCalled := false

	runCommand := func(command string, mongoRestoreArgs ...string) error {
		mgoArgs := make([]string, len(mongoRestoreArgs), len(mongoRestoreArgs))
		for i, v := range mongoRestoreArgs {
			mgoArgs[i] = v
		}
		ranArgs = append(ranArgs, mgoArgs)
		ranCommands = append(ranCommands, command)
		return nil
	}
	s.PatchValue(backups.RunCommand, runCommand)

	restorePath := func() (string, error) {
		restorePathCalled = true
		return "/fake/mongo/restore/path", nil
	}
	s.PatchValue(backups.RestorePath, restorePath)

	ver := version.Number{Major: 1, Minor: 22}
	args := []string{"a", "set", "of", "args"}
	restoreArgsForVersion := func(versionNumber version.Number, mongoDumpPath string) ([]string, error) {
		newMongoDumpPath = mongoDumpPath
		argsVersion = versionNumber
		return args, nil
	}
	s.PatchValue(backups.RestoreArgsForVersion, restoreArgsForVersion)

	err := backups.PlaceNewMongo("fakemongopath", ver)
	c.Assert(restorePathCalled, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(argsVersion, gc.DeepEquals, ver)
	c.Assert(newMongoDumpPath, gc.Equals, "fakemongopath")
	expectedCommands := []string{"/fake/mongo/restore/path"}
	c.Assert(ranCommands, gc.DeepEquals, expectedCommands)
	c.Assert(len(ranArgs), gc.Equals, 1)
	expectedArgs := [][]string{{"a", "set", "of", "args"}}
	c.Assert(ranArgs, gc.DeepEquals, expectedArgs)
}
