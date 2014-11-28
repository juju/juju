// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"path"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

var logDir = must(paths.LogDir("precise"))
var cloudInitOutputLog = path.Join(logDir, "cloud-init-output.log")

func (s *CloudInitSuite) TestFinishInstanceConfig(c *gc.C) {

	userTag := names.NewLocalUserTag("not-touched")

	expectedMcfg := &cloudinit.MachineConfig{
		AuthorizedKeys: "we-are-the-keys",
		AgentEnvironment: map[string]string{
			agent.ProviderType:  "dummy",
			agent.ContainerType: "",
		},
		MongoInfo: &mongo.MongoInfo{Tag: userTag},
		APIInfo:   &api.Info{Tag: userTag},
		DisableSSLHostnameVerification: false,
		PreferIPv6:                     true,
		EnableOSRefreshUpdate:          true,
		EnableOSUpgrade:                true,
	}

	cfg, err := config.New(config.NoDefaults, dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys": "we-are-the-keys",
	}))
	c.Assert(err, jc.ErrorIsNil)

	mcfg := &cloudinit.MachineConfig{
		MongoInfo: &mongo.MongoInfo{Tag: userTag},
		APIInfo:   &api.Info{Tag: userTag},
	}
	err = environs.FinishMachineConfig(mcfg, cfg)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcfg, jc.DeepEquals, expectedMcfg)

	// Test when updates/upgrades are set to false.
	cfg, err = config.New(config.NoDefaults, dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys":          "we-are-the-keys",
		"enable-os-refresh-update": false,
		"enable-os-upgrade":        false,
	}))
	c.Assert(err, jc.ErrorIsNil)
	err = environs.FinishMachineConfig(mcfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedMcfg.EnableOSRefreshUpdate = false
	expectedMcfg.EnableOSUpgrade = false
	c.Assert(mcfg, jc.DeepEquals, expectedMcfg)
}

func (s *CloudInitSuite) TestFinishMachineConfigNonDefault(c *gc.C) {
	userTag := names.NewLocalUserTag("not-touched")
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys":           "we-are-the-keys",
		"ssl-hostname-verification": false,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	mcfg := &cloudinit.MachineConfig{
		MongoInfo: &mongo.MongoInfo{Tag: userTag},
		APIInfo:   &api.Info{Tag: userTag},
	}
	err = environs.FinishMachineConfig(mcfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcfg, jc.DeepEquals, &cloudinit.MachineConfig{
		AuthorizedKeys: "we-are-the-keys",
		AgentEnvironment: map[string]string{
			agent.ProviderType:  "dummy",
			agent.ContainerType: "",
		},
		MongoInfo: &mongo.MongoInfo{Tag: userTag},
		APIInfo:   &api.Info{Tag: userTag},
		DisableSSLHostnameVerification: true,
		PreferIPv6:                     true,
		EnableOSRefreshUpdate:          true,
		EnableOSUpgrade:                true,
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
	c.Assert(err, jc.ErrorIsNil)
	oldAttrs := cfg.AllAttrs()
	mcfg := &cloudinit.MachineConfig{
		Bootstrap: true,
	}
	err = environs.FinishMachineConfig(mcfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mcfg.AuthorizedKeys, gc.Equals, "we-are-the-keys")
	c.Check(mcfg.DisableSSLHostnameVerification, jc.IsFalse)
	password := utils.UserPasswordHash("lisboan-pork", utils.CompatSalt)
	c.Check(mcfg.APIInfo, gc.DeepEquals, &api.Info{
		Password: password, CACert: testing.CACert,
	})
	c.Check(mcfg.MongoInfo, gc.DeepEquals, &mongo.MongoInfo{
		Password: password, Info: mongo.Info{CACert: testing.CACert},
	})
	c.Check(mcfg.StateServingInfo.StatePort, gc.Equals, cfg.StatePort())
	c.Check(mcfg.StateServingInfo.APIPort, gc.Equals, cfg.APIPort())

	oldAttrs["ca-private-key"] = ""
	oldAttrs["admin-secret"] = ""
	c.Check(mcfg.Config.AllAttrs(), gc.DeepEquals, oldAttrs)
	srvCertPEM := mcfg.StateServingInfo.Cert
	srvKeyPEM := mcfg.StateServingInfo.PrivateKey
	_, _, err = cert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Check(err, jc.ErrorIsNil)

	err = cert.Verify(srvCertPEM, testing.CACert, time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = cert.Verify(srvCertPEM, testing.CACert, time.Now().AddDate(9, 0, 0))
	c.Assert(err, jc.ErrorIsNil)
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
		URL:     "http://foo.com/tools/released/juju1.2.3-quantal-amd64.tgz",
		Version: version.MustParseBinary("1.2.3-quantal-amd64"),
	}
	envConfig, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, jc.ErrorIsNil)

	allJobs := []multiwatcher.MachineJob{
		multiwatcher.JobManageEnviron,
		multiwatcher.JobHostUnits,
		multiwatcher.JobManageNetworking,
	}
	cfg := &cloudinit.MachineConfig{
		MachineId:    "10",
		MachineNonce: "5432",
		Tools:        tools,
		Series:       "quantal",
		MongoInfo: &mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{"127.0.0.1:1234"},
				CACert: "CA CERT\n" + testing.CACert,
			},
			Password: "pw1",
			Tag:      names.NewMachineTag("10"),
		},
		APIInfo: &api.Info{
			Addrs:    []string{"127.0.0.1:1234"},
			Password: "pw2",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      names.NewMachineTag("10"),
		},
		DataDir:                 environs.DataDir,
		LogDir:                  agent.DefaultLogDir,
		Jobs:                    allJobs,
		CloudInitOutputLog:      cloudInitOutputLog,
		Config:                  envConfig,
		AgentEnvironment:        map[string]string{agent.ProviderType: "dummy"},
		AuthorizedKeys:          "wheredidileavemykeys",
		MachineAgentServiceName: "jujud-machine-10",
		EnableOSUpgrade:         true,
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
	c.Assert(err, jc.ErrorIsNil)

	unzipped, err := utils.Gunzip(result)
	c.Assert(err, jc.ErrorIsNil)

	config := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(unzipped, &config)
	c.Assert(err, jc.ErrorIsNil)

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
		c.Check(config["apt_upgrade"], jc.IsTrue)
		c.Check(len(runCmd) > 2, jc.IsTrue)
	}
}
