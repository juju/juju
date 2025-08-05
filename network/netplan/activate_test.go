// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package netplan_test

import (
	"os"
	"path"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network/netplan"
	coretesting "github.com/juju/juju/testing"
)

type ActivateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ActivateSuite{})

func (s *ActivateSuite) TestNoDevices(c *gc.C) {
	params := netplan.ActivationParams{}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "no devices specified")
}

func (s *ActivateSuite) TestNoDirectory(c *gc.C) {
	params := netplan.ActivationParams{
		Devices: []netplan.DeviceToBridge{
			{},
		},
		Directory: "/quite/for/sure/this/doesnotexists",
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "open /quite/for/sure/this/doesnotexists.*")
}

func (s *ActivateSuite) TestActivateSuccess(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	tempDir := c.MkDir()
	params := netplan.ActivationParams{
		Devices: []netplan.DeviceToBridge{
			{
				DeviceName: "eno1",
				MACAddress: "00:11:22:33:44:99", // That's a wrong MAC, we should fall back to name
				BridgeName: "br-eno1",
			},
			{
				DeviceName: "eno2",
				MACAddress: "00:11:22:33:44:66",
				BridgeName: "br-eno2",
			},
		},
		Directory: tempDir,
		RunPrefix: "exit 0 &&",
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = os.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = os.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ActivateSuite) TestActivateDeviceAndVLAN(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	tempDir := c.MkDir()
	params := netplan.ActivationParams{
		Devices: []netplan.DeviceToBridge{
			{
				DeviceName: "eno1",
				MACAddress: "00:11:22:33:44:99", // That's a wrong MAC, we should fall back to name
				BridgeName: "br-eno1",
			},
			{
				DeviceName: "eno1.123",
				MACAddress: "00:11:22:33:44:99",
				BridgeName: "br-eno1.123",
			},
		},
		Directory: tempDir,
		RunPrefix: "exit 0 &&",
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = os.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = os.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ActivateSuite) TestActivateFailure(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	tempDir := c.MkDir()
	params := netplan.ActivationParams{
		Devices: []netplan.DeviceToBridge{
			{
				DeviceName: "eno1",
				MACAddress: "00:11:22:33:44:55",
				BridgeName: "br-eno1",
			},
			{
				DeviceName: "eno2",
				MACAddress: "00:11:22:33:44:66",
				BridgeName: "br-eno2",
			},
		},
		Directory: tempDir,
		RunPrefix: `echo -n "This is stdout" && echo -n "This is stderr" >&2 && exit 1 && `,
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = os.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = os.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Assert(result, gc.NotNil)
	c.Check(result.Stdout, gc.DeepEquals, "This is stdout")
	c.Check(result.Stderr, gc.DeepEquals, "This is stderr")
	c.Check(result.Code, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, "activating bridge: error code 1")

	// old files are in place and unchanged
	for i, file := range files {
		content, err := os.ReadFile(path.Join(tempDir, file))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(content), gc.Equals, string(contents[i]))
	}
	// there are no other YAML files in this directory
	dirEntries, err := os.ReadDir(tempDir)
	c.Assert(err, jc.ErrorIsNil)

	yamlCount := 0
	for _, entry := range dirEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			yamlCount++
		}
	}
	c.Assert(yamlCount, gc.Equals, len(files))
}

func (s *ActivateSuite) TestActivateTimeout(c *gc.C) {
	//	coretesting.SkipIfWindowsBug(c, "lp:1771077")
	tempDir := c.MkDir()
	params := netplan.ActivationParams{
		Devices: []netplan.DeviceToBridge{
			{
				DeviceName: "eno1",
				MACAddress: "00:11:22:33:44:55",
				BridgeName: "br-eno1",
			},
			{
				DeviceName: "eno2",
				MACAddress: "00:11:22:33:44:66",
				BridgeName: "br-eno2",
			},
		},
		Directory: tempDir,
		RunPrefix: "sleep 10000 && ",
		Timeout:   1000,
		Clock:     clock.WallClock,
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = os.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = os.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	_, err := netplan.BridgeAndActivate(params)
	c.Assert(err, gc.ErrorMatches, `activating bridge: running command "sleep 10000 && netplan generate && netplan apply && sleep 10": command cancelled`)
}
