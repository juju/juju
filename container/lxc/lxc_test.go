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
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	if runtime.GOOS == "windows" {
		t.Skip("LXC is currently not supported on windows")
	}
	gc.TestingT(t)
}

type LxcSuite struct {
	lxctesting.TestSuite

	events            chan mock.Event
	useClone          bool
	useAUFS           bool
	logDir            string
	loopDeviceManager mockLoopDeviceManager
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
	s.SetFeatureFlags(feature.AddressAllocation)
	s.logDir = c.MkDir()
	loggo.GetLogger("juju.container.lxc").SetLogLevel(loggo.TRACE)
	s.events = make(chan mock.Event, 25)
	s.TestSuite.ContainerFactory.AddListener(s.events)
	s.PatchValue(&lxc.TemplateLockDir, c.MkDir())
	s.PatchValue(&lxc.TemplateStopTimeout, 500*time.Millisecond)
	s.loopDeviceManager = mockLoopDeviceManager{}
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
		}, &containertesting.MockURLGetter{}, nil)
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

func (*LxcSuite) TestParseConfigLine(c *gc.C) {
	for i, test := range []struct {
		about   string
		input   string
		setting string
		value   string
	}{{
		about:   "empty line",
		input:   "",
		setting: "",
		value:   "",
	}, {
		about:   "line with spaces",
		input:   "  line  with spaces   ",
		setting: "",
		value:   "",
	}, {
		about:   "comments",
		input:   "# comment",
		setting: "",
		value:   "",
	}, {
		about:   "commented setting",
		input:   "#lxc.flag = disabled",
		setting: "",
		value:   "",
	}, {
		about:   "comments with spaces",
		input:   "  # comment  with   spaces ",
		setting: "",
		value:   "",
	}, {
		about:   "not a setting",
		input:   "anything here",
		setting: "",
		value:   "",
	}, {
		about:   "valid setting, no whitespace",
		input:   "lxc.setting=value",
		setting: "lxc.setting",
		value:   "value",
	}, {
		about:   "valid setting, with whitespace",
		input:   "  lxc.setting  =  value  ",
		setting: "lxc.setting",
		value:   "value",
	}, {
		about:   "valid setting, with comment on the value",
		input:   "lxc.setting = value  # comment # foo  ",
		setting: "lxc.setting",
		value:   "value",
	}, {
		about:   "valid setting, with comment, spaces and extra equals",
		input:   "lxc.my.best.setting = foo=bar, but   # not really ",
		setting: "lxc.my.best.setting",
		value:   "foo=bar, but",
	}} {
		c.Logf("test %d: %s", i, test.about)
		setting, value := lxc.ParseConfigLine(test.input)
		c.Check(setting, gc.Equals, test.setting)
		c.Check(value, gc.Equals, test.value)
		if setting == "" {
			c.Check(value, gc.Equals, "")
		}
	}
}

