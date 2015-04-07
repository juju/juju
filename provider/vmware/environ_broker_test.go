// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vmware"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type environBrokerSuite struct {
	vmware.BaseSuite
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environBrokerSuite) PrepareStartInstanceFakes() {
	client := vmware.ExposeEnvFakeClient(s.Env)
	s.FakeInstances(client)
	s.FakeInstances(client)
	s.FakeAvailabilityZones(client, "z1")
	s.FakeAvailabilityZones(client, "z1")
	s.FakeAvailabilityZones(client, "z1")
	s.FakeCreateInstance(client, s.ServerUrl)
}

func (s *environBrokerSuite) CreateStartInstanceArgs(c *gc.C) environs.StartInstanceParams {
	tools := []*tools.Tools{{
		Version: version.Binary{Arch: arch.AMD64, Series: "trusty"},
		URL:     "https://example.org",
	}}

	cons := constraints.Value{}

	machineConfig, err := environs.NewBootstrapMachineConfig(cons, "trusty")
	c.Assert(err, jc.ErrorIsNil)

	machineConfig.Tools = tools[0]
	machineConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	return environs.StartInstanceParams{
		MachineConfig: machineConfig,
		Tools:         tools,
		Constraints:   cons,
	}
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	_, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceWithNetworks(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.MachineConfig.Networks = []string{"someNetwork"}
	_, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, gc.ErrorMatches, "starting instances with networks is not supported yet")
}

func (s *environBrokerSuite) TestStartInstanceWithUnsupportedConstraints(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.Tools[0].Version.Arch = "someArch"
	_, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, gc.ErrorMatches, "No mathicng images found for given constraints: .*")
}

// if tools for multiple architectures are avaliable, provider should filter tools by arch of the selected image
func (s *environBrokerSuite) TestStartInstanceFilterToolByArch(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	tools := []*tools.Tools{{
		Version: version.Binary{Arch: arch.I386, Series: "trusty"},
		URL:     "https://example.org",
	}, {
		Version: version.Binary{Arch: arch.AMD64, Series: "trusty"},
		URL:     "https://example.org",
	}}
	//setting tools to I386, but provider should update them to AMD64, because our fake simplestream server return only AMD 64 image
	startInstArgs.Tools = tools
	startInstArgs.MachineConfig.Tools = tools[0]
	res, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.Arch, gc.Equals, arch.AMD64)
	c.Assert(startInstArgs.MachineConfig.Tools.Version.Arch, gc.Equals, arch.AMD64)
}

func (s *environBrokerSuite) TestStartInstanceDefaultConstraintsApplied(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	res, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.CpuCores, gc.Equals, vmware.DefaultCpuCores)
	c.Assert(*res.Hardware.CpuPower, gc.Equals, vmware.DefaultCpuPower)
	c.Assert(*res.Hardware.Mem, gc.Equals, vmware.DefaultMemMb)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, common.MinRootDiskSizeMB)
}

func (s *environBrokerSuite) TestStartInstanceCustomConstraintsApplied(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	cpuCores := uint64(4)
	startInstArgs.Constraints.CpuCores = &cpuCores
	cpuPower := uint64(2001)
	startInstArgs.Constraints.CpuPower = &cpuPower
	mem := uint64(2002)
	startInstArgs.Constraints.Mem = &mem
	rootDisk := uint64(10003)
	startInstArgs.Constraints.RootDisk = &rootDisk
	res, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.CpuCores, gc.Equals, cpuCores)
	c.Assert(*res.Hardware.CpuPower, gc.Equals, cpuPower)
	c.Assert(*res.Hardware.Mem, gc.Equals, mem)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, rootDisk)

}

func (s *environBrokerSuite) TestStartInstanceDefaultDiskSizeIsUsedForSmallConstraintValue(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	rootDisk := uint64(1000)
	startInstArgs.Constraints.RootDisk = &rootDisk
	res, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, common.MinRootDiskSizeMB)
}

func (s *environBrokerSuite) TestStartInstanceInvalidPlacement(c *gc.C) {
	s.PrepareStartInstanceFakes()
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.Placement = "someInvalidPlacement"
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "unknown placement directive: .*")
}

func (s *environBrokerSuite) TestStartInstanceSelectZone(c *gc.C) {
	client := vmware.ExposeEnvFakeClient(s.Env)
	s.FakeAvailabilityZones(client, "z1", "z2")
	s.FakeCreateInstance(client, s.ServerUrl)
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.Placement = "zone=z2"
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
}
