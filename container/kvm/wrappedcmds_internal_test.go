// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"errors"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/kvm/libvirt"
)

type libvirtInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&libvirtInternalSuite{})

func (libvirtInternalSuite) TestWriteMetadata(c *gc.C) {
	d := c.MkDir()

	err := writeMetadata(d)
	c.Check(err, jc.ErrorIsNil)
	b, err := ioutil.ReadFile(filepath.Join(d, metadata))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Matches, `{"instance-id": ".*-.*-.*-.*"}`)
}

func (libvirtInternalSuite) TestWriteDomainXMLSucceeds(c *gc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
			diskInfo{
				source: "/path",
				driver: "qcow2"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Matches, `/tmp/.*/\d+/host00.xml`)
}

func (libvirtInternalSuite) TestWriteDomainXMLMissingValidSystemDisk(c *gc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
			diskInfo{
				source: "/path",
				driver: "raw"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, gc.ErrorMatches, "missing system disk")
	c.Assert(got, gc.Matches, "")
}

func (libvirtInternalSuite) TestWriteDomainXMLMissingOneDisk(c *gc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, gc.ErrorMatches, "got 1 disks, need at least 2")
	c.Assert(got, gc.Matches, "")
}

func (libvirtInternalSuite) TestWriteDomainXMLMissingBothDisk(c *gc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		Hostname: "host00",
		runCmd:   stub.Run,
		disks:    []libvirt.DiskInfo{},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, gc.ErrorMatches, "got 0 disks, need at least 2")
	c.Assert(got, gc.Matches, "")
}

func (libvirtInternalSuite) TestWriteDomainXMLNoHostname(c *gc.C) {
	d := c.MkDir()

	stub := &runStub{}

	p := CreateMachineParams{
		runCmd: stub.Run,
		disks: []libvirt.DiskInfo{
			diskInfo{
				source: "/path-ds",
				driver: "raw"},
			diskInfo{
				source: "/path",
				driver: "qcow"},
		},
	}

	got, err := writeDomainXML(d, p)
	c.Assert(err, gc.ErrorMatches, "missing required hostname")
	c.Assert(got, gc.Matches, "")
}

func (libvirtInternalSuite) TestPoolInfoSuccess(c *gc.C) {
	output := `
Name:           juju-pool
UUID:           06ebee2d-6bd0-4f47-a7dc-dea555fdaa3b
State:          running
Persistent:     yes
Autostart:      yes
Capacity:       35.31 GiB
Allocation:     3.54 GiB
Available:      31.77 GiB
`
	stub := runStub{output: output}
	got, err := poolInfo(stub.Run)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, &libvirtPool{Name: "juju-pool", Autostart: "yes", State: "running"})

}

func (libvirtInternalSuite) TestPoolInfoNoPool(c *gc.C) {
	stub := runStub{err: errors.New("boom")}
	got, err := poolInfo(stub.Run)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.IsNil)
}
