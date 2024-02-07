// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/kvm"
	"github.com/juju/juju/internal/container/kvm/mock"
	kvmtesting "github.com/juju/juju/internal/container/kvm/testing"
	containertesting "github.com/juju/juju/internal/container/testing"
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
	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:      coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey: imagemetadata.ReleasedStream,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (*KVMSuite) TestManagerModelUUIDNeeded(c *gc.C) {
	manager, err := kvm.NewContainerManager(container.ManagerConfig{container.ConfigModelUUID: ""})
	c.Assert(err, gc.ErrorMatches, "model UUID is required")
	c.Assert(manager, gc.IsNil)
}

func (*KVMSuite) TestManagerWarnsAboutUnknownOption(c *gc.C) {
	_, err := kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID: coretesting.ModelTag.Id(),
		"shazam":                  "Captain Marvel",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(c.GetTestLog(), jc.Contains, `INFO juju.container unused config option: "shazam" -> "Captain Marvel"`)
}

func (s *KVMSuite) TestListInitiallyEmpty(c *gc.C) {
	containers, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *KVMSuite) createRunningContainer(c *gc.C, name string) kvm.Container {
	kvmContainer := s.ContainerFactory.New(name)

	nics := network.InterfaceInfos{{
		InterfaceName: "eth0",
		InterfaceType: network.EthernetDevice,
		ConfigType:    network.ConfigDHCP,
	}}
	net := container.BridgeNetworkConfig(0, nics)
	c.Assert(kvmContainer.Start(kvm.StartParams{
		Version:      "12.10",
		Arch:         arch.HostArch(),
		UserDataFile: "userdata.txt",
		Network:      net}), gc.IsNil)
	return kvmContainer
}

func (s *KVMSuite) TestListMatchesManagerName(c *gc.C) {
	s.createRunningContainer(c, "juju-06f00d-match1")
	s.createRunningContainer(c, "juju-06f00d-match2")
	s.createRunningContainer(c, "testNoMatch")
	s.createRunningContainer(c, "other")
	containers, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 2)
	expectedIds := []instance.Id{"juju-06f00d-match1", "juju-06f00d-match2"}
	ids := []instance.Id{containers[0].Id(), containers[1].Id()}
	c.Assert(ids, jc.SameContents, expectedIds)
}

func (s *KVMSuite) TestListMatchesRunningContainers(c *gc.C) {
	running := s.createRunningContainer(c, "juju-06f00d-running")
	s.ContainerFactory.New("juju-06f00d-stopped")
	containers, err := s.manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 1)
	c.Assert(string(containers[0].Id()), gc.Equals, running.Name())
}

func (s *KVMSuite) TestCreateContainer(c *gc.C) {
	inst := containertesting.CreateContainer(c, s.manager, "1/kvm/0")
	name := string(inst.Id())
	cloudInitFilename := filepath.Join(s.ContainerDir, name, "cloud-init")
	containertesting.AssertCloudInit(c, cloudInitFilename)
}

// This test will pass regular unit tests, but is intended for the
// race-checking CI job to assert concurrent creation safety.
func (s *KVMSuite) TestCreateContainerConcurrent(c *gc.C) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			_ = containertesting.CreateContainer(c, s.manager, fmt.Sprintf("1/kvm/%d", idx))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func (s *KVMSuite) TestDestroyContainer(c *gc.C) {
	inst := containertesting.CreateContainer(c, s.manager, "1/kvm/0")

	err := s.manager.DestroyContainer(inst.Id())
	c.Assert(err, jc.ErrorIsNil)

	name := string(inst.Id())
	// Check that the container dir is no longer in the container dir
	c.Assert(filepath.Join(s.ContainerDir, name), jc.DoesNotExist)
	// but instead, in the removed container dir
	c.Assert(filepath.Join(s.RemovedDir, name), jc.IsDirectory)
}

// Test that CreateContainer creates proper startParams.
func (s *KVMSuite) TestCreateContainerUsesReleaseSimpleStream(c *gc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, jc.ErrorIsNil)

	inst := containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)
	startParams := kvm.ContainerFromInstance(inst).(*mock.MockContainer).StartParams
	c.Assert(startParams.ImageDownloadURL, gc.Equals, "")
	c.Assert(startParams.Stream, gc.Equals, "released")
}

