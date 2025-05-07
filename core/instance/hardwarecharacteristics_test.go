// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/instance"
)

type HardwareSuite struct{}

var _ = tc.Suite(&HardwareSuite{})

type parseHardwareTestSpec struct {
	summary string
	args    []string
	hc      *instance.HardwareCharacteristics
	err     string
}

type HC = instance.HardwareCharacteristics

func (ts *parseHardwareTestSpec) check(c *tc.C) {
	hwc, err := instance.ParseHardware(ts.args...)

	// Check the spec'ed error condition first.
	if ts.err != "" {
		c.Check(err, tc.ErrorMatches, ts.err)
		// We expected an error so we don't worry about checking hwc.
		return
	} else if !c.Check(err, jc.ErrorIsNil) {
		// We got an unexpected error so we don't worry about checking hwc.
		return
	}

	// The error condition matched so we check hwc.
	cons1, err := instance.ParseHardware(hwc.String())
	if !c.Check(err, jc.ErrorIsNil) {
		// Parsing didn't work so we don't worry about checking hwc.
		return
	}

	// Compare the round-tripped HWC.
	c.Check(cons1, tc.DeepEquals, hwc)

	// If ts.hc is provided, check that too (so we're not just relying on the
	// round trip via String).
	if ts.hc != nil {
		c.Check(hwc, tc.DeepEquals, *ts.hc)
	}
}

