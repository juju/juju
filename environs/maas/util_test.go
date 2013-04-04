package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
)

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestExtractSystemId(c *C) {
	instanceId := state.InstanceId("/MAAS/api/1.0/nodes/system_id/")

	systemId := extractSystemId(instanceId)

	c.Check(systemId, Equals, "system_id")
}

func (s *UtilSuite) TestGetSystemIdValues(c *C) {
	instanceId1 := state.InstanceId("/MAAS/api/1.0/nodes/system_id1/")
	instanceId2 := state.InstanceId("/MAAS/api/1.0/nodes/system_id2/")
	instanceIds := []state.InstanceId{instanceId1, instanceId2}

	values := getSystemIdValues(instanceIds)

	c.Check(values["id"], DeepEquals, []string{"system_id1", "system_id2"})
}

func (s *UtilSuite) TestUserData(c *C) {

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
		DataDir:     jujuDataDir,
		MongoPort:   mgoPort,
		Config:      envConfig,
		APIPort:     apiPort,
		StateServer: true,
	}
	script1 := "script1"
	script2 := "script2"
	scripts := []string{script1, script2}
	result, err := userData(cfg, scripts...)
	c.Assert(err, IsNil)

	unzipped, err := trivial.Gunzip(result)
	c.Assert(err, IsNil)

	config := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(unzipped, &config)
	c.Assert(err, IsNil)

	// Just check that the cloudinit config looks good.
	c.Check(config["apt_upgrade"], Equals, true)
	// The scripts given to userData where added as the first
	// commands to be run.
	runCmd := config["runcmd"].([]interface{})
	c.Check(runCmd[0], Equals, script2)
	c.Check(runCmd[1], Equals, script1)
}
