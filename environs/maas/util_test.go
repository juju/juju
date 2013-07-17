// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestExtractSystemId(c *C) {
	instanceId := instance.Id("/MAAS/api/1.0/nodes/system_id/")

	systemId := extractSystemId(instanceId)

	c.Check(systemId, Equals, "system_id")
}

func (s *UtilSuite) TestGetSystemIdValues(c *C) {
	instanceId1 := instance.Id("/MAAS/api/1.0/nodes/system_id1/")
	instanceId2 := instance.Id("/MAAS/api/1.0/nodes/system_id2/")
	instanceIds := []instance.Id{instanceId1, instanceId2}

	values := getSystemIdValues(instanceIds)

	c.Check(values["id"], DeepEquals, []string{"system_id1", "system_id2"})
}

func (s *UtilSuite) TestUserData(c *C) {
	testJujuHome := c.MkDir()
	defer config.SetJujuHome(config.SetJujuHome(testJujuHome))
	tools := &state.Tools{
		URL:    "http://foo.com/tools/juju1.2.3-linux-amd64.tgz",
		Binary: version.MustParseBinary("1.2.3-linux-amd64"),
	}
	envConfig, err := config.New(map[string]interface{}{
		"type":            "maas",
		"name":            "foo",
		"default-series":  "series",
		"authorized-keys": "keys",
		"ca-cert":         testing.CACert,
	})
	c.Assert(err, IsNil)

	cfg := &cloudinit.MachineConfig{
		MachineId:       "10",
		MachineNonce:    "5432",
		Tools:           tools,
		StateServerCert: []byte(testing.ServerCert),
		StateServerKey:  []byte(testing.ServerKey),
		StateInfo: &state.Info{
			Password: "pw1",
			CACert:   []byte("CA CERT\n" + testing.CACert),
		},
		APIInfo: &api.Info{
			Password: "pw2",
			CACert:   []byte("CA CERT\n" + testing.CACert),
		},
		DataDir:     environs.DataDir,
		Config:      envConfig,
		StatePort:   envConfig.StatePort(),
		APIPort:     envConfig.APIPort(),
		StateServer: true,
	}
	script1 := "script1"
	script2 := "script2"
	scripts := []string{script1, script2}
	result, err := userData(cfg, scripts...)
	c.Assert(err, IsNil)

	unzipped, err := utils.Gunzip(result)
	c.Assert(err, IsNil)

	config := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(unzipped, &config)
	c.Assert(err, IsNil)

	// Just check that the cloudinit config looks good.
	c.Check(config["apt_upgrade"], Equals, true)
	// The scripts given to userData where added as the first
	// commands to be run.
	runCmd := config["runcmd"].([]interface{})
	c.Check(runCmd[0], Equals, script1)
	c.Check(runCmd[1], Equals, script2)
}

func (s *UtilSuite) TestMachineInfoCloudinitRunCmd(c *C) {
	hostname := "hostname"
	filename := "path/to/file"
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()
	info := machineInfo{hostname}

	script, err := info.cloudinitRunCmd()

	c.Assert(err, IsNil)
	yaml, err := goyaml.Marshal(info)
	c.Assert(err, IsNil)
	expected := fmt.Sprintf("mkdir -p '%s'; echo -n '%s' > '%s'", environs.DataDir, yaml, filename)
	c.Check(script, Equals, expected)
}

func (s *UtilSuite) TestMachineInfoLoad(c *C) {
	hostname := "hostname"
	yaml := fmt.Sprintf("hostname: %s\n", hostname)
	filename := createTempFile(c, []byte(yaml))
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()
	info := machineInfo{}

	err := info.load()

	c.Assert(err, IsNil)
	c.Check(info.Hostname, Equals, hostname)
}