var parseHardwareTests = []parseHardwareTestSpec{
	// Simple errors.
	{
		summary: "nothing at all",
		hc:      &HC{},
	}, {
		summary: "empty",
		args:    []string{"     "},
		hc:      &HC{},
	}, {
		summary: "complete nonsense",
		args:    []string{"cheese"},
		err:     `malformed characteristic "cheese"`,
	}, {
		summary: "missing name",
		args:    []string{"=cheese"},
		err:     `malformed characteristic "=cheese"`,
	}, {
		summary: "unknown characteristic",
		args:    []string{"cheese=edam"},
		err:     `unknown characteristic "cheese"`,
	},

	// "arch" in detail.
	{
		summary: "set arch empty",
		args:    []string{"arch="},
		hc:      &HC{Arch: stringPtr("")},
	}, {
		summary: "set arch amd64",
		args:    []string{"arch=amd64"},
		hc:      &HC{Arch: stringPtr("amd64")},
	}, {
		summary: "set arch arm64",
		args:    []string{"arch=arm64"},
		hc:      &HC{Arch: stringPtr("arm64")},
	}, {
		summary: "set arch amd64 quoted",
		args:    []string{`arch="amd64"`},
		hc:      &HC{Arch: stringPtr("amd64")},
	}, {
		summary: "set nonsense arch 1",
		args:    []string{"arch=cheese"},
		err:     `bad "arch" characteristic: "cheese" not recognized`,
	}, {
		summary: "set nonsense arch 2",
		args:    []string{"arch=123.45"},
		err:     `bad "arch" characteristic: "123.45" not recognized`,
	}, {
		summary: "double set arch together",
		args:    []string{"arch=amd64 arch=amd64"},
		err:     `bad "arch" characteristic: already set`,
	}, {
		summary: "double set arch separately",
		args:    []string{"arch=arm64", "arch="},
		err:     `bad "arch" characteristic: already set`,
	},

	// "cores" in detail.
	{
		summary: "set cores empty",
		args:    []string{"cores="},
		hc:      &HC{CpuCores: uint64Ptr(0)},
	}, {
		summary: "set cores zero",
		args:    []string{"cores=0"},
		hc:      &HC{CpuCores: uint64Ptr(0)},
	}, {
		summary: "set cores",
		args:    []string{"cores=4"},
		hc:      &HC{CpuCores: uint64Ptr(4)},
	}, {
		summary: "set cores quoted",
		args:    []string{`cores="4"`},
		hc:      &HC{CpuCores: uint64Ptr(4)},
	}, {
		summary: "set nonsense cores 1",
		args:    []string{"cores=cheese"},
		err:     `bad "cores" characteristic: must be a non-negative integer`,
	}, {
		summary: "set nonsense cores 2",
		args:    []string{"cores=-1"},
		err:     `bad "cores" characteristic: must be a non-negative integer`,
	}, {
		summary: "set nonsense cores 3",
		args:    []string{"cores=123.45"},
		err:     `bad "cores" characteristic: must be a non-negative integer`,
	}, {
		summary: "double set cores together",
		args:    []string{"cores=128 cores=1"},
		err:     `bad "cores" characteristic: already set`,
	}, {
		summary: "double set cores separately",
		args:    []string{"cores=128", "cores=1"},
		err:     `bad "cores" characteristic: already set`,
	},

	// "cpu-power" in detail.
	{
		summary: "set cpu-power empty",
		args:    []string{"cpu-power="},
		hc:      &HC{CpuPower: uint64Ptr(0)},
	}, {
		summary: "set cpu-power zero",
		args:    []string{"cpu-power=0"},
		hc:      &HC{CpuPower: uint64Ptr(0)},
	}, {
		summary: "set cpu-power",
		args:    []string{"cpu-power=44"},
		hc:      &HC{CpuPower: uint64Ptr(44)},
	}, {
		summary: "set cpu-power quoted",
		args:    []string{`cpu-power="44"`},
		hc:      &HC{CpuPower: uint64Ptr(44)},
	}, {
		summary: "set nonsense cpu-power 1",
		args:    []string{"cpu-power=cheese"},
		err:     `bad "cpu-power" characteristic: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-power 2",
		args:    []string{"cpu-power=-1"},
		err:     `bad "cpu-power" characteristic: must be a non-negative integer`,
	}, {
		summary: "double set cpu-power together",
		args:    []string{"  cpu-power=300 cpu-power=1700 "},
		err:     `bad "cpu-power" characteristic: already set`,
	}, {
		summary: "double set cpu-power separately",
		args:    []string{"cpu-power=300  ", "  cpu-power=1700"},
		err:     `bad "cpu-power" characteristic: already set`,
	},

	// "mem" in detail.
	{
		summary: "set mem empty",
		args:    []string{"mem="},
		hc:      &HC{Mem: uint64Ptr(0)},
	}, {
		summary: "set mem zero",
		args:    []string{"mem=0"},
		hc:      &HC{Mem: uint64Ptr(0)},
	}, {
		summary: "set mem without suffix",
		args:    []string{"mem=512"},
		hc:      &HC{Mem: uint64Ptr(512)},
	}, {
		summary: "set mem with M suffix",
		args:    []string{"mem=512M"},
		hc:      &HC{Mem: uint64Ptr(512)},
	}, {
		summary: "set mem with G suffix",
		args:    []string{"mem=1.5G"},
		hc:      &HC{Mem: uint64Ptr(1536)},
	}, {
		summary: "set mem with T suffix",
		args:    []string{"mem=36.2T"},
		hc:      &HC{Mem: uint64Ptr(37958452)},
	}, {
		summary: "set mem with P suffix",
		args:    []string{"mem=18.9P"},
		hc:      &HC{Mem: uint64Ptr(20293720474)},
	}, {
		summary: "set mem quoted",
		args:    []string{`mem="42M"`},
		hc:      &HC{Mem: uint64Ptr(42)},
	}, {
		summary: "set nonsense mem 1",
		args:    []string{"mem=cheese"},
		err:     `bad "mem" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 2",
		args:    []string{"mem=-1"},
		err:     `bad "mem" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 3",
		args:    []string{"mem=32Y"},
		err:     `bad "mem" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set mem together",
		args:    []string{"mem=1G  mem=2G"},
		err:     `bad "mem" characteristic: already set`,
	}, {
		summary: "double set mem separately",
		args:    []string{"mem=1G", "mem=2G"},
		err:     `bad "mem" characteristic: already set`,
	},

	// "root-disk" in detail.
	{
		summary: "set root-disk empty",
		args:    []string{"root-disk="},
		hc:      &HC{RootDisk: uint64Ptr(0)},
	}, {
		summary: "set root-disk zero",
		args:    []string{"root-disk=0"},
		hc:      &HC{RootDisk: uint64Ptr(0)},
	}, {
		summary: "set root-disk without suffix",
		args:    []string{"root-disk=512"},
		hc:      &HC{RootDisk: uint64Ptr(512)},
	}, {
		summary: "set root-disk with M suffix",
		args:    []string{"root-disk=512M"},
		hc:      &HC{RootDisk: uint64Ptr(512)},
	}, {
		summary: "set root-disk with G suffix",
		args:    []string{"root-disk=1.5G"},
		hc:      &HC{RootDisk: uint64Ptr(1536)},
	}, {
		summary: "set root-disk with T suffix",
		args:    []string{"root-disk=36.2T"},
		hc:      &HC{RootDisk: uint64Ptr(37958452)},
	}, {
		summary: "set root-disk with P suffix",
		args:    []string{"root-disk=18.9P"},
		hc:      &HC{RootDisk: uint64Ptr(20293720474)},
	}, {
		summary: "set root-disk quoted",
		args:    []string{`root-disk="1234M"`},
		hc:      &HC{RootDisk: uint64Ptr(1234)},
	}, {
		summary: "set nonsense root-disk 1",
		args:    []string{"root-disk=cheese"},
		err:     `bad "root-disk" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense root-disk 2",
		args:    []string{"root-disk=-1"},
		err:     `bad "root-disk" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense root-disk 3",
		args:    []string{"root-disk=32Y"},
		err:     `bad "root-disk" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set root-disk together",
		args:    []string{"root-disk=1G  root-disk=2G"},
		err:     `bad "root-disk" characteristic: already set`,
	}, {
		summary: "double set root-disk separately",
		args:    []string{"root-disk=1G", "root-disk=2G"},
		err:     `bad "root-disk" characteristic: already set`,
	},

	// root-disk-source in detail.
	{
		summary: "set root-disk-source empty",
		args:    []string{"root-disk-source="},
		hc:      &HC{RootDiskSource: nil},
	}, {
		summary: "set root-disk-source",
		args:    []string{"root-disk-source=something"},
		hc:      &HC{RootDiskSource: stringPtr("something")},
	}, {
		summary: "set root-disk-source quoted",
		args:    []string{`root-disk-source="Foo Bar"`},
		hc:      &HC{RootDiskSource: stringPtr("Foo Bar")},
	}, {
		summary: "set root-disk-source quoted - other whitespace",
		args:    []string{`root-disk-source="\r\n\t"`},
		hc:      &HC{RootDiskSource: stringPtr("\r\n\t")},
	}, {
		summary: "set root-disk-source quoted (with escapes)",
		args:    []string{`root-disk-source="My Big \"Fat\" Greek Disk"`},
		hc:      &HC{RootDiskSource: stringPtr(`My Big "Fat" Greek Disk`)},
	}, {
		summary: "set root-disk-source quoted (no end quote)",
		args:    []string{`root-disk-source="foo`},
		err:     `root-disk-source: parsing quoted string: literal not terminated`,
	}, {
		summary: "set root-disk-source quoted (invalid escape)",
		args:    []string{`root-disk-source="foo\zbar"`},
		err:     `root-disk-source: parsing quoted string: invalid char escape`,
	}, {
		summary: "double set root-disk-source together",
		args:    []string{"root-disk-source=something root-disk-source=something-else"},
		err:     `bad "root-disk-source" characteristic: already set`,
	}, {
		summary: "doubles set root-disk-source separately",
		args:    []string{"root-disk-source=something", "root-disk-source=something-else"},
		err:     `bad "root-disk-source" characteristic: already set`,
	},

	// "tags" in detail.
	{
		summary: "set tags empty",
		args:    []string{"tags="},
		hc:      &HC{Tags: nil},
	}, {
		summary: "set tags empty (quoted)",
		args:    []string{`tags=""`},
		hc:      &HC{Tags: nil},
	}, {
		summary: "set tags single",
		args:    []string{"tags=abc"},
		hc:      &HC{Tags: &[]string{"abc"}},
	}, {
		summary: "set tags multi",
		args:    []string{"tags=ab,c,def"},
		hc:      &HC{Tags: &[]string{"ab", "c", "def"}},
	}, {
		summary: "set tags single quoted",
		args:    []string{`tags="one tag"`},
		hc:      &HC{Tags: &[]string{"one tag"}},
	}, {
		summary: "set tags multi quoted",
		args:    []string{`tags="ab",c,"d e f","g,h"`},
		hc:      &HC{Tags: &[]string{"ab", "c", "d e f", "g,h"}},
	}, {
		summary: "set tags multi quoted with no comma",
		args:    []string{`tags="ab""c"`},
		err:     `tags: expected comma after quoted value`,
	}, {
		summary: "double set tags together",
		args:    []string{"tags=ab,c tags=def"},
		err:     `bad "tags" characteristic: already set`,
	}, {
		summary: "double set tags separately",
		args:    []string{"tags=ab,c", "tags=def"},
		err:     `bad "tags" characteristic: already set`,
	},

	// "availability-zone" in detail.
	{
		summary: "set availability-zone empty",
		args:    []string{"availability-zone="},
		hc:      &HC{AvailabilityZone: nil},
	}, {
		summary: "set availability-zone non-empty",
		args:    []string{"availability-zone=a_zone"},
		hc:      &HC{AvailabilityZone: stringPtr("a_zone")},
	}, {
		summary: "set availability-zone quoted",
		args:    []string{`availability-zone="A Zone"`},
		hc:      &HC{AvailabilityZone: stringPtr("A Zone")},
	}, {
		summary: "set availability-zone quoted multi errors",
		args:    []string{`availability-zone="a b",c`},
		err:     `malformed characteristic ",c"`,
	}, {
		summary: "double set availability-zone together",
		args:    []string{"availability-zone=a_zone availability-zone=a_zone"},
		err:     `bad "availability-zone" characteristic: already set`,
	}, {
		summary: "double set availability-zone separately",
		args:    []string{"availability-zone=a_zone", "availability-zone="},
		err:     `bad "availability-zone" characteristic: already set`,
	},

	// "virt-type" in detail.
	{
		summary: "set virt-type empty",
		args:    []string{"virt-type="},
		hc:      &HC{VirtType: nil},
	}, {
		summary: "set virt-type non-empty",
		args:    []string{"virt-type=container"},
		hc:      &HC{VirtType: stringPtr("container")},
	}, {
		summary: "set virt-type quoted",
		args:    []string{`virt-type="container"`},
		hc:      &HC{VirtType: stringPtr("container")},
	}, {
		summary: "set virt-type quoted multi errors",
		args:    []string{`virt-type="container",virtual-machine`},
		err:     `malformed characteristic ",virtual-machine"`,
	}, {
		summary: "double set virt-type together",
		args:    []string{"virt-type=container virt-type=container"},
		err:     `bad "virt-type" characteristic: already set`,
	}, {
		summary: "double set virt-type separately",
		args:    []string{"virt-type=container", "virt-type="},
		err:     `bad "virt-type" characteristic: already set`,
	},

	// Everything at once.
	{
		summary: "kitchen sink together",
		args:    []string{" root-disk=4G mem=2T  arch=arm64  cores=4096 cpu-power=9001 availability-zone=a_zone virt-type=container"},
		hc: &HC{
			RootDisk:         uint64Ptr(4096),
			Mem:              uint64Ptr(2097152),
			Arch:             stringPtr("arm64"),
			CpuCores:         uint64Ptr(4096),
			CpuPower:         uint64Ptr(9001),
			AvailabilityZone: stringPtr("a_zone"),
			VirtType:         stringPtr("container"),
		},
	}, {
		summary: "kitchen sink separately",
		args:    []string{"root-disk=4G", "mem=2T", "cores=4096", "cpu-power=9001", "arch=arm64", "availability-zone=a_zone", "virt-type=container"},
		hc: &HC{
			RootDisk:         uint64Ptr(4096),
			Mem:              uint64Ptr(2097152),
			Arch:             stringPtr("arm64"),
			CpuCores:         uint64Ptr(4096),
			CpuPower:         uint64Ptr(9001),
			AvailabilityZone: stringPtr("a_zone"),
			VirtType:         stringPtr("container"),
		},
	}, {
		summary: "kitchen sink together quoted",
		args:    []string{`root-disk=4G mem=2T arch=arm64 cores=4096 cpu-power=9001 availability-zone="A Zone" tags="a b" virt-type="container"`},
		hc: &HC{
			RootDisk:         uint64Ptr(4096),
			Mem:              uint64Ptr(2097152),
			Arch:             stringPtr("arm64"),
			CpuCores:         uint64Ptr(4096),
			CpuPower:         uint64Ptr(9001),
			AvailabilityZone: stringPtr("A Zone"),
			Tags:             &[]string{"a b"},
			VirtType:         stringPtr("container"),
		},
	},
}

