// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lxdprofile"
)

type ProfileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ProfileSuite{})

func (*ProfileSuite) TestEmptyTrue(c *gc.C) {
	p := lxdprofile.Profile{}
	c.Assert(p.Empty(), jc.IsTrue)
}

func (*ProfileSuite) TestEmptyFalse(c *gc.C) {
	p := lxdprofile.Profile{
		Config: map[string]string{
			"hello": "testing",
		}}
	c.Assert(p.Empty(), jc.IsFalse)
}

func (*ProfileSuite) TestValidateConfigDevices(c *gc.C) {
	p := lxdprofile.Profile{
		Config: map[string]string{
			"hello": "testing",
		}}
	c.Assert(p.ValidateConfigDevices(), jc.ErrorIsNil)
}

func (*ProfileSuite) TestValidateConfigDevicesBadConfigBoot(c *gc.C) {
	testValidateConfigDevicesBadConfig(map[string]string{"boot.testme": "testing"}, "boot.testme", c)
}

func (*ProfileSuite) TestValidateConfigDevicesBadConfigLimits(c *gc.C) {
	testValidateConfigDevicesBadConfig(map[string]string{"limits.new": "testing"}, "limits.new", c)
}

func (*ProfileSuite) TestValidateConfigDevicesBadConfigMigration(c *gc.C) {
	testValidateConfigDevicesBadConfig(map[string]string{"migration": "testing"}, "migration", c)
}

func (*ProfileSuite) TestValidateConfigDevicesBadDevice(c *gc.C) {
	p := lxdprofile.Profile{
		Devices: map[string]map[string]string{
			"test": {
				"type": "unix-disk",
			}}}
	c.Assert(p.ValidateConfigDevices(), gc.ErrorMatches, "invalid lxd-profile: contains device type \"unix-disk\"")
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceUnixChar(c *gc.C) {
	testValidateConfigDevicesGoodDevices("unix-char", c)
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceUnixBlock(c *gc.C) {
	testValidateConfigDevicesGoodDevices("unix-block", c)
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceGPU(c *gc.C) {
	testValidateConfigDevicesGoodDevices("gpu", c)
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceUSB(c *gc.C) {
	testValidateConfigDevicesGoodDevices("usb", c)
}

func testValidateConfigDevicesBadConfig(config map[string]string, value string, c *gc.C) {
	p := lxdprofile.Profile{Config: config}
	c.Assert(p.ValidateConfigDevices(), gc.ErrorMatches, fmt.Sprintf("invalid lxd-profile: contains config value %q", value))
}

func testValidateConfigDevicesGoodDevices(deviceType string, c *gc.C) {
	p := lxdprofile.Profile{
		Devices: map[string]map[string]string{
			"test": {
				"type": deviceType,
			}}}
	c.Assert(p.ValidateConfigDevices(), jc.ErrorIsNil)

}
