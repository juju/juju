// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type LxcSuite struct {
	testing.LoggingSuite
	lxc.TestSuite
}

var _ = gc.Suite(&LxcSuite{})

func (s *LxcSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *LxcSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *LxcSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	loggo.GetLogger("juju.container.lxc").SetLogLevel(loggo.TRACE)
}

func (s *LxcSuite) TearDownTest(c *gc.C) {
	s.TestSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func StartContainer(c *gc.C, manager lxc.ContainerManager, machineId string) instance.Instance {
	config := testing.EnvironConfig(c)
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	network := lxc.BridgeNetworkConfig("nic42")

	series := "series"
	nonce := "fake-nonce"
	tools := &tools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}

	inst, err := manager.StartContainer(machineId, series, nonce, network, tools, config, stateInfo, apiInfo)
	c.Assert(err, gc.IsNil)
	return inst
}

func (s *LxcSuite) TestStartContainer(c *gc.C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{})
	instance := StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	// Check our container config files.
	lxcConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, name, "lxc.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.link = nic42")

	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	c.Assert(cloudInitFilename, jc.IsNonEmptyFile)
	data, err := ioutil.ReadFile(cloudInitFilename)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), jc.HasPrefix, "#cloud-config\n")

	x := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &x)
	c.Assert(err, gc.IsNil)

	var scripts []string
	for _, s := range x["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}

	c.Assert(scripts[len(scripts)-2], gc.Equals, "start jujud-machine-1-lxc-0")
	c.Assert(scripts[len(scripts)-1], gc.Equals, "ifconfig")

	// Check the mount point has been created inside the container.
	c.Assert(filepath.Join(s.LxcDir, name, "rootfs/var/log/juju"), jc.IsDirectory)
	// Check that the config file is linked in the restart dir.
	expectedLinkLocation := filepath.Join(s.RestartDir, name+".conf")
	expectedTarget := filepath.Join(s.LxcDir, name, "config")
	linkInfo, err := os.Lstat(expectedLinkLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(linkInfo.Mode()&os.ModeSymlink, gc.Equals, os.ModeSymlink)

	location, err := os.Readlink(expectedLinkLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(location, gc.Equals, expectedTarget)
}

func (s *LxcSuite) TestStopContainer(c *gc.C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{})
	instance := StartContainer(c, manager, "1/lxc/0")

	err := manager.StopContainer(instance)
	c.Assert(err, gc.IsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), jc.IsDirectory)
}

func (s *LxcSuite) TestStopContainerNameClash(c *gc.C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{})
	instance := StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	targetDir := filepath.Join(s.RemovedDir, name)
	err := os.MkdirAll(targetDir, 0755)
	c.Assert(err, gc.IsNil)

	err = manager.StopContainer(instance)
	c.Assert(err, gc.IsNil)

	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir with a ".1" suffix as there was already a directory there.
	c.Assert(filepath.Join(s.RemovedDir, fmt.Sprintf("%s.1", name)), jc.IsDirectory)
}

func (s *LxcSuite) TestNamedManagerPrefix(c *gc.C) {
	manager := lxc.NewContainerManager(lxc.ManagerConfig{Name: "eric"})
	instance := StartContainer(c, manager, "1/lxc/0")
	c.Assert(string(instance.Id()), gc.Equals, "eric-machine-1-lxc-0")
}

func (s *LxcSuite) TestListContainers(c *gc.C) {
	foo := lxc.NewContainerManager(lxc.ManagerConfig{Name: "foo"})
	bar := lxc.NewContainerManager(lxc.ManagerConfig{Name: "bar"})

	foo1 := StartContainer(c, foo, "1/lxc/0")
	foo2 := StartContainer(c, foo, "1/lxc/1")
	foo3 := StartContainer(c, foo, "1/lxc/2")

	bar1 := StartContainer(c, bar, "1/lxc/0")
	bar2 := StartContainer(c, bar, "1/lxc/1")

	result, err := foo.ListContainers()
	c.Assert(err, gc.IsNil)
	testing.MatchInstances(c, result, foo1, foo2, foo3)

	result, err = bar.ListContainers()
	c.Assert(err, gc.IsNil)
	testing.MatchInstances(c, result, bar1, bar2)
}

type NetworkSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (*NetworkSuite) TestGenerateNetworkConfig(c *gc.C) {
	for _, test := range []struct {
		config *lxc.NetworkConfig
		net    string
		link   string
	}{{
		config: nil,
		net:    "veth",
		link:   "lxcbr0",
	}, {
		config: lxc.DefaultNetworkConfig(),
		net:    "veth",
		link:   "lxcbr0",
	}, {
		config: lxc.BridgeNetworkConfig("foo"),
		net:    "veth",
		link:   "foo",
	}, {
		config: lxc.PhysicalNetworkConfig("foo"),
		net:    "phys",
		link:   "foo",
	}} {
		config := lxc.GenerateNetworkConfig(test.config)
		netRegex := fmt.Sprintf(`(\n|.)*lxc.network.type = %s\n(\n|.)*`, test.net)
		c.Assert(config, gc.Matches, netRegex)
		linkRegex := fmt.Sprintf(`(\n|.)*lxc.network.link = %s\n(\n|.)*`, test.link)
		c.Assert(config, gc.Matches, linkRegex)
	}
}

func (*NetworkSuite) TestNetworkConfigTemplate(c *gc.C) {
	config := lxc.NetworkConfigTemplate("foo", "bar")
	expected := `
lxc.network.type = foo
lxc.network.link = bar
lxc.network.flags = up
`
	c.Assert(config, gc.Equals, expected)
}
