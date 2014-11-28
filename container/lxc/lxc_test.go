// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/golxc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/container/lxc/mock"
	lxctesting "github.com/juju/juju/container/lxc/testing"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type LxcSuite struct {
	lxctesting.TestSuite

	events   chan mock.Event
	useClone bool
	useAUFS  bool
}

var _ = gc.Suite(&LxcSuite{})

var lxcCgroupContents = `11:hugetlb:/lxc/juju-machine-1-lxc-0
10:perf_event:/lxc/juju-machine-1-lxc-0
9:blkio:/lxc/juju-machine-1-lxc-0
8:freezer:/lxc/juju-machine-1-lxc-0
7:devices:/lxc/juju-machine-1-lxc-0
6:memory:/lxc/juju-machine-1-lxc-0
5:cpuacct:/lxc/juju-machine-1-lxc-0
4:cpu:/lxc/juju-machine-1-lxc-0
3:cpuset:/lxc/juju-machine-1-lxc-0
2:name=systemd:/lxc/juju-machine-1-lxc-0
`

var hostCgroupContents = `11:hugetlb:/
10:perf_event:/
9:blkio:/
8:freezer:/
7:devices:/
6:memory:/
5:cpuacct:/
4:cpu:/
3:cpuset:/
2:name=systemd:/
`

var malformedCgroupFile = `some bogus content
more bogus content`

func (s *LxcSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	loggo.GetLogger("juju.container.lxc").SetLogLevel(loggo.TRACE)
	s.events = make(chan mock.Event, 25)
	s.TestSuite.ContainerFactory.AddListener(s.events)
	s.PatchValue(&lxc.TemplateLockDir, c.MkDir())
	s.PatchValue(&lxc.TemplateStopTimeout, 500*time.Millisecond)
}

func (s *LxcSuite) TearDownTest(c *gc.C) {
	s.TestSuite.ContainerFactory.RemoveListener(s.events)
	close(s.events)
	s.TestSuite.TearDownTest(c)
}

func (t *LxcSuite) TestPreferFastLXC(c *gc.C) {
	for i, test := range []struct {
		message        string
		releaseVersion string
		expected       bool
	}{{
		message: "missing release file",
	}, {
		message:        "precise release",
		releaseVersion: "12.04",
	}, {
		message:        "trusty release",
		releaseVersion: "14.04",
		expected:       true,
	}, {
		message:        "unstable unicorn",
		releaseVersion: "14.10",
		expected:       true,
	}, {
		message:        "lucid",
		releaseVersion: "10.04",
	}} {
		c.Logf("%v: %v", i, test.message)
		value := lxc.PreferFastLXC(test.releaseVersion)
		c.Assert(value, gc.Equals, test.expected)
	}
}

