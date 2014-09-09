// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/charm"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/utils/ssh"
)

type CharmSyncSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&CharmSyncSuite{})

func (s *CharmSyncSuite) TestTargetArgParsing(c *gc.C) {
	testInput := []struct {
		message   string
		args      []string
		charmPath string
		download  bool
		errMatch  string
	}{
		{
			message:  "no arguments",
			errMatch: "unit name is missing",
		},
		{
			message:  "too many arguments",
			args:     []string{"someCharm/0", "someotherCharm/1"},
			errMatch: "too many arguments provided.",
		},
		{
			message:   "passing charm",
			args:      []string{"someCharm/0", "--charm=/some/charm/folder"},
			charmPath: "/some/charm/folder",
			download:  false,
			errMatch:  "",
		},
		{
			message:   "passing download",
			args:      []string{"someCharm/0", "--charm=/some/charm/folder", "--down"},
			charmPath: "/some/charm/folder",
			download:  true,
			errMatch:  "",
		},
		{
			message:   "no charm path",
			args:      []string{"someCharm/0"},
			charmPath: "",
			download:  false,
			errMatch:  "",
		},
	}
	for i, test := range testInput {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		charmSyncCmd := &CharmSyncCommand{}
		testing.TestInit(c, envcmd.Wrap(charmSyncCmd), test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(charmSyncCmd.toUnit, gc.Equals, test.args[0])
			c.Check(charmSyncCmd.charmPath, gc.Equals, test.charmPath)
			c.Check(charmSyncCmd.download, gc.Equals, test.download)
		}
	}
}

func (s *CharmSyncSuite) prepareUploadCharm(c *gc.C, fakeSeries, fakeDirPath string, unitURLCalled *bool) {
	fakeUnitUrl := func(_ *CharmSyncCommand) (string, error) {
		*unitURLCalled = true
		return "ubuntu@fakeUnitURL", nil
	}
	s.PatchValue(&wrapUnitURL, fakeUnitUrl)

	fakeRemoteTempPath := func(_ *CharmSyncCommand, charmSeries string) (string, error) {
		c.Assert(charmSeries, gc.Equals, fakeSeries)
		return "/tmp/fake", nil
	}
	s.PatchValue(&wrapRemoteTempPath, fakeRemoteTempPath)

	fakeRemoteUnitPath := func(_ *CharmSyncCommand, charmSeries string) (string, error) {
		c.Assert(charmSeries, gc.Equals, fakeSeries)
		return "/fake/remote/unit", nil
	}
	s.PatchValue(&wrapRemoteUnitPath, fakeRemoteUnitPath)

	fakeSshCopy := func(scpArgs []string, _ *ssh.Options) error {
		expectedSCPArgs := []string{"-r", fakeDirPath, "ubuntu@fakeUnitURL:/tmp/fake"}
		c.Assert(scpArgs, jc.DeepEquals, expectedSCPArgs)
		return nil
	}
	s.PatchValue(&sshCopy, fakeSshCopy)

	fakeRun := func(_ *CharmSyncCommand, runParams params.RunParams) ([]params.RunResult, error) {
		c.Assert(runParams.Commands, gc.Equals, "sudo cp -rax /tmp/fake/* /fake/remote/unit; rm -rf /tmp/fake")
		return []params.RunResult{}, nil
	}
	s.PatchValue(&apiRun, fakeRun)
}

