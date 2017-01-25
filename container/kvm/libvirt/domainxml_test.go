// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt_test

import (
	"encoding/xml"

	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/testing"

	. "github.com/juju/juju/container/kvm/libvirt"
	jc "github.com/juju/testing/checkers"
)

// gocheck boilerplate.
type domainXMLSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&domainXMLSuite{})

var amd64DomainStr = `
<domain type="kvm">
    <name>juju-someid</name>
    <vcpu>2</vcpu>
    <currentMemory unit="MiB">1024</currentMemory>
    <memory unit="MiB">1024</memory>
    <os>
        <type>hvm</type>
    </os>
    <devices>
        <disk device="disk" type="file">
            <driver type="qcow2" name="qemu"></driver>
            <source file="/some/path"></source>
            <target dev="vda"></target>
        </disk>
        <disk device="disk" type="file">
            <driver type="raw" name="qemu"></driver>
            <source file="/another/path"></source>
            <target dev="vdb"></target>
        </disk>
        <interface type="bridge">
            <mac address="00:00:00:00:00:00"></mac>
            <model type="virtio"></model>
            <source bridge="parent-dev"></source>
            <guest dev="device-name"></guest>
        </interface>
        <serial type="pty">
            <source path="/dev/pts/2"></source>
            <target port="0"></target>
        </serial>
        <console type="pty" tty="/dev/pts/2">
            <source path="/dev/pts/2"></source>
            <target port="0"></target>
        </console>
    </devices>
</domain>`[1:]

var arm64DomainStr = `
<domain type="kvm">
    <name>juju-someid</name>
    <vcpu>2</vcpu>
    <currentMemory unit="MiB">1024</currentMemory>
    <memory unit="MiB">1024</memory>
    <os>
        <type arch="aarch64" machine="virt">hvm</type>
        <loader readonly="yes" type="pflash">/shared/readonly.fd</loader>
    </os>
    <features>
        <gic version="host"></gic>
    </features>
    <cpu mode="custom" match="exact">
        <model fallback="allow">cortex-a53</model>
    </cpu>
    <devices>
        <disk device="disk" type="file">
            <driver type="qcow2" name="qemu"></driver>
            <source file="/some/path"></source>
            <target dev="vda"></target>
        </disk>
        <disk device="disk" type="file">
            <driver type="raw" name="qemu"></driver>
            <source file="/another/path"></source>
            <target dev="vdb"></target>
        </disk>
        <interface type="bridge">
            <mac address="00:00:00:00:00:00"></mac>
            <model type="virtio"></model>
            <source bridge="parent-dev"></source>
            <guest dev="device-name"></guest>
        </interface>
        <serial type="pty">
            <source path="/dev/pts/2"></source>
            <target port="0"></target>
        </serial>
        <console type="pty" tty="/dev/pts/2">
            <source path="/dev/pts/2"></source>
            <target port="0"></target>
        </console>
    </devices>
</domain>`[1:]

func (domainXMLSuite) TestNewDomain(c *gc.C) {
	table := []struct {
		arch, want string
	}{
		{"amd64", amd64DomainStr},
		{"arm64", arm64DomainStr},
	}
	for i, test := range table {
		c.Logf("TestNewDomain: test #%d for %s", i+1, test.arch)
		ifaces := []InterfaceInfo{
			dummyInterface{
				mac:    "00:00:00:00:00:00",
				parent: "parent-dev",
				name:   "device-name"}}
		disks := []DiskInfo{
			dummyDisk{driver: "qcow2", source: "/some/path"},
			dummyDisk{driver: "raw", source: "/another/path"},
		}
		params := dummyParams{ifaceInfo: ifaces, diskInfo: disks, memory: 1024, cpuCores: 2, hostname: "juju-someid", arch: test.arch}

		if test.arch == "arm64" {
			params.loader = "/shared/readonly.fd"
		}

		d, err := NewDomain(params)
		c.Check(err, jc.ErrorIsNil)
		ml, err := xml.MarshalIndent(&d, "", "    ")
		c.Check(err, jc.ErrorIsNil)
		c.Assert(string(ml), jc.DeepEquals, test.want)
	}
}

func (domainXMLSuite) TestNewDomainError(c *gc.C) {
	d, err := NewDomain(dummyParams{err: errors.Errorf("boom")})
	c.Check(d, jc.DeepEquals, Domain{})
	c.Check(err, gc.ErrorMatches, "boom")
}

type dummyParams struct {
	err       error
	arch      string
	cpuCores  uint64
	diskInfo  []DiskInfo
	hostname  string
	ifaceInfo []InterfaceInfo
	loader    string
	memory    uint64
	nvram     string
}

func (p dummyParams) Arch() string                 { return p.arch }
func (p dummyParams) CPUs() uint64                 { return p.cpuCores }
func (p dummyParams) DiskInfo() []DiskInfo         { return p.diskInfo }
func (p dummyParams) Host() string                 { return p.hostname }
func (p dummyParams) Loader() string               { return p.loader }
func (p dummyParams) NVRAM() string                { return p.nvram }
func (p dummyParams) NetworkInfo() []InterfaceInfo { return p.ifaceInfo }
func (p dummyParams) RAM() uint64                  { return p.memory }
func (p dummyParams) ValidateDomainParams() error  { return p.err }

type dummyDisk struct {
	source string
	driver string
}

func (d dummyDisk) Driver() string { return d.driver }
func (d dummyDisk) Source() string { return d.source }

type dummyInterface struct {
	mac, parent, name string
}

func (i dummyInterface) InterfaceName() string       { return i.name }
func (i dummyInterface) MACAddress() string          { return i.mac }
func (i dummyInterface) ParentInterfaceName() string { return i.parent }
