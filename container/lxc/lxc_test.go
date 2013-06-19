// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/container/lxc/mock"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
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
	lxc.SetRemovedContainerDir(s.oldRemovedDir)
	s.LoggingSuite.TearDownTest(c)
}

func StartContainer(c *C, manager lxc.ContainerManager, machineId string) instance.Instance {
	config := testing.EnvironConfig(c)
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	series := "series"
	nonce := "fake-nonce"
	tools := &state.Tools{
		Binary: version.MustParseBinary("2.3.4-foo-bar"),
		URL:    "http://tools.example.com/2.3.4-foo-bar.tgz",
	}

	inst, err := manager.StartContainer(machineId, series, nonce, tools, config, stateInfo, apiInfo)
	c.Assert(err, IsNil)
	return inst
}

func (s *LxcSuite) TestStartContainer(c *C) {
	manager := lxc.NewContainerManager(mock.MockFactory(), "")
	instance := StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	// Check our container config files.
	testing.AssertNonEmptyFileExists(c, filepath.Join(s.containerDir, name, "lxc.conf"))
	testing.AssertNonEmptyFileExists(c, filepath.Join(s.containerDir, name, "cloud-init"))
	// Check the mount point has been created inside the container.
	testing.AssertDirectoryExists(c, filepath.Join(s.lxcDir, name, "rootfs/var/log/juju"))
}

func (s *LxcSuite) TestStopContainer(c *C) {
	manager := lxc.NewContainerManager(mock.MockFactory(), "")
	instance := StartContainer(c, manager, "1/lxc/0")

	err := manager.StopContainer(instance)
	c.Assert(err, IsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	testing.AssertDirectoryDoesNotExist(c, filepath.Join(s.containerDir, name))
	// but instead, in the removed container dir
	testing.AssertDirectoryExists(c, filepath.Join(s.removedDir, name))
}

func (s *LxcSuite) TestStopContainerNameClash(c *C) {
	manager := lxc.NewContainerManager(mock.MockFactory(), "")
	instance := StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	targetDir := filepath.Join(s.removedDir, name)
	err := os.MkdirAll(targetDir, 0755)
	c.Assert(err, IsNil)

	err = manager.StopContainer(instance)
	c.Assert(err, IsNil)

	// Check that the container dir is no longer in the container dir
	testing.AssertDirectoryDoesNotExist(c, filepath.Join(s.containerDir, name))
	// but instead, in the removed container dir with a ".1" suffix as there was already a directory there.
	testing.AssertDirectoryExists(c, filepath.Join(s.removedDir, fmt.Sprintf("%s.1", name)))
}

func (s *LxcSuite) TestNamedManagerPrefix(c *C) {
	manager := lxc.NewContainerManager(mock.MockFactory(), "eric")
	instance := StartContainer(c, manager, "1/lxc/0")
	c.Assert(string(instance.Id()), Equals, "eric-machine-1-lxc-0")
}

func (s *LxcSuite) TestListContainers(c *C) {
	factory := mock.MockFactory()
	foo := lxc.NewContainerManager(factory, "foo")
	bar := lxc.NewContainerManager(factory, "bar")

	foo1 := StartContainer(c, foo, "1/lxc/0")
	foo2 := StartContainer(c, foo, "1/lxc/1")
	foo3 := StartContainer(c, foo, "1/lxc/2")

	bar1 := StartContainer(c, bar, "1/lxc/0")
	bar2 := StartContainer(c, bar, "1/lxc/1")

	result, err := foo.ListContainers()
	c.Assert(err, IsNil)
	testing.MatchInstances(c, result, foo1, foo2, foo3)

	result, err = bar.ListContainers()
	c.Assert(err, IsNil)
	testing.MatchInstances(c, result, bar1, bar2)
}