func (s *LxcSuite) TestContainerManagerLXCClone(c *gc.C) {
	type test struct {
		releaseVersion string
		useClone       string
		expectClone    bool
	}
	tests := []test{{
		releaseVersion: "12.04",
		useClone:       "true",
		expectClone:    true,
	}, {
		releaseVersion: "14.04",
		expectClone:    true,
	}, {
		releaseVersion: "12.04",
		useClone:       "false",
	}, {
		releaseVersion: "14.04",
		useClone:       "false",
	}}

	for i, test := range tests {
		c.Logf("test %d: %v", i, test)
		s.PatchValue(lxc.ReleaseVersion, func() string { return test.releaseVersion })

		mgr, err := lxc.NewContainerManager(container.ManagerConfig{
			container.ConfigName: "juju",
			"use-clone":          test.useClone,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(lxc.GetCreateWithCloneValue(mgr), gc.Equals, test.expectClone)
	}
}

func (s *LxcSuite) TestContainerDirFilesystem(c *gc.C) {
	for i, test := range []struct {
		message    string
		output     string
		expected   string
		errorMatch string
	}{{
		message:  "btrfs",
		output:   "Type\nbtrfs\n",
		expected: lxc.Btrfs,
	}, {
		message:  "ext4",
		output:   "Type\next4\n",
		expected: "ext4",
	}, {
		message:    "not enough output",
		output:     "foo",
		errorMatch: "could not determine filesystem type",
	}} {
		c.Logf("%v: %s", i, test.message)
		s.HookCommandOutput(&lxc.FsCommandOutput, []byte(test.output), nil)
		value, err := lxc.ContainerDirFilesystem()
		if test.errorMatch == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(value, gc.Equals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorMatch)
		}
	}
}

func (s *LxcSuite) makeManager(c *gc.C, name string) container.Manager {
	params := container.ManagerConfig{
		container.ConfigName: name,
	}
	// Need to ensure use-clone is explicitly set to avoid it
	// being set based on the OS version.
	params["use-clone"] = fmt.Sprintf("%v", s.useClone)
	if s.useAUFS {
		params["use-aufs"] = "true"
	}
	manager, err := lxc.NewContainerManager(params)
	c.Assert(err, jc.ErrorIsNil)
	return manager
}

func (*LxcSuite) TestManagerWarnsAboutUnknownOption(c *gc.C) {
	_, err := lxc.NewContainerManager(container.ManagerConfig{
		container.ConfigName: "BillyBatson",
		"shazam":             "Captain Marvel",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(c.GetTestLog(), jc.Contains, `WARNING juju.container unused config option: "shazam" -> "Captain Marvel"`)
}

func (s *LxcSuite) TestCreateContainer(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	// Check our container config files.
	lxcConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, name, "lxc.conf"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.link = nic42")

	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	data := containertesting.AssertCloudInit(c, cloudInitFilename)

	x := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, &x)
	c.Assert(err, jc.ErrorIsNil)

	var scripts []string
	for _, s := range x["runcmd"].([]interface{}) {
		scripts = append(scripts, s.(string))
	}

	c.Assert(scripts[len(scripts)-3:], gc.DeepEquals, []string{
		"start jujud-machine-1-lxc-0",
		"rm $bin/tools.tar.gz && rm $bin/juju2.3.4-quantal-amd64.sha256",
		"ifconfig",
	})

	// Check the mount point has been created inside the container.
	c.Assert(filepath.Join(s.LxcDir, name, "rootfs", agent.DefaultLogDir), jc.IsDirectory)
	// Check that the config file is linked in the restart dir.
	expectedLinkLocation := filepath.Join(s.RestartDir, name+".conf")
	expectedTarget := filepath.Join(s.LxcDir, name, "config")
	linkInfo, err := os.Lstat(expectedLinkLocation)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(linkInfo.Mode()&os.ModeSymlink, gc.Equals, os.ModeSymlink)

	location, err := symlink.Read(expectedLinkLocation)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(location, gc.Equals, expectedTarget)
}

func (s *LxcSuite) TestCreateContainerFailsWithInjectedError(c *gc.C) {
	errorChannel := make(chan error, 1)
	cleanup := mock.PatchTransientErrorInjectionChannel(errorChannel)
	defer cleanup()

	// One injected error means the container creation will fail
	// but the destroy function will clean up the remaining container
	// resulting in a RetryableCreationError
	errorChannel <- errors.New("start error")

	manager := s.makeManager(c, "test")
	_, err := containertesting.CreateContainerTest(c, manager, "1/lxc/0")
	c.Assert(err, gc.NotNil)

	// this should be a retryable error
	isRetryable := instance.IsRetryableCreationError(errors.Cause(err))
	c.Assert(isRetryable, jc.IsTrue)
}

func (s *LxcSuite) TestCreateContainerWithInjectedErrorDestroyFails(c *gc.C) {
	errorChannel := make(chan error, 2)
	cleanup := mock.PatchTransientErrorInjectionChannel(errorChannel)
	defer cleanup()

	// Two injected errors mean that the container creation and subsequent
	// destroy will fail. This should not result in a RetryableCreationError
	// as the container was left in an error state
	errorChannel <- errors.New("create error")
	errorChannel <- errors.New("destroy error")

	manager := s.makeManager(c, "test")
	_, err := containertesting.CreateContainerTest(c, manager, "1/lxc/0")
	c.Assert(err, gc.NotNil)

	// this should not be a retryable error
	isRetryable := instance.IsRetryableCreationError(errors.Cause(err))
	c.Assert(isRetryable, jc.IsFalse)
}

func (s *LxcSuite) ensureTemplateStopped(name string) <-chan struct{} {
	ch := make(chan struct{}, 1)
	go func() {
		for {
			template := s.ContainerFactory.New(name)
			if template.IsRunning() {
				template.Stop()
				close(ch)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ch
}

func (s *LxcSuite) AssertEvent(c *gc.C, event mock.Event, expected mock.Action, id string) {
	c.Assert(event.Action, gc.Equals, expected)
	c.Assert(event.InstanceId, gc.Equals, id)
}

func (s *LxcSuite) TestCreateContainerEvents(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1")
	id := string(instance.Id())
	s.AssertEvent(c, <-s.events, mock.Created, id)
	s.AssertEvent(c, <-s.events, mock.Started, id)
}

func (s *LxcSuite) TestCreateContainerEventsWithClone(c *gc.C) {
	s.PatchValue(&s.useClone, true)
	// The template containers are created with an upstart job that
	// stops them once cloud init has finished.  We emulate that here.
	template := "juju-quantal-lxc-template"
	ch := s.ensureTemplateStopped(template)
	defer func() { <-ch }()
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1")
	id := string(instance.Id())
	s.AssertEvent(c, <-s.events, mock.Created, template)
	s.AssertEvent(c, <-s.events, mock.Started, template)
	s.AssertEvent(c, <-s.events, mock.Stopped, template)
	s.AssertEvent(c, <-s.events, mock.Cloned, template)
	s.AssertEvent(c, <-s.events, mock.Started, id)
}

func (s *LxcSuite) createTemplate(c *gc.C) golxc.Container {
	name := "juju-quantal-lxc-template"
	ch := s.ensureTemplateStopped(name)
	defer func() { <-ch }()
	network := container.BridgeNetworkConfig("nic42")
	authorizedKeys := "authorized keys list"
	aptProxy := proxy.Settings{}
	aptMirror := "http://my.archive.ubuntu.com/ubuntu"
	template, err := lxc.EnsureCloneTemplate(
		"ext4",
		"quantal",
		network,
		authorizedKeys,
		aptProxy,
		aptMirror,
		true,
		true,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(template.Name(), gc.Equals, name)
	s.AssertEvent(c, <-s.events, mock.Created, name)
	s.AssertEvent(c, <-s.events, mock.Started, name)
	s.AssertEvent(c, <-s.events, mock.Stopped, name)

	autostartLink := lxc.RestartSymlink(name)
	config, err := ioutil.ReadFile(lxc.ContainerConfigFilename(name))
	c.Assert(err, jc.ErrorIsNil)
	expected := `
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up
`
	// NOTE: no autostart, no mounting the log dir
	c.Assert(string(config), gc.Equals, expected)
	c.Assert(autostartLink, jc.DoesNotExist)

	return template
}

func (s *LxcSuite) TestCreateContainerEventsWithCloneExistingTemplate(c *gc.C) {
	s.createTemplate(c)
	s.PatchValue(&s.useClone, true)
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1")
	name := string(instance.Id())
	cloned := <-s.events
	s.AssertEvent(c, cloned, mock.Cloned, "juju-quantal-lxc-template")
	c.Assert(cloned.Args, gc.IsNil)
	s.AssertEvent(c, <-s.events, mock.Started, name)
}

func (s *LxcSuite) TestCreateContainerEventsWithCloneExistingTemplateAUFS(c *gc.C) {
	s.createTemplate(c)
	s.PatchValue(&s.useClone, true)
	s.PatchValue(&s.useAUFS, true)
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1")
	name := string(instance.Id())
	cloned := <-s.events
	s.AssertEvent(c, cloned, mock.Cloned, "juju-quantal-lxc-template")
	c.Assert(cloned.Args, gc.DeepEquals, []string{"--snapshot", "--backingstore", "aufs"})
	s.AssertEvent(c, <-s.events, mock.Started, name)
}

func (s *LxcSuite) TestCreateContainerWithCloneMountsAndAutostarts(c *gc.C) {
	s.createTemplate(c)
	s.PatchValue(&s.useClone, true)
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1")
	name := string(instance.Id())

	autostartLink := lxc.RestartSymlink(name)
	config, err := ioutil.ReadFile(lxc.ContainerConfigFilename(name))
	c.Assert(err, jc.ErrorIsNil)
	mountLine := "lxc.mount.entry=/var/log/juju var/log/juju none defaults,bind 0 0"
	c.Assert(string(config), jc.Contains, mountLine)
	c.Assert(autostartLink, jc.IsSymlink)
}

func (s *LxcSuite) TestContainerState(c *gc.C) {
	manager := s.makeManager(c, "test")
	c.Logf("%#v", manager)
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")

	// The mock container will be immediately "running".
	c.Assert(instance.Status(), gc.Equals, string(golxc.StateRunning))

	// DestroyContainer stops and then destroys the container, putting it
	// into "unknown" state.
	err := manager.DestroyContainer(instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instance.Status(), gc.Equals, string(golxc.StateUnknown))
}

func (s *LxcSuite) TestDestroyContainer(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")

	err := manager.DestroyContainer(instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), jc.IsDirectory)
}

func (s *LxcSuite) TestDestroyContainerNameClash(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	targetDir := filepath.Join(s.RemovedDir, name)
	err := os.MkdirAll(targetDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	err = manager.DestroyContainer(instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir with a ".1" suffix as there was already a directory there.
	c.Assert(filepath.Join(s.RemovedDir, fmt.Sprintf("%s.1", name)), jc.IsDirectory)
}

func (s *LxcSuite) TestNamedManagerPrefix(c *gc.C) {
	manager := s.makeManager(c, "eric")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")
	c.Assert(string(instance.Id()), gc.Equals, "eric-machine-1-lxc-0")
}

func (s *LxcSuite) TestListContainers(c *gc.C) {
	foo := s.makeManager(c, "foo")
	bar := s.makeManager(c, "bar")

	foo1 := containertesting.CreateContainer(c, foo, "1/lxc/0")
	foo2 := containertesting.CreateContainer(c, foo, "1/lxc/1")
	foo3 := containertesting.CreateContainer(c, foo, "1/lxc/2")

	bar1 := containertesting.CreateContainer(c, bar, "1/lxc/0")
	bar2 := containertesting.CreateContainer(c, bar, "1/lxc/1")

	result, err := foo.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	instancetest.MatchInstances(c, result, foo1, foo2, foo3)

	result, err = bar.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	instancetest.MatchInstances(c, result, bar1, bar2)
}

func (s *LxcSuite) TestCreateContainerAutostarts(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")
	autostartLink := lxc.RestartSymlink(string(instance.Id()))
	c.Assert(autostartLink, jc.IsSymlink)
}

func (s *LxcSuite) TestCreateContainerNoRestartDir(c *gc.C) {
	err := os.Remove(s.RestartDir)
	c.Assert(err, jc.ErrorIsNil)

	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")
	name := string(instance.Id())
	autostartLink := lxc.RestartSymlink(name)
	config, err := ioutil.ReadFile(lxc.ContainerConfigFilename(name))
	c.Assert(err, jc.ErrorIsNil)
	expected := `
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up
lxc.start.auto = 1
lxc.mount.entry=/var/log/juju var/log/juju none defaults,bind 0 0
`
	c.Assert(string(config), gc.Equals, expected)
	c.Assert(autostartLink, jc.DoesNotExist)
}

func (s *LxcSuite) TestDestroyContainerRemovesAutostartLink(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")
	err := manager.DestroyContainer(instance.Id())
	c.Assert(err, jc.ErrorIsNil)
	autostartLink := lxc.RestartSymlink(string(instance.Id()))
	c.Assert(autostartLink, jc.SymlinkDoesNotExist)
}

func (s *LxcSuite) TestDestroyContainerNoRestartDir(c *gc.C) {
	err := os.Remove(s.RestartDir)
	c.Assert(err, jc.ErrorIsNil)

	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")
	err = manager.DestroyContainer(instance.Id())
	c.Assert(err, jc.ErrorIsNil)
}

type NetworkSuite struct {
	coretesting.BaseSuite
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
	//In the past, the entire lxc.conf file was just networking. With the addition
	//of the auto start, we now have to have better isolate this test. As such, we
	//parse the conf template results and just get the results that start with
	//'lxc.network' as that is what the test cares about.
	obtained := []string{}
	for _, value := range strings.Split(config, "\n") {
		if strings.HasPrefix(value, "lxc.network") {
			obtained = append(obtained, value)
		}
	}
	expected := []string{
		"lxc.network.type = foo",
		"lxc.network.link = bar",
		"lxc.network.flags = up",
	}
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *LxcSuite) TestIsLXCSupportedOnHost(c *gc.C) {
	baseDir := c.MkDir()
	cgroup := filepath.Join(baseDir, "cgroup")

	ft.File{"cgroup", hostCgroupContents, 0400}.Create(c, baseDir)

	s.PatchValue(lxc.InitProcessCgroupFile, cgroup)
	supports, err := lxc.IsLXCSupported()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supports, jc.IsTrue)

}

func (s *LxcSuite) TestIsLXCSupportedOnLXCContainer(c *gc.C) {
	baseDir := c.MkDir()
	cgroup := filepath.Join(baseDir, "cgroup")

	ft.File{"cgroup", lxcCgroupContents, 0400}.Create(c, baseDir)

	s.PatchValue(lxc.InitProcessCgroupFile, cgroup)
	supports, err := lxc.IsLXCSupported()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supports, jc.IsFalse)

}

func (s *LxcSuite) TestIsLXCSupportedMissingCgroupFile(c *gc.C) {
	s.PatchValue(lxc.InitProcessCgroupFile, "")
	supports, err := lxc.IsLXCSupported()
	c.Assert(err.Error(), gc.Matches, "open : no such file or directory")
	c.Assert(supports, jc.IsFalse)
}

func (s *LxcSuite) TestIsLXCSupportedMalformedCgroupFile(c *gc.C) {
	baseDir := c.MkDir()
	cgroup := filepath.Join(baseDir, "cgroup")

	ft.File{"cgroup", malformedCgroupFile, 0400}.Create(c, baseDir)

	s.PatchValue(lxc.InitProcessCgroupFile, cgroup)
	supports, err := lxc.IsLXCSupported()
	c.Assert(err.Error(), gc.Equals, "Malformed cgroup file")
	c.Assert(supports, jc.IsFalse)
}

func (s *LxcSuite) TestIsLXCSupportedNonLinuxSystem(c *gc.C) {
	if runtime.GOOS == "linux" {
		s.PatchValue(lxc.RuntimeGOOS, "windows")
	}
	supports, err := lxc.IsLXCSupported()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supports, jc.IsFalse)
}
