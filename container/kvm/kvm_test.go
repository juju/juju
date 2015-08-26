// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	kvmtesting "github.com/juju/juju/container/kvm/testing"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

type KVMSuite struct {
	kvmtesting.TestSuite
	manager container.Manager
}

var _ = gc.Suite(&KVMSuite{})

func (s *KVMSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	var err error
	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{container.ConfigName: "test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (*KVMSuite) TestManagerNameNeeded(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{container.ConfigName: ""})
	c.Assert(err, gc.ErrorMatches, "name is required")
	c.Assert(manager, gc.IsNil)
}

func (*KVMSuite) TestManagerWarnsAboutUnknownOption(c *gc.C) {
	_, err := kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigName: "BillyBatson",
		"shazam":             "Captain Marvel",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(c.GetTestLog(), jc.Contains, `WARNING juju.container unused config option: "shazam" -> "Captain Marvel"`)
}

func (s *KVMSuite) TestListInitiallyEmpty(c *gc.C) {
	containers, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *KVMSuite) createRunningContainer(c *gc.C, name string) kvm.Container {
	kvmContainer := s.ContainerFactory.New(name)
	network := container.BridgeNetworkConfig("testbr0", 0, nil)
	c.Assert(kvmContainer.Start(kvm.StartParams{
		Series:       "quantal",
		Arch:         arch.HostArch(),
		UserDataFile: "userdata.txt",
		Network:      network}), gc.IsNil)
	return kvmContainer
}

func (s *KVMSuite) TestListMatchesManagerName(c *gc.C) {
	s.createRunningContainer(c, "test-match1")
	s.createRunningContainer(c, "test-match2")
	s.createRunningContainer(c, "testNoMatch")
	s.createRunningContainer(c, "other")
	containers, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 2)
	expectedIds := []instance.Id{"test-match1", "test-match2"}
	ids := []instance.Id{containers[0].Id(), containers[1].Id()}
	c.Assert(ids, jc.SameContents, expectedIds)
}

func (s *KVMSuite) TestListMatchesRunningContainers(c *gc.C) {
	running := s.createRunningContainer(c, "test-running")
	s.ContainerFactory.New("test-stopped")
	containers, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 1)
	c.Assert(string(containers[0].Id()), gc.Equals, running.Name())
}

func (s *KVMSuite) TestCreateContainer(c *gc.C) {
	instance := containertesting.CreateContainer(c, s.manager, "1/kvm/0")
	name := string(instance.Id())
	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	containertesting.AssertCloudInit(c, cloudInitFilename)
}

func (s *KVMSuite) TestWriteTemplate(c *gc.C) {
	params := kvm.CreateMachineParams{
		Hostname:      "foo-bar",
		NetworkBridge: "br0",
		Interfaces: []network.InterfaceInfo{
			{MACAddress: "00:16:3e:20:b0:11"},
		},
	}
	tempDir := c.MkDir()

	templatePath := filepath.Join(tempDir, "kvm.xml")
	err := kvm.WriteTemplate(templatePath, params)
	c.Assert(err, jc.ErrorIsNil)
	templateBytes, err := ioutil.ReadFile(templatePath)
	c.Assert(err, jc.ErrorIsNil)

	template := string(templateBytes)

	c.Assert(template, jc.Contains, "<name>foo-bar</name>")
	c.Assert(template, jc.Contains, "<mac address='00:16:3e:20:b0:11'/>")
	c.Assert(template, jc.Contains, "<source bridge='br0'/>")
	c.Assert(strings.Count(string(template), "<interface type='bridge'>"), gc.Equals, 1)
}

func (s *KVMSuite) TestCreateMachineUsesTemplate(c *gc.C) {
	const uvtKvmBinName = "uvt-kvm"
	testing.PatchExecutableAsEchoArgs(c, s, uvtKvmBinName)

	tempDir := c.MkDir()
	params := kvm.CreateMachineParams{
		Hostname:      "foo-bar",
		NetworkBridge: "br0",
		Interfaces: []network.InterfaceInfo{
			{MACAddress: "00:16:3e:20:b0:11"},
		},
		UserDataFile: filepath.Join(tempDir, "something"),
	}

	err := kvm.CreateMachine(params)
	c.Assert(err, jc.ErrorIsNil)

	expectedArgs := []string{
		"create",
		"--log-console-output",
		"--user-data",
		filepath.Join(tempDir, "something"),
		"--template",
		filepath.Join(tempDir, "kvm-template.xml"),
		"foo-bar",
	}

	testing.AssertEchoArgs(c, uvtKvmBinName, expectedArgs...)
}

func (s *KVMSuite) TestDestroyContainer(c *gc.C) {
	instance := containertesting.CreateContainer(c, s.manager, "1/lxc/0")

	err := s.manager.DestroyContainer(instance.Id())
	c.Assert(err, jc.ErrorIsNil)

	name := string(instance.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), jc.IsDirectory)
}