// Test that CreateContainer creates proper startParams.
func (s *KVMSuite) TestCreateContainerUsesDailySimpleStream(c *gc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, jc.ErrorIsNil)

	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:      coretesting.ModelTag.Id(),
		config.ContainerImageStreamKey: "daily",
	})
	c.Assert(err, jc.ErrorIsNil)

	inst := containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)
	startParams := kvm.ContainerFromInstance(inst).(*mock.MockContainer).StartParams
	c.Assert(startParams.ImageDownloadURL, gc.Equals, "http://cloud-images.ubuntu.com/daily")
	c.Assert(startParams.Stream, gc.Equals, "daily")
}

func (s *KVMSuite) TestCreateContainerUsesSetImageMetadataURL(c *gc.C) {

	// Mock machineConfig with a mocked simple stream URL.
	instanceConfig, err := containertesting.MockMachineConfig("1/kvm/0")
	c.Assert(err, jc.ErrorIsNil)

	s.manager, err = kvm.NewContainerManager(container.ManagerConfig{
		container.ConfigModelUUID:           coretesting.ModelTag.Id(),
		config.ContainerImageMetadataURLKey: "https://images.linuxcontainers.org",
	})
	c.Assert(err, jc.ErrorIsNil)

	inst := containertesting.CreateContainerWithMachineConfig(c, s.manager, instanceConfig)
	startParams := kvm.ContainerFromInstance(inst).(*mock.MockContainer).StartParams
	c.Assert(startParams.ImageDownloadURL, gc.Equals, "https://images.linuxcontainers.org")
}

func (s *KVMSuite) TestImageAcquisitionUsesSimpleStream(c *gc.C) {

	startParams := kvm.StartParams{
		Version:          "mocked-version",
		Arch:             "mocked-arch",
		Stream:           "released",
		ImageDownloadURL: "mocked-url",
	}
	mockedContainer := kvm.NewEmptyKvmContainer()

	// We are testing only the logging side-effect, so the error is ignored.
	_ = mockedContainer.EnsureCachedImage(startParams)

	expectedArgs := fmt.Sprintf(
		"synchronise images for %s %s %s %s",
		startParams.Arch,
		startParams.Version,
		startParams.Stream,
		startParams.ImageDownloadURL,
	)
	c.Assert(c.GetTestLog(), jc.Contains, expectedArgs)
}

type ConstraintsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) TestDefaults(c *gc.C) {
	testCases := []struct {
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
		cons: "cores=4",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: 4,
			RootDisk: kvm.DefaultDisk,
		},
	}, {
		cons: "cores=0",
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
		cons: "arch=arm64",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`arch constraint of "arm64" being ignored as not supported`,
		},
	}, {
		cons: "container=lxd",
		expected: kvm.StartParams{
			Memory:   kvm.DefaultMemory,
			CpuCores: kvm.DefaultCpu,
			RootDisk: kvm.DefaultDisk,
		},
		infoLog: []string{
			`container constraint of "lxd" being ignored as not supported`,
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
		cons: "mem=4G cores=4 root-disk=20G arch=arm64 cpu-power=100 container=lxd tags=foo,bar",
		expected: kvm.StartParams{
			Memory:   4 * 1024,
			CpuCores: 4,
			RootDisk: 20,
		},
		infoLog: []string{
			`arch constraint of "arm64" being ignored as not supported`,
			`container constraint of "lxd" being ignored as not supported`,
			`cpu-power constraint of 100 being ignored as not supported`,
			`tags constraint of "foo,bar" being ignored as not supported`,
		},
	}}

	for _, test := range testCases {
		c.Logf("testing %q", test.cons)

		var tw loggo.TestWriter
		c.Assert(loggo.RegisterWriter("constraint-tester", &tw), gc.IsNil)
		cons := constraints.MustParse(test.cons)
		params := kvm.ParseConstraintsToStartParams(cons)
		c.Check(params, gc.DeepEquals, test.expected)
		c.Check(tw.Log(), jc.LogMatches, test.infoLog)
		_, _ = loggo.RemoveWriter("constraint-tester")
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
	err := os.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash\nexit 127"), 0777)
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
	err := os.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash"), 0777)
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
	err := os.WriteFile(filepath.Join(tmpDir, "kvm-ok"), []byte("#!/bin/bash"), 0777)
	c.Check(err, jc.ErrorIsNil)
	s.PatchEnvironment("PATH", tmpDir)

	supported, err := kvm.IsKVMSupported()
	c.Check(supported, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *KVMSuite) TestKVMPathIsCorrect(c *gc.C) {
	c.Assert(*kvm.KVMPath, gc.Equals, "/usr/sbin")
}
