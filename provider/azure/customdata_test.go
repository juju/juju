// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"path"

	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type customDataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&customDataSuite{})

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

var logDir = must(paths.LogDir("precise"))
var cloudInitOutputLog = path.Join(logDir, "cloud-init-output.log")

// makeMachineConfig produces a valid cloudinit machine config.
func makeMachineConfig(c *gc.C) *cloudinit.MachineConfig {
	machineId := "0"
	machineTag := names.NewMachineTag(machineId)
	return &cloudinit.MachineConfig{
		MachineId:    machineId,
		MachineNonce: "gxshasqlnng",
		DataDir:      environs.DataDir,
		LogDir:       agent.DefaultLogDir,
		Jobs: []params.MachineJob{
			params.JobManageEnviron,
			params.JobHostUnits,
			params.JobManageNetworking,
		},
		CloudInitOutputLog: cloudInitOutputLog,
		Tools: &tools.Tools{
			Version: version.MustParseBinary("1.2.3-quantal-amd64"),
			URL:     "http://testing.invalid/tools.tar.gz",
		},
		Series: "quantal",
		MongoInfo: &mongo.MongoInfo{
			Info: mongo.Info{
				CACert: testing.CACert,
				Addrs:  []string{"127.0.0.1:123"},
			},
			Tag:      machineTag,
			Password: "password",
		},
		APIInfo: &api.Info{
			CACert: testing.CACert,
			Addrs:  []string{"127.0.0.1:123"},
			Tag:    machineTag,
		},
		MachineAgentServiceName: "jujud-machine-0",
	}
}

// makeBadMachineConfig produces a cloudinit machine config that cloudinit
// will reject as invalid.
func makeBadMachineConfig() *cloudinit.MachineConfig {
	// As it happens, a default-initialized config is invalid.
	return &cloudinit.MachineConfig{Series: "quantal"}
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
	reference, err := environs.ComposeUserData(cfg, nil)
	c.Assert(err, gc.IsNil)
	c.Check(data, gc.DeepEquals, reference)
}