func (s *CharmSyncSuite) TestUploadCharm(c *gc.C) {
	unitURLCalled := false
	fakeSeries := "fakeSeries"
	fakeDirPath := "fakeDirPath"

	charmSyncCmd := &CharmSyncCommand{}
	s.prepareUploadCharm(c, fakeSeries, fakeDirPath, &unitURLCalled)

	_, err := charmSyncCmd.uploadCharm(fakeSeries, fakeDirPath)
	c.Assert(err, gc.IsNil)
	c.Assert(unitURLCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestUploadCharmFailsUnitURL(c *gc.C) {
	unitURLCalled := false
	fakeSeries := "fakeSeries"
	fakeDirPath := "fakeDirPath"

	charmSyncCmd := &CharmSyncCommand{}
	s.prepareUploadCharm(c, fakeSeries, fakeDirPath, &unitURLCalled)
	fakeUnitUrl := func(_ *CharmSyncCommand) (string, error) {
		unitURLCalled = true
		return "", fmt.Errorf("unitURL failed")
	}
	s.PatchValue(&wrapUnitURL, fakeUnitUrl)

	_, err := charmSyncCmd.uploadCharm(fakeSeries, fakeDirPath)
	c.Assert(err, gc.ErrorMatches, "unitURL failed")
	c.Assert(unitURLCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestUploadCharmFailsTransientFolder(c *gc.C) {
	unitURLCalled := false
	fakeSeries := "fakeSeries"
	fakeDirPath := "fakeDirPath"

	charmSyncCmd := &CharmSyncCommand{}
	s.prepareUploadCharm(c, fakeSeries, fakeDirPath, &unitURLCalled)
	fakeRemoteTempPath := func(_ *CharmSyncCommand, charmSeries string) (string, error) {
		c.Assert(charmSeries, gc.Equals, fakeSeries)
		return "", fmt.Errorf("test fails transient folder")
	}
	s.PatchValue(&wrapRemoteTempPath, fakeRemoteTempPath)

	_, err := charmSyncCmd.uploadCharm(fakeSeries, fakeDirPath)
	c.Assert(err, gc.ErrorMatches, "cannote determine remote machine temp folder: test fails transient folder")
	c.Assert(unitURLCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestUploadCharmFailsSshCopy(c *gc.C) {
	unitURLCalled := false
	fakeSeries := "fakeSeries"
	fakeDirPath := "fakeDirPath"

	charmSyncCmd := &CharmSyncCommand{}
	s.prepareUploadCharm(c, fakeSeries, fakeDirPath, &unitURLCalled)
	fakeSshCopy := func(scpArgs []string, _ *ssh.Options) error {
		expectedSCPArgs := []string{"-r", fakeDirPath, "ubuntu@fakeUnitURL:/tmp/fake"}
		c.Assert(scpArgs, jc.DeepEquals, expectedSCPArgs)
		return fmt.Errorf("testing ssh copy error")
	}
	s.PatchValue(&sshCopy, fakeSshCopy)

	_, err := charmSyncCmd.uploadCharm(fakeSeries, fakeDirPath)
	c.Assert(err, gc.ErrorMatches, "cannot copy charm to \"ubuntu@fakeUnitURL\": testing ssh copy error")
	c.Assert(unitURLCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestUploadCharmFailsRemoteCharmCopy(c *gc.C) {
	unitURLCalled := false
	fakeSeries := "fakeSeries"
	fakeDirPath := "fakeDirPath"

	charmSyncCmd := &CharmSyncCommand{}
	s.prepareUploadCharm(c, fakeSeries, fakeDirPath, &unitURLCalled)
	fakeRun := func(_ *CharmSyncCommand, runParams params.RunParams) ([]params.RunResult, error) {
		c.Assert(runParams.Commands, gc.Equals, "sudo cp -rax /tmp/fake/* /fake/remote/unit; rm -rf /tmp/fake")
		return []params.RunResult{}, fmt.Errorf("run sudo cp failed")
	}
	s.PatchValue(&apiRun, fakeRun)

	_, err := charmSyncCmd.uploadCharm(fakeSeries, fakeDirPath)
	c.Assert(err, gc.ErrorMatches, "cannot copy charm to destination: run sudo cp failed")
	c.Assert(unitURLCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestUnitUrl(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "aFakeUnit"
	fakeHostFromTarget := func(_ *CharmSyncCommand, _ string) (string, error) { return "aFakeUnitHost", nil }
	s.PatchValue(&wrapHostFromTarget, fakeHostFromTarget)
	charmUnitURL, err := charmSyncCmd.unitURL()
	c.Assert(err, gc.IsNil)
	c.Assert(charmUnitURL, gc.Equals, "ubuntu@aFakeUnitHost")

}

func (s *CharmSyncSuite) TestUnitUrlFailsMissingToUnit(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = ""
	fakeHostFromTarget := func(_ *CharmSyncCommand, _ string) (string, error) { return "aFakeUnitHost", nil }
	s.PatchValue(&wrapHostFromTarget, fakeHostFromTarget)
	_, err := charmSyncCmd.unitURL()
	c.Assert(err, gc.ErrorMatches, "the unit name must be specified")
}

func (s *CharmSyncSuite) TestUnitUrlFailsHostFromTarget(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "aFakeUnit"
	fakeHostFromTarget := func(_ *CharmSyncCommand, _ string) (string, error) { return "", fmt.Errorf("host from target failed") }
	s.PatchValue(&wrapHostFromTarget, fakeHostFromTarget)
	_, err := charmSyncCmd.unitURL()
	c.Assert(err, gc.ErrorMatches, "host from target failed")
}

func (s *CharmSyncSuite) TestInferCharm(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	pathCharmCalled := false
	fakePathCharm := func(string) (*charm.Dir, error) {
		pathCharmCalled = true
		return &charm.Dir{}, nil
	}
	s.PatchValue(&wrapPathCharm, fakePathCharm)
	charmSyncCmd.charmPath = "/fake/charm/path"
	_, err := charmSyncCmd.inferCharm()
	c.Assert(err, gc.IsNil)
	c.Assert(pathCharmCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestInferCharmNoCharmPath(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.charmPath = ""

	pathCharmCalled := false
	cwdPathCharmCalled := false

	fakePathCharm := func(string) (*charm.Dir, error) {
		pathCharmCalled = true
		return &charm.Dir{}, nil
	}
	fakeCwdPathCharm := func() (*charm.Dir, error) {
		cwdPathCharmCalled = true
		return &charm.Dir{}, nil
	}

	s.PatchValue(&wrapPathCharm, fakePathCharm)
	s.PatchValue(&wrapCwdCharm, fakeCwdPathCharm)
	_, err := charmSyncCmd.inferCharm()
	c.Assert(err, gc.IsNil)
	c.Assert(pathCharmCalled, jc.IsFalse)
	c.Assert(cwdPathCharmCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestInferCharmNoCharmPathCwdFails(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.charmPath = ""

	pathCharmCalled := false
	cwdPathCharmCalled := false

	fakePathCharm := func(string) (*charm.Dir, error) {
		pathCharmCalled = true
		return &charm.Dir{}, nil
	}
	fakeCwdPathCharm := func() (*charm.Dir, error) {
		cwdPathCharmCalled = true
		return &charm.Dir{}, fmt.Errorf("fake testing error of cwdPathCharm")
	}

	s.PatchValue(&wrapPathCharm, fakePathCharm)
	s.PatchValue(&wrapCwdCharm, fakeCwdPathCharm)
	_, err := charmSyncCmd.inferCharm()
	c.Assert(err, gc.ErrorMatches, "charm path not supplied and current work dir cannot be used: fake testing error of cwdPathCharm")
	c.Assert(pathCharmCalled, jc.IsFalse)
	c.Assert(cwdPathCharmCalled, jc.IsTrue)
}

func (s *CharmSyncSuite) TestRemoteUnitPath(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "wordpress/0"
	fakeDataDir := func(series string) (string, error) { return "/a/fake/path", nil }
	s.PatchValue(&wrapDataDir, fakeDataDir)

	remotePath, err := charmSyncCmd.remoteUnitPath("fakeSeries")
	c.Assert(err, gc.IsNil)
	c.Assert(remotePath, gc.Equals, "/a/fake/path/agents/unit-wordpress-0/charm")
}

func (s *CharmSyncSuite) TestRemoteUnitPathDatarDirFails(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "wordpress/0"
	fakeDataDir := func(series string) (string, error) { return "", fmt.Errorf("fake invalid series") }
	s.PatchValue(&wrapDataDir, fakeDataDir)

	_, err := charmSyncCmd.remoteUnitPath("fakeSeries")
	c.Assert(err, gc.ErrorMatches, "cannot determine target data directory: fake invalid series")
}

func (s *CharmSyncSuite) TestRemoteUnitPathErrsOnInvalidUnit(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "invalid unit/0"
	fakeDataDir := func(series string) (string, error) { return "/a/fake/path", nil }
	s.PatchValue(&wrapDataDir, fakeDataDir)

	_, err := charmSyncCmd.remoteUnitPath("fakeSeries")
	c.Assert(err, gc.ErrorMatches, "invalid unit name specified: \"invalid unit/0\"")
}

func (s *CharmSyncSuite) TestRemoteTempPath(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "wordpress/0"
	fakeTempDir := func(series string) (string, error) { return "/a/fake/temp/dir", nil }
	s.PatchValue(&wrapTempDir, fakeTempDir)

	deterministicUUID, err := utils.UUIDFromString("12345678-90ab-4cde-8123-4567890abcde")
	c.Check(err, gc.IsNil)

	fakeNewUUID := func() (utils.UUID, error) { return deterministicUUID, nil }
	s.PatchValue(&wrapNewUUID, fakeNewUUID)

	remoteTempPath, err := charmSyncCmd.remoteTempPath("fakeSeries")
	c.Assert(err, gc.IsNil)
	c.Assert(remoteTempPath, gc.Equals, "/a/fake/temp/dir/charm_sync_12345678-90ab-4cde-8123-4567890abcde")
}

func (s *CharmSyncSuite) TestRemoteTempPathUUIDFails(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "wordpress/0"
	fakeTempDir := func(series string) (string, error) { return "/a/fake/temp/dir", nil }
	s.PatchValue(&wrapTempDir, fakeTempDir)

	fakeNewUUID := func() (utils.UUID, error) { return utils.UUID{}, fmt.Errorf("fake uuid generation error") }
	s.PatchValue(&wrapNewUUID, fakeNewUUID)

	_, err := charmSyncCmd.remoteTempPath("fakeSeries")
	c.Assert(err, gc.ErrorMatches, "cannot generate an UUID for the transient charm folder: fake uuid generation error")
}

func (s *CharmSyncSuite) TestRemoteTempPathTempDirFails(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	charmSyncCmd.toUnit = "wordpress/0"
	fakeTempDir := func(series string) (string, error) { return "", fmt.Errorf("Fake TempDir error") }
	s.PatchValue(&wrapTempDir, fakeTempDir)

	fakeNewUUID := func() (utils.UUID, error) { return utils.UUID{}, nil }
	s.PatchValue(&wrapNewUUID, fakeNewUUID)

	_, err := charmSyncCmd.remoteTempPath("fakeSeries")
	c.Assert(err, gc.ErrorMatches, "cannot generate a remote temp folder name: Fake TempDir error")
}

func (s *CharmSyncSuite) TestUnitPath(c *gc.C) {
	charmSyncCmd := &CharmSyncCommand{}
	fakeUnitUrl := func(_ *CharmSyncCommand) (string, error) {
		return "ubuntu@fakeUnitURL", nil
	}
	s.PatchValue(&wrapUnitURL, fakeUnitUrl)

	fakeRemoteUnitPath := func(_ *CharmSyncCommand, _ string) (string, error) {
		return "/fake/remote/unit", nil
	}
	s.PatchValue(&wrapRemoteUnitPath, fakeRemoteUnitPath)

	unitHostPort, err := charmSyncCmd.unitPath("fakeSeries")
	c.Assert(err, gc.IsNil)
	c.Assert(unitHostPort, gc.Equals, "ubuntu@fakeUnitURL:/fake/remote/unit")
}
