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
	"launchpad.net/golxc"
	"launchpad.net/goyaml"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/lxc"
	lxctesting "launchpad.net/juju-core/container/lxc/testing"
	containertesting "launchpad.net/juju-core/container/testing"
	instancetest "launchpad.net/juju-core/instance/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type LxcSuite struct {
	lxctesting.TestSuite
}

var _ = gc.Suite(&LxcSuite{})

func (s *LxcSuite) SetUpSuite(c *gc.C) {
	s.TestSuite.SetUpSuite(c)
	tmpDir := c.MkDir()
	restore := testbase.PatchEnvironment("PATH", tmpDir)
	s.AddSuiteCleanup(func(*gc.C) { restore() })
	err := ioutil.WriteFile(
		filepath.Join(tmpDir, "apt-config"),
		[]byte(aptConfigScript),
		0755)
	c.Assert(err, gc.IsNil)
}

func (s *LxcSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	loggo.GetLogger("juju.container.lxc").SetLogLevel(loggo.TRACE)
}

const (
	aptHTTPProxy     = "http://1.2.3.4:3142"
	configProxyExtra = `Acquire::https::Proxy "false";
Acquire::ftp::Proxy "false";`
)

var (
	configHttpProxy = fmt.Sprintf(`Acquire::http::Proxy "%s";`, aptHTTPProxy)
	aptConfigScript = fmt.Sprintf("#!/bin/sh\n echo '%s\n%s'", configHttpProxy, configProxyExtra)
)

func (s *LxcSuite) TestStartContainer(c *gc.C) {
	manager := lxc.NewContainerManager(container.ManagerConfig{})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	// Check our container config files.
	lxcConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, name, "lxc.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.link = nic42")

	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	data := containertesting.AssertCloudInit(c, cloudInitFilename)

	x := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &x)
	c.Assert(err, gc.IsNil)

	c.Assert(x["apt_proxy"], gc.Equals, aptHTTPProxy)

	var scripts []string
	for _, s := range x["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}

	c.Assert(scripts[len(scripts)-4:], gc.DeepEquals, []string{
		"start jujud-machine-1-lxc-0",
		"install -D -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy-extra'",
		fmt.Sprintf(`printf '%%s\n' '%s' > '/etc/apt/apt.conf.d/99proxy-extra'`, configProxyExtra),
		"ifconfig",
	})

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

func (s *LxcSuite) TestContainerState(c *gc.C) {
	manager := lxc.NewContainerManager(container.ManagerConfig{})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")

	// The mock container will be immediately "running".
	c.Assert(instance.Status(), gc.Equals, string(golxc.StateRunning))

	// StopContainer stops and then destroys the container, putting it
	// into "unknown" state.
	err := manager.StopContainer(instance)
	c.Assert(err, gc.IsNil)
	c.Assert(instance.Status(), gc.Equals, string(golxc.StateUnknown))
}

func (s *LxcSuite) TestStopContainer(c *gc.C) {
	manager := lxc.NewContainerManager(container.ManagerConfig{})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")

	err := manager.StopContainer(instance)
	c.Assert(err, gc.IsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), jc.IsDirectory)
}

func (s *LxcSuite) TestStopContainerNameClash(c *gc.C) {
	manager := lxc.NewContainerManager(container.ManagerConfig{})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")

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
	manager := lxc.NewContainerManager(container.ManagerConfig{Name: "eric"})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")
	c.Assert(string(instance.Id()), gc.Equals, "eric-machine-1-lxc-0")
}

func (s *LxcSuite) TestListContainers(c *gc.C) {
	foo := lxc.NewContainerManager(container.ManagerConfig{Name: "foo"})
	bar := lxc.NewContainerManager(container.ManagerConfig{Name: "bar"})

	foo1 := containertesting.StartContainer(c, foo, "1/lxc/0")
	foo2 := containertesting.StartContainer(c, foo, "1/lxc/1")
	foo3 := containertesting.StartContainer(c, foo, "1/lxc/2")

	bar1 := containertesting.StartContainer(c, bar, "1/lxc/0")
	bar2 := containertesting.StartContainer(c, bar, "1/lxc/1")

	result, err := foo.ListContainers()
	c.Assert(err, gc.IsNil)
	instancetest.MatchInstances(c, result, foo1, foo2, foo3)

	result, err = bar.ListContainers()
	c.Assert(err, gc.IsNil)
	instancetest.MatchInstances(c, result, bar1, bar2)
}

func (s *LxcSuite) TestStartContainerAutostarts(c *gc.C) {
	manager := lxc.NewContainerManager(container.ManagerConfig{})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")
	autostartLink := lxc.RestartSymlink(string(instance.Id()))
	c.Assert(autostartLink, jc.IsSymlink)
}

func (s *LxcSuite) TestStopContainerRemovesAutostartLink(c *gc.C) {
	manager := lxc.NewContainerManager(container.ManagerConfig{})
	instance := containertesting.StartContainer(c, manager, "1/lxc/0")
	err := manager.StopContainer(instance)
	c.Assert(err, gc.IsNil)
	autostartLink := lxc.RestartSymlink(string(instance.Id()))
	c.Assert(autostartLink, jc.SymlinkDoesNotExist)
}

type NetworkSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (*NetworkSuite) TestGenerateNetworkConfig(c *gc.C) {
	for _, test := range []struct {
		config *container.NetworkConfig
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
		config: container.BridgeNetworkConfig("foo"),
		net:    "veth",
		link:   "foo",
	}, {
		config: container.PhysicalNetworkConfig("foo"),
		net:    "phys",
		link:   "foo",
	}} {
		config := lxc.GenerateNetworkConfig(test.config)
		c.Assert(config, jc.Contains, fmt.Sprintf("lxc.network.type = %s\n", test.net))
		c.Assert(config, jc.Contains, fmt.Sprintf("lxc.network.link = %s\n", test.link))
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
