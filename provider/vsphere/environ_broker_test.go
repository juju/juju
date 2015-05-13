// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/govmomi/vim25/methods"
	"github.com/juju/govmomi/vim25/soap"
	"github.com/juju/govmomi/vim25/types"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type environBrokerSuite struct {
	vsphere.BaseSuite
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environBrokerSuite) PrepareStartInstanceFakes(c *gc.C) {
	// Prevent falling over to the public datasource.
	s.BaseSuite.PatchValue(&imagemetadata.DefaultBaseURL, "")

	client := vsphere.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	s.FakeInstances(client)
	s.FakeAvailabilityZones(client, "z1")
	s.FakeAvailabilityZones(client, "z1")
	s.FakeAvailabilityZones(client, "z1")
	s.FakeCreateInstance(client, s.ServerUrl, c)
}

func (s *environBrokerSuite) CreateStartInstanceArgs(c *gc.C) environs.StartInstanceParams {
	tools := []*tools.Tools{{
		Version: version.Binary{Arch: arch.AMD64, Series: "trusty"},
		URL:     "https://example.org",
	}}

	cons := constraints.Value{}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(cons, "trusty")
	c.Assert(err, jc.ErrorIsNil)

	instanceConfig.Tools = tools[0]
	instanceConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	return environs.StartInstanceParams{
		InstanceConfig: instanceConfig,
		Tools:          tools,
		Constraints:    cons,
	}
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	_, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceWithNetworks(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.InstanceConfig.Networks = []string{"someNetwork"}
	_, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, gc.ErrorMatches, "starting instances with networks is not supported yet")
}

func (s *environBrokerSuite) TestStartInstanceWithUnsupportedConstraints(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.Tools[0].Version.Arch = "someArch"
	_, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, gc.ErrorMatches, "no matching images found for given constraints: .*")
}

// if tools for multiple architectures are avaliable, provider should filter tools by arch of the selected image
func (s *environBrokerSuite) TestStartInstanceFilterToolByArch(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
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
	startInstArgs.InstanceConfig.Tools = tools[0]
	res, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.Arch, gc.Equals, arch.AMD64)
	c.Assert(startInstArgs.InstanceConfig.Tools.Version.Arch, gc.Equals, arch.AMD64)
}

func (s *environBrokerSuite) TestStartInstanceDefaultConstraintsApplied(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	res, err := s.Env.StartInstance(startInstArgs)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.CpuCores, gc.Equals, vsphere.DefaultCpuCores)
	c.Assert(*res.Hardware.CpuPower, gc.Equals, vsphere.DefaultCpuPower)
	c.Assert(*res.Hardware.Mem, gc.Equals, vsphere.DefaultMemMb)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, common.MinRootDiskSizeGiB*1024)
}

func (s *environBrokerSuite) TestStartInstanceCustomConstraintsApplied(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
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

func (s *environBrokerSuite) TestStartInstanceCallsFinishMachineConfig(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	s.PatchValue(&vsphere.FinishInstanceConfig, func(mcfg *instancecfg.InstanceConfig, cfg *config.Config) (err error) {
		return errors.New("FinishMachineConfig called")
	})
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "FinishMachineConfig called")
}

func (s *environBrokerSuite) TestStartInstanceDefaultDiskSizeIsUsedForSmallConstraintValue(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	rootDisk := uint64(1000)
	startInstArgs.Constraints.RootDisk = &rootDisk
	res, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, common.MinRootDiskSizeGiB*1024)
}

func (s *environBrokerSuite) TestStartInstanceInvalidPlacement(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.Placement = "someInvalidPlacement"
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "unknown placement directive: .*")
}

func (s *environBrokerSuite) TestStartInstanceSelectZone(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	s.FakeAvailabilityZones(client, "z1", "z2")
	s.FakeAvailabilityZones(client, "z1", "z2")
	s.FakeCreateInstance(client, s.ServerUrl, c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.Placement = "zone=z2"
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStartInstanceCallsAvailabilityZoneAllocations(c *gc.C) {
	s.PrepareStartInstanceFakes(c)
	startInstArgs := s.CreateStartInstanceArgs(c)
	startInstArgs.DistributionGroup = func() ([]instance.Id, error) {
		return []instance.Id{instance.Id("someId")}, nil
	}
	s.PatchValue(&vsphere.AvailabilityZoneAllocations, func(env common.ZonedEnviron, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		c.Assert(len(group), gc.Equals, 1)
		c.Assert(string(group[0]), gc.Equals, "someId")
		return nil, errors.New("AvailabilityZoneAllocations called")
	})
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "AvailabilityZoneAllocations called")
}

func (s *environBrokerSuite) TestStartInstanceTriesToCreateInstanceInAllAvailabilityZones(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	s.FakeInstances(client)
	s.FakeAvailabilityZones(client, "z1", "z2")
	s.FakeAvailabilityZones(client, "z1", "z2")
	s.FakeAvailabilityZones(client, "z1", "z2")
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	client.SetProxyHandler("CreateImportSpec", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateImportSpecBody)
		resBody.Res = &types.CreateImportSpecResponse{
			Returnval: types.OvfCreateImportSpecResult{
				Error: []types.LocalizedMethodFault{{
					LocalizedMessage: "Error zone 1",
				}},
			},
		}
	})
	s.FakeAvailabilityZones(client, "z1", "z2")
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	client.SetProxyHandler("CreateImportSpec", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateImportSpecBody)
		resBody.Res = &types.CreateImportSpecResponse{
			Returnval: types.OvfCreateImportSpecResult{
				Error: []types.LocalizedMethodFault{{
					LocalizedMessage: "Error zone 2",
				}},
			},
		}
	})
	startInstArgs := s.CreateStartInstanceArgs(c)
	_, err := s.Env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "Can't create instance in any of availability zones, last error: Failed to import OVA file: Error zone 2")
}
