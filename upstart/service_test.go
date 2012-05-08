package upstart_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/upstart"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServiceSuite struct {
	origPath string
	testPath string
	service  *upstart.Service
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpTest(c *C) {
	s.origPath = os.Getenv("PATH")
	s.testPath = c.MkDir()
	os.Setenv("PATH", s.testPath+":"+s.origPath)
	s.service = &upstart.Service{Name: "some-service", InitDir: c.MkDir()}
	_, err := os.Create(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) MakeTool(c *C, name, script string) {
	path := filepath.Join(s.testPath, name)
	err := ioutil.WriteFile(path, []byte("#!/bin/bash\n"+script), 0755)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.origPath)
}

func (s *ServiceSuite) TestInitDir(c *C) {
	svc := upstart.NewService("blah")
	c.Assert(svc.InitDir, Equals, "/etc/init")
}

func (s *ServiceSuite) TestInstalled(c *C) {
	c.Assert(s.service.Installed(), Equals, true)
	err := os.Remove(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
	c.Assert(s.service.Installed(), Equals, false)
}

func (s *ServiceSuite) TestRunning(c *C) {
	s.MakeTool(c, "status", "exit 1")
	c.Assert(s.service.Running(), Equals, false)
	s.MakeTool(c, "status", `echo "GIBBERISH NONSENSE"`)
	c.Assert(s.service.Running(), Equals, false)
	s.MakeTool(c, "status", `echo "some-service start/running, process 12345"`)
	c.Assert(s.service.Running(), Equals, true)
}

func (s *ServiceSuite) TestStable(c *C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process $RANDOM"`)
	c.Assert(s.service.Running(), Equals, true)
	c.Assert(s.service.Stable(), Equals, false)
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
	c.Assert(s.service.Running(), Equals, true)
	c.Assert(s.service.Stable(), Equals, true)
}

func (s *ServiceSuite) TestStart(c *C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
	s.MakeTool(c, "start", "exit 99")
	c.Assert(s.service.Start(), IsNil)
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
	c.Assert(s.service.Start(), ErrorMatches, "exit status 99")
	s.MakeTool(c, "start", "exit 0")
	c.Assert(s.service.Start(), IsNil)
}

func (s *ServiceSuite) TestStop(c *C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Stop(), IsNil)
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
	c.Assert(s.service.Stop(), ErrorMatches, "exit status 99")
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Stop(), IsNil)
}

func (s *ServiceSuite) TestRemoveMissing(c *C) {
	err := os.Remove(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
	c.Assert(s.service.Remove(), IsNil)
}

func (s *ServiceSuite) TestRemoveStopped(c *C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
	c.Assert(s.service.Remove(), IsNil)
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *ServiceSuite) TestRemoveRunning(c *C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Remove(), ErrorMatches, "exit status 99")
	_, err := os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(err, IsNil)
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Remove(), IsNil)
	_, err = os.Stat(filepath.Join(s.service.InitDir, "some-service.conf"))
	c.Assert(os.IsNotExist(err), Equals, true)
}
