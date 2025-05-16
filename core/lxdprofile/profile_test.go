// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/internal/testhelpers"
)

type ProfileSuite struct {
	testhelpers.IsolationSuite
}

func TestProfileSuite(t *stdtesting.T) { tc.Run(t, &ProfileSuite{}) }
func (*ProfileSuite) TestEmptyTrue(c *tc.C) {
	p := lxdprofile.Profile{}
	c.Assert(p.Empty(), tc.IsTrue)
}

func (*ProfileSuite) TestEmptyFalse(c *tc.C) {
	p := lxdprofile.Profile{
		Config: map[string]string{
			"hello": "testing",
		}}
	c.Assert(p.Empty(), tc.IsFalse)
}

func (*ProfileSuite) TestValidateConfigDevices(c *tc.C) {
	p := lxdprofile.Profile{
		Config: map[string]string{
			"hello": "testing",
		}}
	c.Assert(p.ValidateConfigDevices(), tc.ErrorIsNil)
}

func (*ProfileSuite) TestValidateConfigDevicesBadConfigBoot(c *tc.C) {
	testValidateConfigDevicesBadConfig(map[string]string{"boot.testme": "testing"}, "boot.testme", c)
}

func (*ProfileSuite) TestValidateConfigDevicesBadConfigLimits(c *tc.C) {
	testValidateConfigDevicesBadConfig(map[string]string{"limits.new": "testing"}, "limits.new", c)
}

func (*ProfileSuite) TestValidateConfigDevicesBadConfigMigration(c *tc.C) {
	testValidateConfigDevicesBadConfig(map[string]string{"migration": "testing"}, "migration", c)
}

func (*ProfileSuite) TestValidateConfigDevicesBadDevice(c *tc.C) {
	p := lxdprofile.Profile{
		Devices: map[string]map[string]string{
			"test": {
				"type": "unix-disk",
			}}}
	c.Assert(p.ValidateConfigDevices(), tc.ErrorMatches, "invalid lxd-profile: contains device type \"unix-disk\"")
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceUnixChar(c *tc.C) {
	testValidateConfigDevicesGoodDevices("unix-char", c)
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceUnixBlock(c *tc.C) {
	testValidateConfigDevicesGoodDevices("unix-block", c)
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceGPU(c *tc.C) {
	testValidateConfigDevicesGoodDevices("gpu", c)
}

func (*ProfileSuite) TestValidateConfigDevicesGoodDeviceUSB(c *tc.C) {
	testValidateConfigDevicesGoodDevices("usb", c)
}

func testValidateConfigDevicesBadConfig(config map[string]string, value string, c *tc.C) {
	p := lxdprofile.Profile{Config: config}
	c.Assert(p.ValidateConfigDevices(), tc.ErrorMatches, fmt.Sprintf("invalid lxd-profile: contains config value %q", value))
}

func testValidateConfigDevicesGoodDevices(deviceType string, c *tc.C) {
	p := lxdprofile.Profile{
		Devices: map[string]map[string]string{
			"test": {
				"type": deviceType,
			}}}
	c.Assert(p.ValidateConfigDevices(), tc.ErrorIsNil)

}