// Test that CreateContainer creates proper startParams.
func (s *KVMSuite) TestCreateContainerUtilizesReleaseSimpleStream(c *gc.C) {

	envCfg, err := config.New(
		config.NoDefaults,
		dummy.SampleConfig().Merge(
			coretesting.Attrs{"image-stream": "released"},
		),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.Config = envCfg

	// CreateContainer sets TestStartParams internally; we call this
	// purely for the side-effect.
	containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)

	c.Assert(kvm.TestStartParams.ImageDownloadUrl, gc.Equals, "")
}

// Test that CreateContainer creates proper startParams.
func (s *KVMSuite) TestCreateContainerUtilizesDailySimpleStream(c *gc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.ImageStream = "daily"

	// CreateContainer sets TestStartParams internally; we call this
	// purely for the side-effect.
	containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)

	c.Assert(kvm.TestStartParams.ImageDownloadUrl, gc.Equals, "http://cloud-images.ubuntu.com/daily")
}

func (s *KVMSuite) TestStartContainerUtilizesSimpleStream(c *gc.C) {

	const libvirtBinName = "uvt-simplestreams-libvirt"
	testing.PatchExecutableAsEchoArgs(c, s, libvirtBinName)

	startParams := kvm.StartParams{
		Series:           "mocked-series",
		Arch:             "mocked-arch",
		ImageDownloadUrl: "mocked-url",
	}
	mockedContainer := kvm.NewEmptyKvmContainer()
	mockedContainer.Start(startParams)

	expectedArgs := strings.Split(
		fmt.Sprintf(
			"sync arch=%s release=%s --source=%s",
			startParams.Arch,
			startParams.Series,
			startParams.ImageDownloadUrl,
		),
		" ",
	)

	testing.AssertEchoArgs(c, libvirtBinName, expectedArgs...)
}

type ConstraintsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) TestDefaults(c *gc.C) {

	for _, test := range []struct {
		cons     string
		expected kvm.StartParams
		infoLog  []string
	}{{
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "mem=256M",
		expected: kvm.StartParams{
			Memory:   kvm.MinMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "mem=4G",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cpu-cores=4",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: 4,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cpu-cores=0",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.MinCpu,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "root-disk=512M",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.MinDisk,
		},
	}, {
		cons: "root-disk=4G",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: 4,
		},
	}, {
		cons: "arch=armhf",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`arch constraint of "armhf" being ignored as not supported`,
		},
	}, {
		cons: "container=lxc",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`container constraint of "lxc" being ignored as not supported`,
		},
	}, {
		cons: "cpu-power=100",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`cpu-power constraint of 100 being ignored as not supported`,
		},
	}, {
		cons: "tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}, {
		cons: "mem=4G cpu-cores=4 root-disk=20G arch=armhf cpu-power=100 container=lxc tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: 4,
			RootDisk: 20,
		},
		infoLog: []string{
			`arch constraint of "armhf" being ignored as not supported`,
			`container constraint of "lxc" being ignored as not supported`,
			`cpu-power constraint of 100 being ignored as not supported`,
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}} {
		var tw loggo.TestWriter
		c.Assert(loggo.RegisterWriter("constraint-tester", &tw, loggo.DEBUG), gc.IsNil)
		cons := constraints.MustParse(test.cons)
		params := kvm.ParseConstraintsToStartParams(cons)
		c.Check(params, gc.DeepEquals, test.expected)
		c.Check(tw.Log(), jc.LogMatches, test.infoLog)
		loggo.RemoveWriter("constraint-tester")
	}
}

// Test the output when no binary can be found.
func (s *KVMSuite) TestIsKVMSupportedKvmOkNotFound(c *gc.C) {
	// With no path, and no backup directory, we should fail.
	s.PatchEnvironment("PATH", "")
	s.PatchValue(kvm.KVMPath, "")

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, "kvm-ok executable not found")
}

// Test the output when the binary is found, but errors out.
func (s *KVMSuite) TestIsKVMSupportedBinaryErrorsOut(c *gc.C) {
	// Clear path so real binary is not found.
	s.PatchEnvironment("PATH", "")

	// Create mocked binary which returns an error and give the test access.
	tmpDir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash\nexit 127"), 0777)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(kvm.KVMPath, tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, jc.IsFalse)
	c.Assert(err, gc.ErrorMatches, "exit status 127")
}

// Test the case where kvm-ok is not in the path, but is in the
// specified directory.
func (s *KVMSuite) TestIsKVMSupportedNoPath(c *gc.C) {
	// Create a mocked binary so that this test does not fail for
	// developers without kvm-ok.
	s.PatchEnvironment("PATH", "")
	tmpDir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash"), 0777)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(kvm.KVMPath, tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

// Test the case that kvm-ok is found in the path.
func (s *KVMSuite) TestIsKVMSupportedOnlyPath(c *gc.C) {
	// Create a mocked binary so that this test does not fail for
	// developers without kvm-ok.
	tmpDir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash"), 0777)
	s.PatchEnvironment("PATH", tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *KVMSuite) TestKVMPathIsCorrect(c *gc.C) {
	c.Assert(*kvm.KVMPath, gc.Equals, "/usr/sbin")
}