func (s *LxcSuite) TestUpdateContainerConfig(c *gc.C) {
	networkConfig := container.BridgeNetworkConfig("nic42", 4321, []network.InterfaceInfo{{
		DeviceIndex:    0,
		CIDR:           "0.1.2.0/20",
		InterfaceName:  "eth0",
		MACAddress:     "aa:bb:cc:dd:ee:f0",
		Address:        network.NewAddress("0.1.2.3"),
		GatewayAddress: network.NewAddress("0.1.2.1"),
	}, {
		DeviceIndex:   1,
		InterfaceName: "eth1",
	}})
	storageConfig := &container.StorageConfig{
		AllowMount: true,
	}

	manager := s.makeManager(c, "test")
	instanceConfig, err := containertesting.MockMachineConfig("1/lxc/0")
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.Config = envConfig
	instance := containertesting.CreateContainerWithMachineAndNetworkAndStorageConfig(
		c, manager, instanceConfig, networkConfig, storageConfig,
	)
	name := string(instance.Id())

	// Append a few extra lines to the config.
	extraLines := []string{
		"  lxc.rootfs =  /some/thing  # else ",
		"",
		"  # just comment",
		"lxc.network.vlan.id=42",
		"something else  # ignore",
		"lxc.network.type=veth",
		"lxc.network.link = foo  # comment",
		"lxc.network.hwaddr = bar",
	}
	configPath := lxc.ContainerConfigFilename(name)
	configFile, err := os.OpenFile(configPath, os.O_RDWR|os.O_APPEND, 0644)
	c.Assert(err, jc.ErrorIsNil)
	_, err = configFile.WriteString(strings.Join(extraLines, "\n") + "\n")
	c.Assert(err, jc.ErrorIsNil)
	err = configFile.Close()
	c.Assert(err, jc.ErrorIsNil)

	expectedConf := fmt.Sprintf(`
# network config
# interface "eth0"
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up
lxc.network.name = eth0
lxc.network.hwaddr = aa:bb:cc:dd:ee:f0
lxc.network.ipv4 = 0.1.2.3/32
lxc.network.ipv4.gateway = 0.1.2.1
lxc.network.mtu = 4321

# interface "eth1"
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up
lxc.network.name = eth1
lxc.network.mtu = 4321


lxc.mount.entry = %s var/log/juju none defaults,bind 0 0

lxc.aa_profile = lxc-container-default-with-mounting
lxc.cgroup.devices.allow = b 7:* rwm
lxc.cgroup.devices.allow = c 10:237 rwm
`, s.logDir) + strings.Join(extraLines, "\n") + "\n"

	lxcConfContents, err := ioutil.ReadFile(configPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(lxcConfContents), gc.Equals, expectedConf)

	linesToReplace := []string{
		"", // empty lines are ignored
		"  lxc.network.type = bar # free drinks  !! ", // formatting is sanitized.
		"  # comments are ignored",
		"lxc.network.type=foo",                   // replace the second "type".
		"lxc.network.name = em0 # renamed now",   // replace the first "name"
		"lxc.network.name = em1",                 // replace the second "name"
		"lxc.network.mtu = 1234",                 // replace only the first "mtu".
		"lxc.network.hwaddr = ff:ee:dd:cc:bb:aa", // replace the first "hwaddr".
		"lxc.network.hwaddr=deadbeef",            // replace second "hwaddr".
		"lxc.network.hwaddr=nonsense",            // no third "hwaddr", so append.
		"lxc.network.hwaddr = ",                  // no fourth "hwaddr" to remove - ignored.
		"lxc.network.link=",                      // remove only the first "link"
		"lxc.network.vlan.id=69",                 // replace.
		"lxc.missing = appended",                 // missing - appended.
		"lxc.network.type = phys",                // replace the third "type".
		"lxc.mount.entry=",                       // delete existing "entry".
		"lxc.rootfs = /foo/bar",                  // replace first "rootfs".
		"lxc.rootfs = /bar/foo",                  // append new.
	}
	newConfig := strings.Join(linesToReplace, "\n")
	updatedConfig := `
# network config
# interface "eth0"
lxc.network.type = bar
lxc.network.flags = up
lxc.network.name = em0
lxc.network.hwaddr = ff:ee:dd:cc:bb:aa
lxc.network.ipv4 = 0.1.2.3/32
lxc.network.ipv4.gateway = 0.1.2.1
lxc.network.mtu = 1234

# interface "eth1"
lxc.network.type = foo
lxc.network.link = nic42
lxc.network.flags = up
lxc.network.name = em1
lxc.network.mtu = 4321



lxc.aa_profile = lxc-container-default-with-mounting
lxc.cgroup.devices.allow = b 7:* rwm
lxc.cgroup.devices.allow = c 10:237 rwm
lxc.rootfs = /foo/bar

  # just comment
lxc.network.vlan.id = 69
something else  # ignore
lxc.network.type = phys
lxc.network.link = foo  # comment
lxc.network.hwaddr = deadbeef
lxc.network.hwaddr = nonsense
lxc.missing = appended
lxc.rootfs = /bar/foo
`
	err = lxc.UpdateContainerConfig(name, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	lxcConfContents, err = ioutil.ReadFile(configPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(lxcConfContents), gc.Equals, updatedConfig)

	// Now test the example in updateContainerConfig's doc string.
	oldConfig := `
lxc.foo = off

lxc.bar=42
`
	newConfig = `
lxc.bar=
lxc.foo = bar
lxc.foo = baz # xx
`
	updatedConfig = `
lxc.foo = bar

lxc.foo = baz
`
	err = ioutil.WriteFile(configPath, []byte(oldConfig), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = lxc.UpdateContainerConfig(name, newConfig)
	c.Assert(err, jc.ErrorIsNil)
	lxcConfContents, err = ioutil.ReadFile(configPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(lxcConfContents), gc.Equals, updatedConfig)
}

func (*LxcSuite) TestReorderNetworkConfig(c *gc.C) {
	path := c.MkDir()
	configFile := filepath.Join(path, "config")
	for i, test := range []struct {
		about         string
		input         string
		shouldReorder bool
		expectErr     string
		output        string
	}{{
		about:         "empty input",
		input:         "",
		shouldReorder: false,
		expectErr:     "",
		output:        "",
	}, {
		about: "no network settings",
		input: `
# comment
lxc.foo = bar

  lxc.test=# none
lxc.bar.foo=42 # comment
`,
		shouldReorder: false,
	}, {
		about: "just one lxc.network.type",
		input: `
# comment
lxc.foo = bar

lxc.network.type = veth

  lxc.test=# none
lxc.bar.foo=42 # comment
`,
		shouldReorder: false,
	}, {
		about: "correctly ordered network config",
		input: `
# Network configuration
lxc.network.type = veth
lxc.network.hwaddr = aa:bb:cc:dd:ee:f0
lxc.network.flags = up
lxc.network.link = br0
lxc.network.type = veth
lxc.network.flags = up
lxc.network.link = br2
lxc.network.hwaddr = aa:bb:cc:dd:ee:f1
lxc.network.name = eth1
lxc.network.type = veth
lxc.network.flags = up
lxc.network.link = br3
lxc.network.hwaddr = aa:bb:cc:dd:ee:f2
lxc.network.name = eth2
lxc.hook.mount = /usr/share/lxc/config/hook.sh
`,
		shouldReorder: false,
	}, {
		about: "1 hwaddr before first type",
		input: `
lxc.foo = bar # stays here
# Network configuration
  lxc.network.hwaddr = aa:bb:cc:dd:ee:f0 # comment
lxc.network.type = veth    # comment2  
lxc.network.flags = up # all the rest..
lxc.network.link = br0 # ..is kept...
lxc.network.type = veth # ..as it is.
lxc.network.flags = up
lxc.network.link = br2
lxc.hook.mount = /usr/share/lxc/config/hook.sh
`,
		shouldReorder: true,
		output: `
lxc.foo = bar # stays here
# Network configuration
lxc.network.type = veth    # comment2  
  lxc.network.hwaddr = aa:bb:cc:dd:ee:f0 # comment
lxc.network.flags = up # all the rest..
lxc.network.link = br0 # ..is kept...
lxc.network.type = veth # ..as it is.
lxc.network.flags = up
lxc.network.link = br2
lxc.hook.mount = /usr/share/lxc/config/hook.sh
`,
	}, {
		about: "several network lines before first type",
		input: `
lxc.foo = bar # stays here
# Network configuration
  lxc.network.hwaddr = aa:bb:cc:dd:ee:f0 # comment
lxc.network.flags = up # first up
lxc.network.link = br0
lxc.network.type = veth    # comment2  
lxc.network.type = vlan
lxc.network.flags = up # all the rest..
lxc.network.link = br1 # ...is kept...
lxc.network.vlan.id = 42 # ...as it is.
lxc.hook.mount = /usr/share/lxc/config/hook.sh
`,
		shouldReorder: true,
		output: `
lxc.foo = bar # stays here
# Network configuration
lxc.network.type = veth    # comment2  
  lxc.network.hwaddr = aa:bb:cc:dd:ee:f0 # comment
lxc.network.flags = up # first up
lxc.network.link = br0
lxc.network.type = vlan
lxc.network.flags = up # all the rest..
lxc.network.link = br1 # ...is kept...
lxc.network.vlan.id = 42 # ...as it is.
lxc.hook.mount = /usr/share/lxc/config/hook.sh
`,
	}, {
		about: "one network setting without lxc.network.type",
		input: `
# comment
lxc.foo = bar

lxc.network.anything=goes#badly

  lxc.test=# none
lxc.bar.foo=42 # comment
`,
		expectErr: `cannot have line\(s\) ".*" without lxc.network.type in config ".*"`,
	}, {
		about: "several network settings without lxc.network.type",
		input: `
# comment
lxc.foo = bar

lxc.network.anything=goes#badly
lxc.network.vlan.id = 42
lxc.network.name = foo

  lxc.test=# none
lxc.bar.foo=42 # comment
`,
		expectErr: `cannot have line\(s\) ".*" without lxc.network.type in config ".*"`,
	}} {
		c.Logf("test %d: %q", i, test.about)
		err := ioutil.WriteFile(configFile, []byte(test.input), 0644)
		c.Assert(err, jc.ErrorIsNil)
		wasReordered, err := lxc.ReorderNetworkConfig(configFile)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			c.Check(wasReordered, gc.Equals, test.shouldReorder)
			continue
		}
		data, err := ioutil.ReadFile(configFile)
		c.Assert(err, jc.ErrorIsNil)
		if test.shouldReorder {
			c.Check(string(data), gc.Equals, test.output)
			c.Check(wasReordered, jc.IsTrue)
		} else {
			c.Check(string(data), gc.Equals, test.input)
			c.Check(wasReordered, jc.IsFalse)
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
	params["log-dir"] = s.logDir
	if s.useAUFS {
		params["use-aufs"] = "true"
	}
	manager, err := lxc.NewContainerManager(
		params, &containertesting.MockURLGetter{},
		&s.loopDeviceManager,
	)
	c.Assert(err, jc.ErrorIsNil)
	return manager
}

func (*LxcSuite) TestManagerWarnsAboutUnknownOption(c *gc.C) {
	_, err := lxc.NewContainerManager(container.ManagerConfig{
		container.ConfigName: "BillyBatson",
		"shazam":             "Captain Marvel",
	}, &containertesting.MockURLGetter{}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(c.GetTestLog(), jc.Contains, `WARNING juju.container unused config option: "shazam" -> "Captain Marvel"`)
}

func (s *LxcSuite) TestCreateContainer(c *gc.C) {
	manager := s.makeManager(c, "test")
	instance := containertesting.CreateContainer(c, manager, "1/lxc/0")

	name := string(instance.Id())
	// Check our container config files: initial lxc.conf, the
	// run-time effective config, and cloud-init userdata.
	lxcConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, name, "lxc.conf"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.link = nic42")
	lxcConfContents, err = ioutil.ReadFile(lxc.ContainerConfigFilename(name))
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
	if expected == mock.Created {
		c.Assert(event.EnvArgs, gc.Not(gc.HasLen), 0)
	}
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
	network := container.BridgeNetworkConfig("nic42", 4321, nil)
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
		&containertesting.MockURLGetter{},
		false,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(template.Name(), gc.Equals, name)

	createEvent := <-s.events
	c.Assert(createEvent.Action, gc.Equals, mock.Created)
	c.Assert(createEvent.InstanceId, gc.Equals, name)
	argsSet := set.NewStrings(createEvent.TemplateArgs...)
	c.Assert(argsSet.Contains("imageURL"), jc.IsTrue)
	s.AssertEvent(c, <-s.events, mock.Started, name)
	s.AssertEvent(c, <-s.events, mock.Stopped, name)

	autostartLink := lxc.RestartSymlink(name)
	config, err := ioutil.ReadFile(lxc.ContainerConfigFilename(name))
	c.Assert(err, jc.ErrorIsNil)
	expected := `
# network config
# interface "eth0"
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up
lxc.network.mtu = 4321

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
	mountLine := fmt.Sprintf("lxc.mount.entry = %s var/log/juju none defaults,bind 0 0", s.logDir)
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

	c.Assert(s.loopDeviceManager.detachLoopDevicesArgs, jc.DeepEquals, [][]string{
		{filepath.Join(s.LxcDir, name, "rootfs"), "/"},
	})
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
	expected := fmt.Sprintf(`
# network config
# interface "eth0"
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up

lxc.start.auto = 1
lxc.mount.entry = %s var/log/juju none defaults,bind 0 0
`, s.logDir)
	c.Assert(string(config), gc.Equals, expected)
	c.Assert(autostartLink, jc.DoesNotExist)
}

func (s *LxcSuite) TestCreateContainerWithBlockStorage(c *gc.C) {
	err := os.Remove(s.RestartDir)
	c.Assert(err, jc.ErrorIsNil)

	manager := s.makeManager(c, "test")
	machineConfig, err := containertesting.MockMachineConfig("1/lxc/0")
	c.Assert(err, jc.ErrorIsNil)
	storageConfig := &container.StorageConfig{AllowMount: true}
	networkConfig := container.BridgeNetworkConfig("nic42", 4321, nil)
	instance := containertesting.CreateContainerWithMachineAndNetworkAndStorageConfig(c, manager, machineConfig, networkConfig, storageConfig)
	name := string(instance.Id())
	autostartLink := lxc.RestartSymlink(name)
	config, err := ioutil.ReadFile(lxc.ContainerConfigFilename(name))
	c.Assert(err, jc.ErrorIsNil)
	expected := fmt.Sprintf(`
# network config
# interface "eth0"
lxc.network.type = veth
lxc.network.link = nic42
lxc.network.flags = up
lxc.network.mtu = 4321

lxc.start.auto = 1
lxc.mount.entry = %s var/log/juju none defaults,bind 0 0

lxc.aa_profile = lxc-container-default-with-mounting
lxc.cgroup.devices.allow = b 7:* rwm
lxc.cgroup.devices.allow = c 10:237 rwm
`, s.logDir)
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
	dhcpNIC := network.InterfaceInfo{
		DeviceIndex:   0,
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0",
		// The following is not part of the LXC config, but cause the
		// generated cloud-init user-data to change accordingly.
		ConfigType: network.ConfigDHCP,
	}
	staticNIC := network.InterfaceInfo{
		DeviceIndex:    1,
		CIDR:           "0.1.2.0/20", // used to infer the subnet mask.
		MACAddress:     "aa:bb:cc:dd:ee:f1",
		InterfaceName:  "eth1",
		Address:        network.NewAddress("0.1.2.3"),
		GatewayAddress: network.NewAddress("0.1.2.1"),
		// The rest is passed to cloud-init.
		ConfigType: network.ConfigStatic,
		DNSServers: network.NewAddresses("ns1.invalid", "ns2.invalid"),
	}
	extraConfigNIC := network.InterfaceInfo{
		DeviceIndex:   2,
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		InterfaceName: "eth2",
		VLANTag:       42,
		NoAutoStart:   true,
		// The rest is passed to cloud-init.
		ConfigType: network.ConfigManual,
		DNSServers: network.NewAddresses("ns1.invalid", "ns2.invalid"),
		ExtraConfig: map[string]string{
			"pre-up":   "ip route add default via 0.1.2.1",
			"up":       "ip route add 0.1.2.1 dev eth2",
			"pre-down": "ip route del 0.1.2.1 dev eth2",
			"down":     "ip route del default via 0.1.2.1",
		},
	}
	// Test /24 is used by default when the CIDR is invalid or empty.
	staticNICNoCIDR, staticNICBadCIDR := staticNIC, staticNIC
	staticNICNoCIDR.CIDR = ""
	staticNICBadCIDR.CIDR = "bad"
	// Test when NoAutoStart is true gateway is not added, even if there.
	staticNICNoAutoWithGW := staticNIC
	staticNICNoAutoWithGW.NoAutoStart = true

	var lastTestLog string
	allNICs := []network.InterfaceInfo{dhcpNIC, staticNIC, extraConfigNIC}
	for i, test := range []struct {
		about             string
		config            *container.NetworkConfig
		nics              []network.InterfaceInfo
		rendered          []string
		logContains       string
		logDoesNotContain string
	}{{
		about:  "empty config",
		config: nil,
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = lxcbr0",
			"lxc.network.flags = up",
		},
		logContains:       `WARNING juju.container.lxc network type missing, using the default "bridge" config`,
		logDoesNotContain: `INFO juju.container.lxc setting MTU to 0 for LXC network interfaces`,
	}, {
		about:  "default config",
		config: lxc.DefaultNetworkConfig(),
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = lxcbr0",
			"lxc.network.flags = up",
		},
		logDoesNotContain: `INFO juju.container.lxc setting MTU to 0 for LXC network interfaces`,
	}, {
		about:  "bridge config with MTU 1500, device foo, no NICs",
		config: container.BridgeNetworkConfig("foo", 1500, nil),
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = foo",
			"lxc.network.flags = up",
			"lxc.network.mtu = 1500",
		},
		logContains: `INFO juju.container.lxc setting MTU to 1500 for all LXC network interfaces`,
	}, {
		about:  "phys config with MTU 9000, device foo, no NICs",
		config: container.PhysicalNetworkConfig("foo", 9000, nil),
		rendered: []string{
			"lxc.network.type = phys",
			"lxc.network.link = foo",
			"lxc.network.flags = up",
			"lxc.network.mtu = 9000",
		},
		logContains: `INFO juju.container.lxc setting MTU to 9000 for all LXC network interfaces`,
	}, {
		about:  "bridge config with MTU 8000, device foo, all NICs",
		config: container.BridgeNetworkConfig("foo", 8000, allNICs),
		nics:   allNICs,
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = foo",
			"lxc.network.flags = up",
			"lxc.network.name = eth0",
			"lxc.network.hwaddr = aa:bb:cc:dd:ee:f0",
			"lxc.network.mtu = 8000",

			"lxc.network.type = veth",
			"lxc.network.link = foo",
			"lxc.network.flags = up",
			"lxc.network.name = eth1",
			"lxc.network.hwaddr = aa:bb:cc:dd:ee:f1",
			"lxc.network.ipv4 = 0.1.2.3/32",
			"lxc.network.ipv4.gateway = 0.1.2.1",
			"lxc.network.mtu = 8000",

			"lxc.network.type = vlan",
			"lxc.network.vlan.id = 42",
			"lxc.network.link = foo",
			"lxc.network.name = eth2",
			"lxc.network.hwaddr = aa:bb:cc:dd:ee:f2",
			"lxc.network.mtu = 8000",
		},
		logContains: `INFO juju.container.lxc setting MTU to 8000 for all LXC network interfaces`,
	}, {
		about:  "bridge config with MTU 0, device foo, staticNICNoCIDR",
		config: container.BridgeNetworkConfig("foo", 0, []network.InterfaceInfo{staticNICNoCIDR}),
		nics:   []network.InterfaceInfo{staticNICNoCIDR},
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = foo",
			"lxc.network.flags = up",
			"lxc.network.name = eth1",
			"lxc.network.hwaddr = aa:bb:cc:dd:ee:f1",
			"lxc.network.ipv4 = 0.1.2.3/32",
			"lxc.network.ipv4.gateway = 0.1.2.1",
		},
		logDoesNotContain: `INFO juju.container.lxc setting MTU to 0 for all LXC network interfaces`,
	}, {
		about:  "bridge config with MTU 0, device foo, staticNICBadCIDR",
		config: container.BridgeNetworkConfig("foo", 0, []network.InterfaceInfo{staticNICBadCIDR}),
		nics:   []network.InterfaceInfo{staticNICBadCIDR},
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = foo",
			"lxc.network.flags = up",
			"lxc.network.name = eth1",
			"lxc.network.hwaddr = aa:bb:cc:dd:ee:f1",
			"lxc.network.ipv4 = 0.1.2.3/32",
			"lxc.network.ipv4.gateway = 0.1.2.1",
		},
	}, {
		about:  "bridge config with MTU 0, device foo, staticNICNoAutoWithGW",
		config: container.BridgeNetworkConfig("foo", 0, []network.InterfaceInfo{staticNICNoAutoWithGW}),
		nics:   []network.InterfaceInfo{staticNICNoAutoWithGW},
		rendered: []string{
			"lxc.network.type = veth",
			"lxc.network.link = foo",
			"lxc.network.name = eth1",
			"lxc.network.hwaddr = aa:bb:cc:dd:ee:f1",
			"lxc.network.ipv4 = 0.1.2.3/32",
		},
		logContains: `WARNING juju.container.lxc not setting IPv4 gateway "0.1.2.1" for non-auto start interface "eth1"`,
	}} {
		c.Logf("test #%d: %s", i, test.about)
		config := lxc.GenerateNetworkConfig(test.config)
		// Parse the config to drop comments and empty lines. This is
		// needed to ensure the order of all settings match what we
		// expect to get rendered, as the order matters.
		var configLines []string
		for _, line := range strings.Split(config, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			configLines = append(configLines, line)
		}
		currentLog := strings.TrimPrefix(c.GetTestLog(), lastTestLog)
		c.Check(configLines, jc.DeepEquals, test.rendered)
		if test.logContains != "" {
			c.Check(currentLog, jc.Contains, test.logContains)
		}
		if test.logDoesNotContain != "" {
			c.Check(currentLog, gc.Not(jc.Contains), test.logDoesNotContain)
		}
		// TODO(dimitern) In a follow-up, test the generated user-data
		// honors the other settings.
		lastTestLog = c.GetTestLog()
	}
}

func (*NetworkSuite) TestNetworkConfigTemplate(c *gc.C) {
	// Intentionally using an invalid type "foo" here to test it gets
	// changed to the default "veth" and a warning is logged.
	config := lxc.NetworkConfigTemplate(container.NetworkConfig{"foo", "bar", 4321, nil})
	// In the past, the entire lxc.conf file was just networking. With
	// the addition of the auto start, we now have to have better
	// isolate this test. As such, we parse the conf template results
	// and just get the results that start with 'lxc.network' as that
	// is what the test cares about.
	obtained := []string{}
	for _, value := range strings.Split(config, "\n") {
		if strings.HasPrefix(value, "lxc.network") {
			obtained = append(obtained, value)
		}
	}
	expected := []string{
		"lxc.network.type = veth",
		"lxc.network.link = bar",
		"lxc.network.flags = up",
		"lxc.network.mtu = 4321",
	}
	c.Assert(obtained, jc.DeepEquals, expected)
	log := c.GetTestLog()
	c.Assert(log, jc.Contains,
		`WARNING juju.container.lxc unknown network type "foo", using the default "bridge" config`,
	)
	c.Assert(log, jc.Contains,
		`INFO juju.container.lxc setting MTU to 4321 for all LXC network interfaces`,
	)
}

func (s *LxcSuite) TestIsLXCSupportedOnHost(c *gc.C) {
	s.PatchValue(lxc.RunningInsideLXC, func() (bool, error) {
		return false, nil
	})
	supports, err := lxc.IsLXCSupported()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supports, jc.IsTrue)
}

func (s *LxcSuite) TestIsLXCSupportedOnLXCContainer(c *gc.C) {
	s.PatchValue(lxc.RunningInsideLXC, func() (bool, error) {
		return true, nil
	})
	supports, err := lxc.IsLXCSupported()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supports, jc.IsFalse)
}

func (s *LxcSuite) TestIsLXCSupportedNonLinuxSystem(c *gc.C) {
	s.PatchValue(lxc.RuntimeGOOS, "windows")
	s.PatchValue(lxc.RunningInsideLXC, func() (bool, error) {
		panic("should not be called")
	})
	supports, err := lxc.IsLXCSupported()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supports, jc.IsFalse)
}

func (s *LxcSuite) TestWgetEnvironmentUsesNoProxy(c *gc.C) {
	var wgetScript []byte
	fakeCert := []byte("fakeCert")
	s.PatchValue(lxc.WriteWgetTmpFile, func(filename string, data []byte, perm os.FileMode) error {
		wgetScript = data
		return nil
	})
	_, closer, err := lxc.WgetEnvironment(fakeCert)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	c.Assert(string(wgetScript), jc.Contains, "/usr/bin/wget --no-proxy --ca-certificate")
}

type mockLoopDeviceManager struct {
	detachLoopDevicesArgs [][]string
}

func (m *mockLoopDeviceManager) DetachLoopDevices(rootfs, prefix string) error {
	m.detachLoopDevicesArgs = append(m.detachLoopDevicesArgs, []string{rootfs, prefix})
	return nil
}
