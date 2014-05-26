// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

func Test(t *testing.T) { gc.TestingT(t) }

type UpstartSuite struct {
	coretesting.BaseSuite
	testPath string
	service  *upstart.Service
}

var _ = gc.Suite(&UpstartSuite{})

func (s *UpstartSuite) SetUpTest(c *gc.C) {
	s.testPath = c.MkDir()
	s.PatchEnvPathPrepend(s.testPath)
	s.PatchValue(&upstart.InstallStartRetryAttempts, utils.AttemptStrategy{})
	s.service = &upstart.Service{Name: "some-service", InitDir: c.MkDir()}
	_, err := os.Create(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
}

func (s *UpstartSuite) StoppedStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *UpstartSuite) RunningStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

func (s *UpstartSuite) TestInitDir(c *gc.C) {
	svc := upstart.NewService("blah")
	c.Assert(svc.InitDir, gc.Equals, "/etc/init")
}

func (s *UpstartSuite) TestInstalled(c *gc.C) {
	c.Assert(s.service.Installed(), gc.Equals, true)
	err := os.Remove(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(s.service.Installed(), gc.Equals, false)
}

func (s *UpstartSuite) TestRunning(c *gc.C) {
	s.MakeTool(c, "status", "exit 1")
	c.Assert(s.service.Running(), gc.Equals, false)
	s.MakeTool(c, "status", `echo "GIBBERISH NONSENSE"`)
	c.Assert(s.service.Running(), gc.Equals, false)
	s.RunningStatus(c)
	c.Assert(s.service.Running(), gc.Equals, true)
}

func (s *UpstartSuite) TestStart(c *gc.C) {
	s.RunningStatus(c)
	s.MakeTool(c, "start", "exit 99")
	c.Assert(s.service.Start(), gc.IsNil)
	s.StoppedStatus(c)
	c.Assert(s.service.Start(), gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	c.Assert(s.service.Start(), gc.IsNil)
}

func (s *UpstartSuite) TestStop(c *gc.C) {
	s.StoppedStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Stop(), gc.IsNil)
	s.RunningStatus(c)
	c.Assert(s.service.Stop(), gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Stop(), gc.IsNil)
}

func (s *UpstartSuite) TestRemoveMissing(c *gc.C) {
	err := os.Remove(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(s.service.StopAndRemove(), gc.IsNil)
}

func (s *UpstartSuite) TestRemoveStopped(c *gc.C) {
	s.StoppedStatus(c)
	c.Assert(s.service.StopAndRemove(), gc.IsNil)
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *UpstartSuite) TestRemoveRunning(c *gc.C) {
	s.RunningStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.StopAndRemove(), gc.ErrorMatches, ".*exit status 99.*")
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, gc.IsNil)
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.StopAndRemove(), gc.IsNil)
	_, err = os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *UpstartSuite) TestStopAndRemove(c *gc.C) {
	s.RunningStatus(c)
	s.MakeTool(c, "stop", "exit 99")

	// StopAndRemove will fail, as it calls stop.
	c.Assert(s.service.StopAndRemove(), gc.ErrorMatches, ".*exit status 99.*")
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, gc.IsNil)

	// Plain old Remove will succeed.
	c.Assert(s.service.Remove(), gc.IsNil)
	_, err = os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *UpstartSuite) TestInstallErrors(c *gc.C) {
	conf := &upstart.Conf{}
	check := func(msg string) {
		c.Assert(conf.Install(), gc.ErrorMatches, msg)
		_, err := conf.InstallCommands()
		c.Assert(err, gc.ErrorMatches, msg)
	}
	check("missing Name")
	conf.Name = "some-service"
	check("missing InitDir")
	conf.InitDir = c.MkDir()
	check("missing Desc")
	conf.Desc = "this is an upstart service"
	check("missing Cmd")
}

const expectStart = `description "this is an upstart service"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
`

func (s *UpstartSuite) dummyConf(c *gc.C) *upstart.Conf {
	return &upstart.Conf{
		Service: *s.service,
		Desc:    "this is an upstart service",
		Cmd:     "do something",
	}
}

func (s *UpstartSuite) assertInstall(c *gc.C, conf *upstart.Conf, expectEnd string) {
	expectContent := expectStart + expectEnd
	expectPath := filepath.Join(conf.InitDir, "some-service.conf")

	cmds, err := conf.InstallCommands()
	c.Assert(err, gc.IsNil)
	c.Assert(cmds, gc.DeepEquals, []string{
		"cat >> " + expectPath + " << 'EOF'\n" + expectContent + "EOF\n",
		"start some-service",
	})

	s.MakeTool(c, "start", "exit 99")
	err = conf.Install()
	c.Assert(err, gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	err = conf.Install()
	c.Assert(err, gc.IsNil)
	content, err := ioutil.ReadFile(expectPath)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, expectContent)
}

func (s *UpstartSuite) TestInstallSimple(c *gc.C) {
	conf := s.dummyConf(c)
	s.assertInstall(c, conf, "\n\nexec do something\n")
}

func (s *UpstartSuite) TestInstallOutput(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Out = "/some/output/path"
	s.assertInstall(c, conf, "\n\nexec do something >> /some/output/path 2>&1\n")
}

func (s *UpstartSuite) TestInstallEnv(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Env = map[string]string{"FOO": "bar baz", "QUX": "ping pong"}
	s.assertInstall(c, conf, `env FOO="bar baz"
env QUX="ping pong"


exec do something
`)
}

func (s *UpstartSuite) TestInstallLimit(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Limit = map[string]string{"nofile": "65000 65000", "nproc": "20000 20000"}
	s.assertInstall(c, conf, `
limit nofile 65000 65000
limit nproc 20000 20000

exec do something
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
	err := os.Symlink(pathTo("status-started"), pathTo("status"))
	c.Assert(err, gc.IsNil)

	conf := s.dummyConf(c)
	err = conf.Install()
	c.Assert(err, gc.IsNil)
	c.Assert(&conf.Service, jc.Satisfies, (*upstart.Service).Running)
}
