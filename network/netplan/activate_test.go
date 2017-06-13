package netplan_test

import (
	"io/ioutil"
	"path"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network/netplan"
)

type ActivateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ActivateSuite{})

func (s *ActivateSuite) TestNoDevices(c *gc.C) {
	params := netplan.ActivationParams{}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "no devices specified")
}

func (s *ActivateSuite) TestNoDirectory(c *gc.C) {
	params := netplan.ActivationParams{
		Devices: []netplan.DeviceToBridge{
			netplan.DeviceToBridge{},
		},
		Directory: "/quite/for/sure/this/doesnotexists",
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "open /quite/for/sure/this/doesnotexists.*")
}

func (s *ActivateSuite) TestActivateSuccess(c *gc.C) {
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
		RunMode:   netplan.RunModeSuccess,
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = ioutil.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Check(result, gc.IsNil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *ActivateSuite) TestActivateFailure(c *gc.C) {
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
		RunMode:   netplan.RunModeFailure,
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = ioutil.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Assert(result, gc.NotNil)
	c.Check(result.Stdout, gc.DeepEquals, []byte("This is stdout"))
	c.Check(result.Stderr, gc.DeepEquals, []byte("This is stderr"))
	c.Check(result.Code, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, "bridge activation error code 1")
}

func (s *ActivateSuite) TestActivateTimeout(c *gc.C) {
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
		RunMode:   netplan.RunModeTimeout,
		Timeout:   1000,
		Clock:     clock.WallClock,
	}
	files := []string{"00.yaml", "01.yaml"}
	contents := make([][]byte, len(files))
	for i, file := range files {
		var err error
		contents[i], err = ioutil.ReadFile(path.Join("testdata/TestReadWriteBackup", file))
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(path.Join(tempDir, file), contents[i], 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	result, err := netplan.BridgeAndActivate(params)
	c.Assert(result, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "bridge activation error: command cancelled")
}
