package windows_test

import (
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/windows"
)

type serviceSuite struct {
	coretesting.BaseSuite

	name     string
	conf     common.Conf
	execPath string

	stub    *testing.Stub
	stubMgr *windows.StubSvcManager

	svcExistsErr error

	mgr *windows.Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	var err error
	s.execPath = `C:\juju\bin\jujud.exe`
	s.stub = &testing.Stub{}
	s.stubMgr = windows.PatchServiceManager(s, s.stub)

	// Set up the service.
	s.name = "machine-1"
	s.conf = common.Conf{
		Desc:      "service for " + s.name,
		ExecStart: s.execPath + " " + s.name,
	}

	s.svcExistsErr = errors.New("Service machine-1 already installed")

	s.mgr, err = windows.NewService(s.name, s.conf)
	c.Assert(err, gc.IsNil)

	// Clear services
	s.stubMgr.Clear()
	s.stub.ResetCalls()
}

func (s *serviceSuite) TestInstall(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	exists, err := s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)

	s.stub.CheckCallNames(c, "listServices", "Create")
}

func (s *serviceSuite) TestInstallAlreadyExists(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	exists, err := s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)

	err = s.mgr.Install()
	c.Assert(err.Error(), gc.Equals, s.svcExistsErr.Error())

	s.stub.CheckCallNames(c, "listServices", "Create", "listServices")
}

func (s *serviceSuite) TestStop(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	running, err := s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsFalse)

	err = s.mgr.Start()
	c.Assert(err, gc.IsNil)

	running, err = s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsTrue)

	err = s.mgr.Stop()
	c.Assert(err, gc.IsNil)

	running, err = s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsFalse)
}

func (s *serviceSuite) TestStopStart(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	running, err := s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsFalse)

	err = s.mgr.Start()
	c.Assert(err, gc.IsNil)

	running, err = s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsTrue)

	err = s.mgr.Stop()
	c.Assert(err, gc.IsNil)

	running, err = s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsFalse)

	err = s.mgr.Start()
	c.Assert(err, gc.IsNil)

	running, err = s.mgr.Running()
	c.Assert(err, gc.IsNil)
	c.Assert(running, jc.IsTrue)
}

func (s *serviceSuite) TestRemove(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	exists, err := s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)

	err = s.mgr.Remove()
	c.Assert(err, gc.IsNil)

	exists, err = s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsFalse)

	s.stub.CheckCallNames(c, "listServices", "Create", "listServices", "listServices", "Running", "Delete")
}

func (s *serviceSuite) TestRemoveRunningService(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	exists, err := s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)

	err = s.mgr.Start()
	c.Assert(err, gc.IsNil)

	err = s.mgr.Remove()
	c.Assert(err, gc.IsNil)

	exists, err = s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsFalse)

	s.stub.CheckCallNames(c, "listServices", "Create", "listServices", "Running", "Start", "listServices", "listServices", "Running", "Stop", "Delete")
}

func (s *serviceSuite) TestRemoveInexistent(c *gc.C) {
	exists, err := s.stubMgr.Exists(s.name, s.conf)
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsFalse)

	err = s.mgr.Remove()
	c.Assert(err, gc.IsNil)

	s.stub.CheckCallNames(c, "listServices")
}

func (s *serviceSuite) TestInstalled(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	exists, err := s.mgr.Installed()
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)

	s.stub.CheckCallNames(c, "listServices", "Create", "listServices")
}

func (s *serviceSuite) TestInstalledListError(c *gc.C) {
	err := s.mgr.Install()
	c.Assert(err, gc.IsNil)

	listErr := errors.New("random error")
	s.stub.SetErrors(listErr)

	exists, err := s.mgr.Installed()
	c.Assert(err.Error(), gc.Equals, listErr.Error())
	c.Assert(exists, jc.IsFalse)
}
