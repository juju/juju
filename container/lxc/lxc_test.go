// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/container/lxc"
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
	lxc.TestSuite
}

var _ = Suite(&LxcSuite{})

func (s *LxcSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *LxcSuite) TearDownSuite(c *C) {
	s.TestSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *LxcSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
}

func (s *LxcSuite) TearDownTest(c *C) {
	s.TestSuite.TearDownTest(c)
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
		URL:    "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}

	inst, err := manager.StartContainer(machineId, series, nonce, tools, config, stateInfo, apiInfo)
	c.Assert(err, IsNil)
	return inst
}

func (s *LxcSuite) TestStartContainer(c *C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{})
	instance := StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	// Check our container config files.
	c.Assert(filepath.Join(s.ContainerDir, name, "lxc.conf"), IsNonEmptyFile)
	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	c.Assert(cloudInitFilename, IsNonEmptyFile)
	data, err := ioutil.ReadFile(cloudInitFilename)
	c.Assert(err, IsNil)
	c.Assert(string(data), HasPrefix, "#cloud-config\n")

	x := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &x)
	c.Assert(err, IsNil)

	var scripts []string
	for _, s := range x["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}

	c.Assert(scripts[len(scripts)-1], Equals, "start jujud-machine-1-lxc-0")

	// Check the mount point has been created inside the container.
	c.Assert(filepath.Join(s.LxcDir, name, "rootfs/var/log/juju"), IsDirectory)
}

func (s *LxcSuite) TestStopContainer(c *C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{})
	instance := StartContainer(c, manager, "1/lxc/0")

	err := manager.StopContainer(instance)
	c.Assert(err, IsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), IsDirectory)
}

func (s *LxcSuite) TestStopContainerNameClash(c *C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{})
	instance := StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	targetDir := filepath.Join(s.RemovedDir, name)
	err := os.MkdirAll(targetDir, 0755)
	c.Assert(err, IsNil)

	err = manager.StopContainer(instance)
	c.Assert(err, IsNil)

	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), DoesNotExist)
	// but instead, in the removed container dir with a ".1" suffix as there was already a directory there.
	c.Assert(filepath.Join(s.RemovedDir, fmt.Sprintf("%s.1", name)), IsDirectory)
}

func (s *LxcSuite) TestNamedManagerPrefix(c *C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{Name: "eric"})
	instance := StartContainer(c, manager, "1/lxc/0")
	c.Assert(string(instance.Id()), Equals, "eric-machine-1-lxc-0")
}

func (s *LxcSuite) TestListContainers(c *C) {
	foo := lxc.NewContainerManager(lxc.ManagerConfig{Name: "foo"})
	bar := lxc.NewContainerManager(lxc.ManagerConfig{Name: "bar"})

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
