// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// dummySampleConfig returns the dummy sample config without
// the state server configured.
// will not run a state server.
func dummySampleConfig() testing.Attrs {
	return dummy.SampleConfig().Merge(testing.Attrs{
		"state-server": false,
	})
}

type CloudInitSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&CloudInitSuite{})

func (s *CloudInitSuite) TestFinishInstanceConfig(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys": "we-are-the-keys",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	mcfg := &cloudinit.MachineConfig{
		StateInfo: &state.Info{Tag: "not touched"},
		APIInfo:   &api.Info{Tag: "not touched"},
	}
	err = environs.FinishMachineConfig(mcfg, cfg, constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(mcfg, gc.DeepEquals, &cloudinit.MachineConfig{
		AuthorizedKeys: "we-are-the-keys",
		AgentEnvironment: map[string]string{
			agent.ProviderType:  "dummy",
			agent.ContainerType: "",
		},
		StateInfo: &state.Info{Tag: "not touched"},
		APIInfo:   &api.Info{Tag: "not touched"},
		DisableSSLHostnameVerification: false,
	})
}

func (s *CloudInitSuite) TestFinishMachineConfigNoSSLVerification(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys":           "we-are-the-keys",
		"ssl-hostname-verification": false,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	mcfg := &cloudinit.MachineConfig{
		StateInfo: &state.Info{Tag: "not touched"},
		APIInfo:   &api.Info{Tag: "not touched"},
	}
	err = environs.FinishMachineConfig(mcfg, cfg, constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(mcfg, gc.DeepEquals, &cloudinit.MachineConfig{
		AuthorizedKeys: "we-are-the-keys",
		AgentEnvironment: map[string]string{
			agent.ProviderType:  "dummy",
			agent.ContainerType: "",
		},
		StateInfo: &state.Info{Tag: "not touched"},
		APIInfo:   &api.Info{Tag: "not touched"},
		DisableSSLHostnameVerification: true,
	})
}

func (s *CloudInitSuite) TestFinishBootstrapConfig(c *gc.C) {
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys": "we-are-the-keys",
		"admin-secret":    "lisboan-pork",
		"agent-version":   "1.2.3",
		"state-server":    false,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	oldAttrs := cfg.AllAttrs()
	mcfg := &cloudinit.MachineConfig{
		StateServer: true,
	}
	cons := constraints.MustParse("mem=1T cpu-power=999999999")
	err = environs.FinishMachineConfig(mcfg, cfg, cons)
	c.Assert(err, gc.IsNil)
	c.Check(mcfg.AuthorizedKeys, gc.Equals, "we-are-the-keys")
	c.Check(mcfg.DisableSSLHostnameVerification, jc.IsFalse)
	password := utils.CompatPasswordHash("lisboan-pork")
	c.Check(mcfg.APIInfo, gc.DeepEquals, &api.Info{
		Password: password, CACert: []byte(testing.CACert),
	})
	c.Check(mcfg.StateInfo, gc.DeepEquals, &state.Info{
		Password: password, CACert: []byte(testing.CACert),
	})
	c.Check(mcfg.StatePort, gc.Equals, cfg.StatePort())
	c.Check(mcfg.APIPort, gc.Equals, cfg.APIPort())
	c.Check(mcfg.Constraints, gc.DeepEquals, cons)

	oldAttrs["ca-private-key"] = ""
	oldAttrs["admin-secret"] = ""
	delete(oldAttrs, "secret")
	c.Check(mcfg.Config.AllAttrs(), gc.DeepEquals, oldAttrs)
	srvCertPEM := mcfg.StateServerCert
	srvKeyPEM := mcfg.StateServerKey
	_, _, err = cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Check(err, gc.IsNil)

	err = cert.Verify(srvCertPEM, []byte(testing.CACert), time.Now())
	c.Assert(err, gc.IsNil)
	err = cert.Verify(srvCertPEM, []byte(testing.CACert), time.Now().AddDate(9, 0, 0))
	c.Assert(err, gc.IsNil)
	err = cert.Verify(srvCertPEM, []byte(testing.CACert), time.Now().AddDate(10, 0, 1))
	c.Assert(err, gc.NotNil)
}

func (*CloudInitSuite) TestUserData(c *gc.C) {
	testJujuHome := c.MkDir()
	defer config.SetJujuHome(config.SetJujuHome(testJujuHome))
	tools := &tools.Tools{
		URL:     "http://foo.com/tools/releases/juju1.2.3-linux-amd64.tgz",
		Version: version.MustParseBinary("1.2.3-linux-amd64"),
	}
	envConfig, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, gc.IsNil)

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
		DataDir:          environs.DataDir,
		Config:           envConfig,
		StatePort:        envConfig.StatePort(),
		APIPort:          envConfig.APIPort(),
		StateServer:      true,
		AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
	}
	script1 := "script1"
	script2 := "script2"
	scripts := []string{script1, script2}
	result, err := environs.ComposeUserData(cfg, scripts...)
	c.Assert(err, gc.IsNil)

	unzipped, err := utils.Gunzip(result)
	c.Assert(err, gc.IsNil)

	config := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(unzipped, &config)
	c.Assert(err, gc.IsNil)

	// Just check that the cloudinit config looks good.
	c.Check(config["apt_upgrade"], gc.Equals, true)
	// The scripts given to userData where added as the first
	// commands to be run.
	runCmd := config["runcmd"].([]interface{})
	c.Check(runCmd[0], gc.Equals, script1)
	c.Check(runCmd[1], gc.Equals, script2)
}
