// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping upstart tests on windows")
	}
	gc.TestingT(t)
}

type UpstartSuite struct {
	coretesting.BaseSuite
	testPath string
	service  *upstart.Service
	initDir  string
}

var _ = gc.Suite(&UpstartSuite{})

func (s *UpstartSuite) SetUpTest(c *gc.C) {
	s.testPath = c.MkDir()
	s.initDir = c.MkDir()
	s.PatchEnvPathPrepend(s.testPath)
	s.PatchValue(&upstart.InitDir, s.initDir)
	s.service = upstart.NewService(
		"some-service",
		common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command",
		},
	)
}

var checkargs = `
#!/bin/bash --norc
if [ "$1" != "--system" ]; then
  exit 255
fi
if [ "$2" != "some-service" ]; then
  exit 255
fi
if [ "$3" != "" ]; then
  exit 255
fi
`[1:]

func (s *UpstartSuite) MakeTool(c *gc.C, name, script string) {
	path := filepath.Join(s.testPath, name)
	err := ioutil.WriteFile(path, []byte(checkargs+script), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpstartSuite) StoppedStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *UpstartSuite) RunningStatusNoProcessID(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running"`)
}

func (s *UpstartSuite) RunningStatusWithProcessID(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

func (s *UpstartSuite) goodInstall(c *gc.C) {
	s.MakeTool(c, "start", "exit 0")
	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpstartSuite) TestInstalled(c *gc.C) {
	installed, err := s.service.Installed()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(installed, jc.IsFalse)

	s.goodInstall(c)
	installed, err = s.service.Installed()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(installed, jc.IsTrue)
}

func (s *UpstartSuite) TestExists(c *gc.C) {
	// Setup creates the file, but it is empty.
	exists, err := s.service.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsFalse)

	s.goodInstall(c)
	exists, err = s.service.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsTrue)
}

func (s *UpstartSuite) TestExistsNonEmpty(c *gc.C) {
	s.goodInstall(c)
	s.service.Service.Conf.ExecStart = "/path/to/other-command"

	exists, err := s.service.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsFalse)
}

func (s *UpstartSuite) TestRunning(c *gc.C) {
	s.MakeTool(c, "status", "exit 1")
	running, err := s.service.Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsFalse)

	s.MakeTool(c, "status", `echo "GIBBERISH NONSENSE"`)
	running, err = s.service.Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsFalse)

	s.RunningStatusNoProcessID(c)
	running, err = s.service.Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsTrue)

	s.RunningStatusWithProcessID(c)
	running, err = s.service.Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsTrue)
}

func (s *UpstartSuite) TestStart(c *gc.C) {
	s.RunningStatusWithProcessID(c)
	s.MakeTool(c, "start", "exit 99")
	c.Assert(s.service.Start(), jc.ErrorIsNil)
	s.StoppedStatus(c)
	c.Assert(s.service.Start(), gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	c.Assert(s.service.Start(), jc.ErrorIsNil)
}

func (s *UpstartSuite) TestStop(c *gc.C) {
	s.StoppedStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Stop(), jc.ErrorIsNil)
	s.RunningStatusWithProcessID(c)
	c.Assert(s.service.Stop(), gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Stop(), jc.ErrorIsNil)
}

func (s *UpstartSuite) TestRemoveMissing(c *gc.C) {
	err := s.service.Remove()

	c.Check(err, jc.ErrorIsNil)
}

func (s *UpstartSuite) TestRemoveStopped(c *gc.C) {
	s.goodInstall(c)
	s.StoppedStatus(c)

	err := s.service.Remove()
	c.Assert(err, jc.ErrorIsNil)

	filename := filepath.Join(upstart.InitDir, "some-service.conf")
	_, err = os.Stat(filename)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *UpstartSuite) TestStopRunning(c *gc.C) {
	s.goodInstall(c)
	s.RunningStatusWithProcessID(c)
	s.MakeTool(c, "stop", "exit 99")
	filename := filepath.Join(upstart.InitDir, "some-service.conf")
	err := s.service.Stop()
	c.Assert(err, gc.ErrorMatches, ".*exit status 99.*")

	_, err = os.Stat(filename)
	c.Assert(err, jc.ErrorIsNil)

	s.MakeTool(c, "stop", "exit 0")
	err = s.service.Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpstartSuite) TestInstallErrors(c *gc.C) {
	conf := common.Conf{}
	check := func(msg string) {
		c.Assert(s.service.Install(), gc.ErrorMatches, msg)
		_, err := s.service.InstallCommands()
		c.Assert(err, gc.ErrorMatches, msg)
	}
	s.service.Service.Conf = conf
	s.service.Service.Name = ""
	check("missing Name")
	s.service.Service.Name = "some-service"
	check("missing Desc")
	s.service.Service.Conf.Desc = "this is an upstart service"
	check("missing ExecStart")
}

const expectStart = `description "this is an upstart service"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
`

func (s *UpstartSuite) dummyConf(c *gc.C) common.Conf {
	return common.Conf{
		Desc:      "this is an upstart service",
		ExecStart: "/path/to/some-command x y z",
	}
}

