// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package providerinit_test

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
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
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
// This function also exists in environs/config_test
// Maybe place it in dummy and export it?
func dummySampleConfig() testing.Attrs {
	return dummy.SampleConfig().Merge(testing.Attrs{
		"state-server": false,
	})
}

type CloudInitSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&CloudInitSuite{})

// TODO: add this to the utils package
func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

func (s *CloudInitSuite) TestFinishInstanceConfig(c *gc.C) {

	userTag := names.NewLocalUserTag("not-touched")

	expectedMcfg := &instancecfg.InstanceConfig{
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

	icfg := &instancecfg.InstanceConfig{
		MongoInfo: &mongo.MongoInfo{Tag: userTag},
		APIInfo:   &api.Info{Tag: userTag},
	}
	err = instancecfg.FinishInstanceConfig(icfg, cfg)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg, jc.DeepEquals, expectedMcfg)

	// Test when updates/upgrades are set to false.
	cfg, err = config.New(config.NoDefaults, dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys":          "we-are-the-keys",
		"enable-os-refresh-update": false,
		"enable-os-upgrade":        false,
	}))
	c.Assert(err, jc.ErrorIsNil)
	err = instancecfg.FinishInstanceConfig(icfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedMcfg.EnableOSRefreshUpdate = false
	expectedMcfg.EnableOSUpgrade = false
	c.Assert(icfg, jc.DeepEquals, expectedMcfg)
}

func (s *CloudInitSuite) TestFinishInstanceConfigNonDefault(c *gc.C) {
	userTag := names.NewLocalUserTag("not-touched")
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"authorized-keys":           "we-are-the-keys",
		"ssl-hostname-verification": false,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	icfg := &instancecfg.InstanceConfig{
		MongoInfo: &mongo.MongoInfo{Tag: userTag},
		APIInfo:   &api.Info{Tag: userTag},
	}
	err = instancecfg.FinishInstanceConfig(icfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg, jc.DeepEquals, &instancecfg.InstanceConfig{
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
	icfg := &instancecfg.InstanceConfig{
		Bootstrap: true,
	}
	err = instancecfg.FinishInstanceConfig(icfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(icfg.AuthorizedKeys, gc.Equals, "we-are-the-keys")
	c.Check(icfg.DisableSSLHostnameVerification, jc.IsFalse)
	password := utils.UserPasswordHash("lisboan-pork", utils.CompatSalt)
	c.Check(icfg.APIInfo, gc.DeepEquals, &api.Info{
		Password: password, CACert: testing.CACert,
		EnvironTag: testing.EnvironmentTag,
	})
	c.Check(icfg.MongoInfo, gc.DeepEquals, &mongo.MongoInfo{
		Password: password, Info: mongo.Info{CACert: testing.CACert},
	})
	c.Check(icfg.StateServingInfo.StatePort, gc.Equals, cfg.StatePort())
	c.Check(icfg.StateServingInfo.APIPort, gc.Equals, cfg.APIPort())
	c.Check(icfg.StateServingInfo.CAPrivateKey, gc.Equals, oldAttrs["ca-private-key"])

	oldAttrs["ca-private-key"] = ""
	oldAttrs["admin-secret"] = ""
	c.Check(icfg.Config.AllAttrs(), gc.DeepEquals, oldAttrs)
	srvCertPEM := icfg.StateServingInfo.Cert
	srvKeyPEM := icfg.StateServingInfo.PrivateKey
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
	// Use actual series paths instead of local defaults
	logDir := must(paths.LogDir("quantal"))
	dataDir := must(paths.DataDir("quantal"))
	tools := &tools.Tools{
		URL:     "http://foo.com/tools/released/juju1.2.3-quantal-amd64.tgz",
		Version: version.MustParseBinary("1.2.3-quantal-amd64"),
	}
	envConfig, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, jc.ErrorIsNil)

	series := "quantal"
	allJobs := []multiwatcher.MachineJob{
		multiwatcher.JobManageEnviron,
		multiwatcher.JobHostUnits,
		multiwatcher.JobManageNetworking,
	}
	cfg := &instancecfg.InstanceConfig{
		MachineId:    "10",
		MachineNonce: "5432",
		Tools:        tools,
		Series:       series,
		MongoInfo: &mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{"127.0.0.1:1234"},
				CACert: "CA CERT\n" + testing.CACert,
			},
			Password: "pw1",
			Tag:      names.NewMachineTag("10"),
		},
		APIInfo: &api.Info{
			Addrs:      []string{"127.0.0.1:1234"},
			Password:   "pw2",
			CACert:     "CA CERT\n" + testing.CACert,
			Tag:        names.NewMachineTag("10"),
			EnvironTag: testing.EnvironmentTag,
		},
		DataDir:                 dataDir,
		LogDir:                  path.Join(logDir, "juju"),
		Jobs:                    allJobs,
		CloudInitOutputLog:      path.Join(logDir, "cloud-init-output.log"),
		Config:                  envConfig,
		AgentEnvironment:        map[string]string{agent.ProviderType: "dummy"},
		AuthorizedKeys:          "wheredidileavemykeys",
		MachineAgentServiceName: "jujud-machine-10",
		EnableOSUpgrade:         true,
	}
	if bootstrap {
		cfg.Bootstrap = true
		cfg.StateServingInfo = &params.StateServingInfo{
			StatePort:    envConfig.StatePort(),
			APIPort:      envConfig.APIPort(),
			Cert:         testing.ServerCert,
			PrivateKey:   testing.ServerKey,
			CAPrivateKey: testing.CAKey,
		}
	}
	script1 := "script1"
	script2 := "script2"
	cloudcfg, err := cloudinit.New(series)
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg.AddRunCmd(script1)
	cloudcfg.AddRunCmd(script2)
	result, err := providerinit.ComposeUserData(cfg, cloudcfg)
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
		c.Check(config, jc.DeepEquals, map[interface{}]interface{}{
			"output": map[interface{}]interface{}{
				"all": "| tee -a /var/log/cloud-init-output.log",
			},
			"runcmd": []interface{}{
				"script1", "script2",
				"set -xe",
				"install -D -m 644 /dev/null '/etc/init/juju-clean-shutdown.conf'",
				"printf '%s\\n' '\nauthor \"Juju Team <juju@lists.ubuntu.com>\"\ndescription \"Stop all network interfaces on shutdown\"\nstart on runlevel [016]\ntask\nconsole output\n\nexec /sbin/ifdown -a -v --force\n' > '/etc/init/juju-clean-shutdown.conf'",
				"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
				"printf '%s\\n' '5432' > '/var/lib/juju/nonce.txt'",
			},
			"ssh_authorized_keys": []interface{}{"wheredidileavemykeys"},
		})
	} else {
		// Just check that the cloudinit config looks good,
		// and that there are more runcmds than the additional
		// ones we passed into ComposeUserData.
		c.Check(config["package_upgrade"], jc.IsTrue)
		c.Check(len(runCmd) > 2, jc.IsTrue)
	}
}
