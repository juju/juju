// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cert"
	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
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
	testing.BaseSuite
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

func (s *CloudInitSuite) TestFinishMachineConfigNonDefault(c *gc.C) {
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
		Bootstrap: true,
	}
	cons := constraints.MustParse("mem=1T cpu-power=999999999")
	err = environs.FinishMachineConfig(mcfg, cfg, cons)
	c.Assert(err, gc.IsNil)
	c.Check(mcfg.AuthorizedKeys, gc.Equals, "we-are-the-keys")
	c.Check(mcfg.DisableSSLHostnameVerification, jc.IsFalse)
	password := utils.UserPasswordHash("lisboan-pork", utils.CompatSalt)
	c.Check(mcfg.APIInfo, gc.DeepEquals, &api.Info{
		Password: password, CACert: testing.CACert,
	})
	c.Check(mcfg.StateInfo, gc.DeepEquals, &state.Info{
		Password: password, CACert: testing.CACert,
	})
	c.Check(mcfg.StateServingInfo.StatePort, gc.Equals, cfg.StatePort())
	c.Check(mcfg.StateServingInfo.APIPort, gc.Equals, cfg.APIPort())
	c.Check(mcfg.Constraints, gc.DeepEquals, cons)

	oldAttrs["ca-private-key"] = ""
	oldAttrs["admin-secret"] = ""
	c.Check(mcfg.Config.AllAttrs(), gc.DeepEquals, oldAttrs)
	srvCertPEM := mcfg.StateServingInfo.Cert
	srvKeyPEM := mcfg.StateServingInfo.PrivateKey
	_, _, err = cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Check(err, gc.IsNil)

	err = cert.Verify(srvCertPEM, testing.CACert, time.Now())
	c.Assert(err, gc.IsNil)
	err = cert.Verify(srvCertPEM, testing.CACert, time.Now().AddDate(9, 0, 0))
	c.Assert(err, gc.IsNil)
	err = cert.Verify(srvCertPEM, testing.CACert, time.Now().AddDate(10, 0, 1))
	c.Assert(err, gc.NotNil)
}

func (s *CloudInitSuite) TestUserData(c *gc.C) {
	s.testUserData(c, false)
}

func (s *CloudInitSuite) TestStateServerUserData(c *gc.C) {
	s.testUserData(c, true)
}

func (*CloudInitSuite) testUserData(c *gc.C, bootstrap bool) {
	testJujuHome := c.MkDir()
	defer osenv.SetJujuHome(osenv.SetJujuHome(testJujuHome))
	tools := &tools.Tools{
		URL:     "http://foo.com/tools/releases/juju1.2.3-linux-amd64.tgz",
		Version: version.MustParseBinary("1.2.3-linux-amd64"),
	}
	envConfig, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, gc.IsNil)

	allJobs := []params.MachineJob{
		params.JobManageEnviron,
		params.JobHostUnits,
	}
	cfg := &cloudinit.MachineConfig{
		MachineId:    "10",
		MachineNonce: "5432",
		Tools:        tools,
		StateInfo: &state.Info{
			Addrs:    []string{"127.0.0.1:1234"},
			Password: "pw1",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      "machine-10",
		},
		APIInfo: &api.Info{
			Addrs:    []string{"127.0.0.1:1234"},
			Password: "pw2",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      "machine-10",
		},
		DataDir:                 environs.DataDir,
		LogDir:                  agent.DefaultLogDir,
		Jobs:                    allJobs,
		CloudInitOutputLog:      environs.CloudInitOutputLog,
		Config:                  envConfig,
		AgentEnvironment:        map[string]string{agent.ProviderType: "dummy"},
		AuthorizedKeys:          "wheredidileavemykeys",
		MachineAgentServiceName: "jujud-machine-10",
	}
	if bootstrap {
		cfg.Bootstrap = true
		cfg.StateServingInfo = &params.StateServingInfo{
			StatePort:  envConfig.StatePort(),
			APIPort:    envConfig.APIPort(),
			Cert:       testing.ServerCert,
			PrivateKey: testing.ServerKey,
		}
	}
	script1 := "script1"
	script2 := "script2"
	cloudcfg := coreCloudinit.New()
	cloudcfg.AddRunCmd(script1)
	cloudcfg.AddRunCmd(script2)
	result, err := environs.ComposeUserData(cfg, cloudcfg)
	c.Assert(err, gc.IsNil)

	unzipped, err := utils.Gunzip(result)
	c.Assert(err, gc.IsNil)

	config := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(unzipped, &config)
	c.Assert(err, gc.IsNil)

	// The scripts given to userData where added as the first
	// commands to be run.
	runCmd := config["runcmd"].([]interface{})
	c.Check(runCmd[0], gc.Equals, script1)
	c.Check(runCmd[1], gc.Equals, script2)

	if bootstrap {
		// The cloudinit config should have nothing but the basics:
		// SSH authorized keys, the additional runcmds, and log output.
		//
		// Note: the additional runcmds *do* belong here, at least
		// for MAAS. MAAS needs to configure and then bounce the
		// network interfaces, which would sever the SSH connection
		// in the synchronous bootstrap phase.
		c.Check(config, gc.DeepEquals, map[interface{}]interface{}{
			"output": map[interface{}]interface{}{
				"all": "| tee -a /var/log/cloud-init-output.log",
			},
			"runcmd": []interface{}{
				"script1", "script2",
				"set -xe",
				"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
				"printf '%s\\n' '5432' > '/var/lib/juju/nonce.txt'",
			},
			"ssh_authorized_keys": []interface{}{"wheredidileavemykeys"},
		})
	} else {
		// Just check that the cloudinit config looks good,
		// and that there are more runcmds than the additional
		// ones we passed into ComposeUserData.
		c.Check(config["apt_upgrade"], gc.Equals, true)
		c.Check(len(runCmd) > 2, jc.IsTrue)
	}
}