func (s *UpstartSuite) assertInstall(c *gc.C, conf common.Conf, expectEnd string) {
	expectContent := expectStart + expectEnd
	expectPath := filepath.Join(upstart.InitDir, "some-service.conf")

	s.service.Service.Conf = conf
	svc := s.service
	cmds, err := s.service.InstallCommands()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmds, gc.DeepEquals, []string{
		"cat > " + expectPath + " << 'EOF'\n" + expectContent + "EOF\n",
	})
	cmds, err = s.service.StartCommands()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmds, gc.DeepEquals, []string{
		"start some-service",
	})

	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
	s.MakeTool(c, "start", "exit 99")
	err = svc.Install()
	c.Assert(err, jc.ErrorIsNil)
	err = svc.Start()
	c.Assert(err, gc.ErrorMatches, ".*exit status 99.*")

	s.MakeTool(c, "start", "exit 0")
	err = svc.Install()
	c.Assert(err, jc.ErrorIsNil)
	err = svc.Start()
	c.Assert(err, jc.ErrorIsNil)

	content, err := ioutil.ReadFile(expectPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, expectContent)
}

func (s *UpstartSuite) TestInstallSimple(c *gc.C) {
	conf := s.dummyConf(c)
	s.assertInstall(c, conf, `

script


  exec /path/to/some-command x y z
end script
`)
}

func (s *UpstartSuite) TestInstallExtraScript(c *gc.C) {
	conf := s.dummyConf(c)
	conf.ExtraScript = "extra lines of script"
	s.assertInstall(c, conf, `

script
extra lines of script

  exec /path/to/some-command x y z
end script
`)
}

func (s *UpstartSuite) TestInstallLogfile(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Logfile = "/some/output/path"
	s.assertInstall(c, conf, `

script


  # Ensure log files are properly protected
  touch /some/output/path
  chown syslog:syslog /some/output/path
  chmod 0600 /some/output/path

  exec /path/to/some-command x y z >> /some/output/path 2>&1
end script
`)
}

func (s *UpstartSuite) TestInstallEnv(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Env = map[string]string{"FOO": "bar baz", "QUX": "ping pong"}
	s.assertInstall(c, conf, `env FOO="bar baz"
env QUX="ping pong"


script


  exec /path/to/some-command x y z
end script
`)
}

func (s *UpstartSuite) TestInstallLimit(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Limit = map[string]int{
		"nofile": 65000,
		"nproc":  20000,
	}
	s.assertInstall(c, conf, `
limit nofile 65000 65000
limit nproc 20000 20000

script


  exec /path/to/some-command x y z
end script
`)
}

func (s *UpstartSuite) TestInstallAlreadyRunning(c *gc.C) {
	pathTo := func(name string) string {
		return filepath.Join(s.testPath, name)
	}
	s.MakeTool(c, "status-stopped", `echo "some-service stop/waiting"`)
	s.MakeTool(c, "status-started", `echo "some-service start/running, process 123"`)
	s.MakeTool(c, "stop", fmt.Sprintf(
		"rm %s; ln -s %s %s",
		pathTo("status"), pathTo("status-stopped"), pathTo("status"),
	))
	s.MakeTool(c, "start", fmt.Sprintf(
		"rm %s; ln -s %s %s",
		pathTo("status"), pathTo("status-started"), pathTo("status"),
	))
	err := symlink.New(pathTo("status-started"), pathTo("status"))
	c.Assert(err, jc.ErrorIsNil)

	svc := upstart.NewService("some-service", s.dummyConf(c))
	err = svc.Install()
	c.Assert(err, jc.ErrorIsNil)
	installed, err := svc.Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(installed, jc.IsTrue)
}

type IsRunningSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&IsRunningSuite{})

const modeExecutable = 0500
const modeNotExecutable = 0400

// createInitctl creates a dummy initctl which returns the given
// exitcode and patches the upstart package to use it.
func (s *IsRunningSuite) createInitctl(c *gc.C, stderr string, exitcode int, mode os.FileMode) {
	path := filepath.Join(c.MkDir(), "initctl")
	var body string
	if stderr != "" {
		// Write to stderr.
		body = ">&2 echo " + utils.ShQuote(stderr)
	}
	script := fmt.Sprintf(`
#!/usr/bin/env bash
%s
exit %d
`[1:], body, exitcode)
	c.Logf(script)
	err := ioutil.WriteFile(path, []byte(script), mode)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(upstart.InitctlPath, path)
}

func (s *IsRunningSuite) TestUpstartInstalled(c *gc.C) {
	s.createInitctl(c, "", 0, modeExecutable)

	isUpstart, err := upstart.IsRunning()
	c.Assert(isUpstart, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *IsRunningSuite) TestUpstartNotInstalled(c *gc.C) {
	s.PatchValue(upstart.InitctlPath, "/foo/bar/not-exist")

	isUpstart, err := upstart.IsRunning()
	c.Assert(isUpstart, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *IsRunningSuite) TestUpstartInstalledButBroken(c *gc.C) {
	const stderr = "<something broke>"
	const errorCode = 99
	s.createInitctl(c, stderr, errorCode, modeExecutable)

	isUpstart, err := upstart.IsRunning()
	c.Assert(isUpstart, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*exit status %d", errorCode))
}

func (s *IsRunningSuite) TestUpstartInstalledButNotRunning(c *gc.C) {
	const stderr = `Name "com.ubuntu.Upstart" does not exist`
	const errorCode = 1
	s.createInitctl(c, stderr, errorCode, modeExecutable)

	isUpstart, err := upstart.IsRunning()
	c.Assert(isUpstart, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*exit status %d", errorCode))
}

func (s *IsRunningSuite) TestInitctlCantBeRun(c *gc.C) {
	s.createInitctl(c, "", 0, modeNotExecutable)

	isUpstart, err := upstart.IsRunning()
	c.Assert(isUpstart, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, ".+: permission denied")
}
