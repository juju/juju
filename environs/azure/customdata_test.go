// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type CustomDataSuite struct{}

var _ = gc.Suite(&CustomDataSuite{})

// makeMachineConfig produces a valid cloudinit machine config.
func makeMachineConfig(c *gc.C) *cloudinit.MachineConfig {
	dir := c.MkDir()
	machineID := "0"
	return &cloudinit.MachineConfig{
		MachineId:    machineID,
		MachineNonce: "gxshasqlnng",
		DataDir:      dir,
		Tools:        &state.Tools{URL: "file://" + dir},
		StateInfo: &state.Info{
			CACert: []byte(testing.CACert),
			Addrs:  []string{"127.0.0.1:123"},
			Tag:    state.MachineTag(machineID),
		},
		APIInfo: &api.Info{
			CACert: []byte(testing.CACert),
			Addrs:  []string{"127.0.0.1:123"},
			Tag:    state.MachineTag(machineID),
		},
	}
}

// makeBadMachineConfig produces a cloudinit machine config that cloudinit
// will reject as invalid.
func makeBadMachineConfig() *cloudinit.MachineConfig {
	// As it happens, a default-initialized config is invalid.
	return &cloudinit.MachineConfig{}
}

func (*CustomDataSuite) TestUserDataFailsOnBadData(c *gc.C) {
	_, err := userData(makeBadMachineConfig())
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "problem with cloudinit config: .*")
}

func (*CustomDataSuite) TestUserDataProducesZipFile(c *gc.C) {
	marker := "recognizable_text"
	cfg := makeMachineConfig(c)
	cfg.MachineNonce = marker

	data, err := userData(cfg)
	c.Assert(err, gc.IsNil)

	content, err := utils.Gunzip(data)
	c.Assert(err, gc.IsNil)
	// The regex syntax is weird here.  It's a trick to work around
	// gocheck's matcher prefixing "^" to the regex and suffixing "$".
	// With this trick, we can scan for a pattern in a multi-line input.
	c.Check(string(content), gc.Matches, "(?s).*"+marker+".*")
}

func (*CustomDataSuite) TestMakeCustomDataPropagatesError(c *gc.C) {
	_, err := makeCustomData(makeBadMachineConfig())
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "failure while generating custom data: problem with cloudinit config: .*")
}

func (*CustomDataSuite) TestMakeCustomDataEncodesUserData(c *gc.C) {
	cfg := makeMachineConfig(c)

	encodedData, err := makeCustomData(cfg)
	c.Assert(err, gc.IsNil)

	data, err := base64.StdEncoding.DecodeString(encodedData)
	c.Assert(err, gc.IsNil)
	reference, err := userData(cfg)
	c.Assert(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, reference)
}