func stringPtr(s string) *string { return &s }
func uint64Ptr(u uint64) *uint64 { return &u }

func (s *HardwareSuite) TestParseHardware(c *tc.C) {
	for i, t := range parseHardwareTests {
		c.Logf("test %d: %s", i, t.summary)
		t.check(c)
	}
}

func (s HardwareSuite) TestClone(c *tc.C) {
	var hcNil *instance.HardwareCharacteristics
	c.Assert(hcNil.Clone(), tc.IsNil)
	hc := instance.MustParseHardware("root-disk=4G", "mem=2T", "cores=4096", "cpu-power=9001", "arch=arm64", "availability-zone=a_zone", "virt-type=virtual-machine")
	hc2 := hc.Clone()
	c.Assert(hc, jc.DeepEquals, *hc2)
}

// Regression test for https://bugs.launchpad.net/juju/+bug/1895756
func (s HardwareSuite) TestCloneSpace(c *tc.C) {
	az := "a -"
	hc := &instance.HardwareCharacteristics{AvailabilityZone: &az}
	clone := hc.Clone()
	c.Assert(hc, jc.DeepEquals, clone)
}

// Ensure fields like the Tags slice are deep-copied
func (s HardwareSuite) TestCloneDeep(c *tc.C) {
	tags := []string{"a"}
	hc := &instance.HardwareCharacteristics{Tags: &tags}
	clone := hc.Clone()
	c.Assert(hc, jc.DeepEquals, clone)
	tags[0] = "z"
	c.Assert((*clone.Tags)[0], tc.Equals, "a")
}
