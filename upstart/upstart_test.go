// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/upstart"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type UpstartSuite struct {
	origPath string
	testPath string
	service  *upstart.Service
}

var _ = Suite(&UpstartSuite{})

func (s *UpstartSuite) SetUpTest(c *C) {
	s.origPath = os.Getenv("PATH")
	s.testPath = c.MkDir()
	os.Setenv("PATH", s.testPath+":"+s.origPath)
	s.service = &upstart.Service{Name: "some-service", InitDir: c.MkDir()}
	_, err := os.Create(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
}

func (s *UpstartSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.origPath)
}

var checkargs = `
#!/bin/bash
if [ "$1" != "some-service" ]; then
  exit 255
fi
if [ "$2" != "" ]; then
  exit 255
fi
`[1:]

func (s *UpstartSuite) MakeTool(c *C, name, script string) {
	path := filepath.Join(s.testPath, name)
	err := ioutil.WriteFile(path, []byte(checkargs+script), 0755)
	c.Assert(err, IsNil)
}

func (s *UpstartSuite) StoppedStatus(c *C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *UpstartSuite) RunningStatus(c *C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

func (s *UpstartSuite) TestInitDir(c *C) {
	svc := upstart.NewService("blah")
	c.Assert(svc.InitDir, Equals, "/etc/init")
}

func (s *UpstartSuite) TestInstalled(c *C) {
	c.Assert(s.service.Installed(), Equals, true)
	err := os.Remove(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
	c.Assert(s.service.Installed(), Equals, false)
}

func (s *UpstartSuite) TestRunning(c *C) {
	s.MakeTool(c, "status", "exit 1")
	c.Assert(s.service.Running(), Equals, false)
	s.MakeTool(c, "status", `echo "GIBBERISH NONSENSE"`)
	c.Assert(s.service.Running(), Equals, false)
	s.RunningStatus(c)
	c.Assert(s.service.Running(), Equals, true)
}

func (s *UpstartSuite) TestStart(c *C) {
	s.RunningStatus(c)
	s.MakeTool(c, "start", "exit 99")
	c.Assert(s.service.Start(), IsNil)
	s.StoppedStatus(c)
	c.Assert(s.service.Start(), ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	c.Assert(s.service.Start(), IsNil)
}

func (s *UpstartSuite) TestStop(c *C) {
	s.StoppedStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Stop(), IsNil)
	s.RunningStatus(c)
	c.Assert(s.service.Stop(), ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Stop(), IsNil)
}

func (s *UpstartSuite) TestRemoveMissing(c *C) {
	err := os.Remove(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
	c.Assert(s.service.Remove(), IsNil)
}

func (s *UpstartSuite) TestRemoveStopped(c *C) {
	s.StoppedStatus(c)
	c.Assert(s.service.Remove(), IsNil)
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
}

func (s *UpstartSuite) TestRemoveRunning(c *C) {
	s.RunningStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Remove(), ErrorMatches, ".*exit status 99.*")
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Remove(), IsNil)
	_, err = os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
}

func (s *UpstartSuite) TestInstallErrors(c *C) {
	conf := &upstart.Conf{}
	check := func(msg string) {
		c.Assert(conf.Install(), ErrorMatches, msg)
		_, err := conf.InstallCommands()
		c.Assert(err, ErrorMatches, msg)
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

func (s *UpstartSuite) dummyConf(c *C) *upstart.Conf {
	return &upstart.Conf{
		Service: *s.service,
		Desc:    "this is an upstart service",
		Cmd:     "do something",
	}
}

func (s *UpstartSuite) assertInstall(c *C, conf *upstart.Conf, expectEnd string) {
	expectContent := expectStart + expectEnd
	expectPath := filepath.Join(conf.InitDir, "some-service.conf")

	cmds, err := conf.InstallCommands()
	c.Assert(err, IsNil)
	c.Assert(cmds, DeepEquals, []string{
		"cat >> " + expectPath + " << 'EOF'\n" + expectContent + "EOF\n",
		"start some-service",
	})

	s.MakeTool(c, "start", "exit 99")
	err = conf.Install()
	c.Assert(err, ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	err = conf.Install()
	c.Assert(err, IsNil)
	content, err := ioutil.ReadFile(expectPath)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, expectContent)
}

func (s *UpstartSuite) TestInstallSimple(c *C) {
	conf := s.dummyConf(c)
	s.assertInstall(c, conf, "\n\nexec do something\n")
}

func (s *UpstartSuite) TestInstallOutput(c *C) {
	conf := s.dummyConf(c)
	conf.Out = "/some/output/path"
	s.assertInstall(c, conf, "\n\nexec do something >> /some/output/path 2>&1\n")
}

func (s *UpstartSuite) TestInstallEnv(c *C) {
	conf := s.dummyConf(c)
	conf.Env = map[string]string{"FOO": "bar baz", "QUX": "ping pong"}
	s.assertInstall(c, conf, `env FOO="bar baz"
env QUX="ping pong"


exec do something
`)
}

func (s *UpstartSuite) TestInstallLimit(c *C) {
	conf := s.dummyConf(c)
	conf.Limit = map[string]string{"nofile": "65000 65000", "nproc": "20000 20000"}
	s.assertInstall(c, conf, `
limit nofile 65000 65000
limit nproc 20000 20000

exec do something
`)
}

func (s *UpstartSuite) TestInstallAlreadyRunning(c *C) {
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
	c.Assert(err, IsNil)

	conf := s.dummyConf(c)
	err = conf.Install()
	c.Assert(err, IsNil)
	c.Assert(&conf.Service, checkers.Satisfies, (*upstart.Service).Running)
}
