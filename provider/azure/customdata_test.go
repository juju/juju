// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"path"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/multiwatcher"
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
var dataDir = must(paths.DataDir("precise"))
var cloudInitOutputLog = path.Join(logDir, "cloud-init-output.log")

// makeInstanceConfig produces a valid cloudinit machine config.
func makeInstanceConfig(c *gc.C) *instancecfg.InstanceConfig {
	machineId := "0"
	machineTag := names.NewMachineTag(machineId)
	return &instancecfg.InstanceConfig{
		MachineId:    machineId,
		MachineNonce: "gxshasqlnng",
		DataDir:      dataDir,
		LogDir:       logDir,
		Jobs: []multiwatcher.MachineJob{
			multiwatcher.JobManageEnviron,
			multiwatcher.JobHostUnits,
			multiwatcher.JobManageNetworking,
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
			CACert:     testing.CACert,
			Addrs:      []string{"127.0.0.1:123"},
			Tag:        machineTag,
			EnvironTag: testing.EnvironmentTag,
		},
		MachineAgentServiceName: "jujud-machine-0",
	}
}

// makeBadInstanceConfig produces a cloudinit machine config that cloudinit
// will reject as invalid.
func makeBadInstanceConfig() *instancecfg.InstanceConfig {
	// As it happens, a default-initialized config is invalid.
	return &instancecfg.InstanceConfig{Series: "quantal"}
}

func (*customDataSuite) TestMakeCustomDataPropagatesError(c *gc.C) {
	_, err := makeCustomData(makeBadInstanceConfig())
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "failure while generating custom data: invalid machine configuration: invalid machine id")
}

func (*customDataSuite) TestMakeCustomDataEncodesUserData(c *gc.C) {
	cfg := makeInstanceConfig(c)

	encodedData, err := makeCustomData(cfg)
	c.Assert(err, jc.ErrorIsNil)

	data, err := base64.StdEncoding.DecodeString(encodedData)
	c.Assert(err, jc.ErrorIsNil)
	reference, err := providerinit.ComposeUserData(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, gc.DeepEquals, reference)
}
