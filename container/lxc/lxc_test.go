// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/container/lxc/mock"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

func Test(t *stdtesting.T) {
	TestingT(t)
}

type LxcSuite struct {
	testing.LoggingSuite
	containerDir       string
	removedDir         string
	lxcDir             string
	oldContainerDir    string
	oldRemovedDir      string
	oldLxcContainerDir string
}

var _ = Suite(&LxcSuite{})

func (s *LxcSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *LxcSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *LxcSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.oldContainerDir = lxc.SetContainerDir(s.containerDir)
	s.removedDir = c.MkDir()
	s.oldRemovedDir = lxc.SetRemovedContainerDir(s.removedDir)
	s.lxcDir = c.MkDir()
	s.oldLxcContainerDir = lxc.SetLxcContainerDir(s.lxcDir)
}

func (s *LxcSuite) TearDownTest(c *C) {
	lxc.SetContainerDir(s.oldContainerDir)
	lxc.SetLxcContainerDir(s.oldLxcContainerDir)
	s.LoggingSuite.TearDownTest(c)
}

func (s *LxcSuite) TestNewContainer(c *C) {
	factory := lxc.NewFactory(mock.MockFactory())
	container, err := factory.NewContainer("2/lxc/0")
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-2-lxc-0"))
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, IsTrue)
	c.Assert(machineId, Equals, "2/lxc/0")
}

func (s *LxcSuite) TestNewFromExisting(c *C) {
	mock := mock.MockFactory()
	mockLxc := mock.New("machine-1-lxc-0")
	factory := lxc.NewFactory(mock)
	container, err := factory.NewFromExisting(mockLxc)
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-1-lxc-0"))
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, IsTrue)
	c.Assert(machineId, Equals, "1/lxc/0")
}

func ContainerCreate(c *C, container container.Container) {
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, IsTrue)
	config := testing.EnvironConfig(c)
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	series := "series"
	nonce := "fake-nonce"
	tools := &state.Tools{
		Binary: version.MustParseBinary("2.3.4-foo-bar"),
		URL:    "http://tools.example.com/2.3.4-foo-bar.tgz",
	}

	err := container.Create(series, nonce, tools, config, stateInfo, apiInfo)
	c.Assert(err, IsNil)
}

func (s *LxcSuite) TestContainerCreate(c *C) {

	factory := lxc.NewFactory(mock.MockFactory())
	container, err := factory.NewContainer("1/lxc/0")
	c.Assert(err, IsNil)

	ContainerCreate(c, container)

	name := string(container.Id())
	// Check our container config files.
	testing.AssertNonEmptyFileExists(c, filepath.Join(s.containerDir, name, "lxc.conf"))
	testing.AssertNonEmptyFileExists(c, filepath.Join(s.containerDir, name, "cloud-init"))
	// Check the mount point has been created inside the container.
	testing.AssertDirectoryExists(c, filepath.Join(s.lxcDir, name, "rootfs/var/log/juju"))
}

func (s *LxcSuite) TestContainerDestroy(c *C) {
	factory := lxc.NewFactory(mock.MockFactory())
	container, err := factory.NewContainer("1/lxc/0")
	c.Assert(err, IsNil)

	ContainerCreate(c, container)
	err = container.Destroy()
	c.Assert(err, IsNil)

	name := string(container.Id())
	// Check that the container dir is no longer in the container dir
	testing.AssertDirectoryDoesNotExist(c, filepath.Join(s.containerDir, name))
	// but instead, in the removed container dir
	testing.AssertDirectoryExists(c, filepath.Join(s.removedDir, name))
}

func (s *LxcSuite) TestContainerRemovedDirNameClash(c *C) {
	factory := lxc.NewFactory(mock.MockFactory())
	container, err := factory.NewContainer("1/lxc/0")
	c.Assert(err, IsNil)

	name := string(container.Id())
	targetDir := filepath.Join(s.removedDir, name)
	err = os.MkdirAll(targetDir, 0755)
	c.Assert(err, IsNil)

	ContainerCreate(c, container)
	err = container.Destroy()
	c.Assert(err, IsNil)

	// Check that the container dir is no longer in the container dir
	testing.AssertDirectoryDoesNotExist(c, filepath.Join(s.containerDir, name))
	// but instead, in the removed container dir with a ".1" suffix as there was already a directory there.
	testing.AssertDirectoryExists(c, filepath.Join(s.removedDir, fmt.Sprintf("%s.1", name)))
}
