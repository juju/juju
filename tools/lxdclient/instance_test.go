// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"fmt"
	"math"
	"time"

	jc "github.com/juju/testing/checkers"
	lxdshared "github.com/lxc/lxd/shared"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools/lxdclient"
)

type instanceSuite struct {
	jujutesting.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

var templateContainerInfo = lxdshared.ContainerInfo{
	Architecture: "x86_64",
	Config: map[string]string{
		"limits.cpu":     "2",
		"limits.memory":  "256MB",
		"user.something": "something value",
	},
	CreationDate:    time.Now(),
	Devices:         nil,
	Ephemeral:       false,
	ExpandedConfig:  nil,
	ExpandedDevices: nil,
	Name:            "container-name",
	Profiles:        []string{""},
	Status:          lxdshared.Starting.String(),
	StatusCode:      lxdshared.Starting,
}

func (s *instanceSuite) TestNewInstanceSummaryTemplate(c *gc.C) {
	archStr, err := lxdshared.ArchitectureName(lxdshared.ARCH_64BIT_INTEL_X86)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(templateContainerInfo.Architecture, gc.Equals, archStr)
	summary := lxdclient.NewInstanceSummary(&templateContainerInfo)
	c.Check(summary.Name, gc.Equals, "container-name")
	c.Check(summary.Status, gc.Equals, lxdclient.StatusStarting)
	c.Check(summary.Hardware.Architecture, gc.Equals, "amd64")
	c.Check(summary.Hardware.NumCores, gc.Equals, uint(2))
	c.Check(summary.Hardware.MemoryMB, gc.Equals, uint(256))
	// NotImplemented yet
	c.Check(summary.Hardware.RootDiskMB, gc.Equals, uint64(0))
	c.Check(summary.Metadata, gc.DeepEquals, map[string]string{"something": "something value"})
}

func infoWithMemory(mem string) *lxdshared.ContainerInfo {
	info := templateContainerInfo
	info.Config = map[string]string{
		"limits.memory": mem,
	}
	return &info
}

func (s *instanceSuite) TestNewInstanceSummaryMemory(c *gc.C) {
	// No suffix
	summary := lxdclient.NewInstanceSummary(infoWithMemory("128"))
	c.Check(summary.Hardware.MemoryMB, gc.Equals, uint(0))
	// Invalid integer
	summary = lxdclient.NewInstanceSummary(infoWithMemory("blah"))
	c.Check(summary.Hardware.MemoryMB, gc.Equals, uint(0))
	// Too big to fit in uint
	tooBig := fmt.Sprintf("%vMB", uint64(math.MaxUint32)+1)
	summary = lxdclient.NewInstanceSummary(infoWithMemory(tooBig))
	c.Check(summary.Hardware.MemoryMB, gc.Equals, uint(math.MaxUint32))
	// Just big enough
	justEnough := fmt.Sprintf("%vMB", uint(math.MaxUint32)-1)
	summary = lxdclient.NewInstanceSummary(infoWithMemory(justEnough))
	c.Check(summary.Hardware.MemoryMB, gc.Equals, uint(math.MaxUint32-1))
}

func infoWithArchitecture(arch int) *lxdshared.ContainerInfo {
	info := templateContainerInfo
	info.Architecture, _ = lxdshared.ArchitectureName(arch)
	return &info
}

func (s *instanceSuite) TestNewInstanceSummaryArchitectures(c *gc.C) {
	summary := lxdclient.NewInstanceSummary(infoWithArchitecture(lxdshared.ARCH_32BIT_INTEL_X86))
	c.Check(summary.Hardware.Architecture, gc.Equals, "i386")
	summary = lxdclient.NewInstanceSummary(infoWithArchitecture(lxdshared.ARCH_64BIT_INTEL_X86))
	c.Check(summary.Hardware.Architecture, gc.Equals, "amd64")
	summary = lxdclient.NewInstanceSummary(infoWithArchitecture(lxdshared.ARCH_64BIT_POWERPC_LITTLE_ENDIAN))
	c.Check(summary.Hardware.Architecture, gc.Equals, "ppc64el")
	info := templateContainerInfo
	info.Architecture = "unknown"
	summary = lxdclient.NewInstanceSummary(&info)
	c.Check(summary.Hardware.Architecture, gc.Equals, "unknown")
}
