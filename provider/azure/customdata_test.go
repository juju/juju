// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
)

type customDataSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&customDataSuite{})

// makeMachineConfig produces a valid cloudinit machine config.
func makeMachineConfig(c *gc.C) *cloudinit.MachineConfig {
	machineID := "0"
	return &cloudinit.MachineConfig{
		MachineId:          machineID,
		MachineNonce:       "gxshasqlnng",
		DataDir:            environs.DataDir,
		LogDir:             environs.LogDir,
		CloudInitOutputLog: environs.CloudInitOutputLog,
		RsyslogConfPath:    environs.RsyslogConfPath,
		Tools:              &tools.Tools{URL: "file://" + c.MkDir()},
		StateInfo: &state.Info{
			CACert:   []byte(testing.CACert),
			Addrs:    []string{"127.0.0.1:123"},
			Tag:      names.MachineTag(machineID),
			Password: "password",
		},
		APIInfo: &api.Info{
			CACert: []byte(testing.CACert),
			Addrs:  []string{"127.0.0.1:123"},
			Tag:    names.MachineTag(machineID),
		},
		SyslogPort:              2345,
		MachineAgentServiceName: "jujud-machine-0",
	}
}

// makeBadMachineConfig produces a cloudinit machine config that cloudinit
// will reject as invalid.
func makeBadMachineConfig() *cloudinit.MachineConfig {
	// As it happens, a default-initialized config is invalid.
	return &cloudinit.MachineConfig{}
}

func (*customDataSuite) TestMakeCustomDataPropagatesError(c *gc.C) {
	_, err := makeCustomData(makeBadMachineConfig())
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "failure while generating custom data: invalid machine configuration: invalid machine id")
}

func (*customDataSuite) TestMakeCustomDataEncodesUserData(c *gc.C) {
	cfg := makeMachineConfig(c)

	encodedData, err := makeCustomData(cfg)
	c.Assert(err, gc.IsNil)

	data, err := base64.StdEncoding.DecodeString(encodedData)
	c.Assert(err, gc.IsNil)
	reference, err := environs.ComposeUserData(cfg)
	c.Assert(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, reference)
}
