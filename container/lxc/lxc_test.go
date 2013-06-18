// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"path/filepath"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container/lxc"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type LxcSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	home               *testing.FakeHome
	containerDir       string
	lxcDir             string
	oldContainerDir    string
	oldLxcContainerDir string
}

var _ = Suite(&LxcSuite{})

func (s *LxcSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *LxcSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *LxcSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.home = testing.MakeSampleHome(c)
	s.containerDir = c.MkDir()
	s.oldContainerDir = lxc.SetContainerDir(s.containerDir)
	s.lxcDir = c.MkDir()
	s.oldLxcContainerDir = lxc.SetLxcContainerDir(s.lxcDir)
}

func (s *LxcSuite) TearDownTest(c *C) {
	lxc.SetContainerDir(s.oldContainerDir)
	lxc.SetLxcContainerDir(s.oldLxcContainerDir)
	s.home.Restore()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *LxcSuite) TestNewContainer(c *C) {
	factory := lxc.NewFactory(MockFactory())
	container, err := factory.NewContainer("2/lxc/0")
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-2-lxc-0"))
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, Equals, true)
	c.Assert(machineId, Equals, "2/lxc/0")
}

func (s *LxcSuite) TestNewFromExisting(c *C) {
	mock := MockFactory()
	mockLxc := mock.New("machine-1-lxc-0")
	factory := lxc.NewFactory(mock)
	container, err := factory.NewFromExisting(mockLxc)
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-1-lxc-0"))
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, Equals, true)
	c.Assert(machineId, Equals, "1/lxc/0")
}

func (s *LxcSuite) TestContainerCreate(c *C) {

	machineId := "1/lxc/0"
	config := testing.EnvironConfig(c)
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	factory := lxc.NewFactory(MockFactory())
	container, err := factory.NewContainer(machineId)
	c.Assert(err, IsNil)

	series := "series"
	nonce := "fake-nonce"
	tools := &state.Tools{
		Binary: version.MustParseBinary("2.3.4-foo-bar"),
		URL:    "http://tools.example.com/2.3.4-foo-bar.tgz",
	}

	err = container.Create(series, nonce, tools, config, stateInfo, apiInfo)
	c.Assert(err, IsNil)

	name := string(container.Id())
	// Check our container config files.
	testing.AssertNonEmptyFileExists(c, filepath.Join(s.containerDir, name, "lxc.conf"))
	testing.AssertNonEmptyFileExists(c, filepath.Join(s.containerDir, name, "cloud-init"))
	// Check the mount point has been created inside the container.
	testing.AssertDirectoryExists(c, filepath.Join(s.lxcDir, name, "rootfs/var/log/juju"))
}
