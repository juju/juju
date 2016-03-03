// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package providerinit_test

import (
	"encoding/base64"
	"fmt"
	"path"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// dummySampleConfig returns the dummy sample config without
// the controller configured.
// This function also exists in environs/config_test
// Maybe place it in dummy and export it?
func dummySampleConfig() testing.Attrs {
	return dummy.SampleConfig().Merge(testing.Attrs{
		"controller": false,
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
		"controller":      false,
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
		ModelTag: testing.ModelTag,
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
	s.testUserData(c, "quantal", false)
}

func (s *CloudInitSuite) TestControllerUserData(c *gc.C) {
	s.testUserData(c, "quantal", true)
}

func (s *CloudInitSuite) TestControllerUserDataPrecise(c *gc.C) {
	s.testUserData(c, "precise", true)
}

func (*CloudInitSuite) testUserData(c *gc.C, series string, bootstrap bool) {
	testJujuXDGDataHome := c.MkDir()
	defer osenv.SetJujuXDGDataHome(osenv.SetJujuXDGDataHome(testJujuXDGDataHome))
	// Use actual series paths instead of local defaults
	logDir := must(paths.LogDir(series))
	metricsSpoolDir := must(paths.MetricsSpoolDir(series))
	dataDir := must(paths.DataDir(series))
	tools := &tools.Tools{
		URL:     "http://tools.testing/tools/released/juju.tgz",
		Version: version.Binary{version.MustParse("1.2.3"), "quantal", "amd64"},
	}
	envConfig, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, jc.ErrorIsNil)

	allJobs := []multiwatcher.MachineJob{
		multiwatcher.JobManageModel,
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
			Addrs:    []string{"127.0.0.1:1234"},
			Password: "pw2",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      names.NewMachineTag("10"),
			ModelTag: testing.ModelTag,
		},
		DataDir:                 dataDir,
		LogDir:                  path.Join(logDir, "juju"),
		MetricsSpoolDir:         metricsSpoolDir,
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
	result, err := providerinit.ComposeUserData(cfg, cloudcfg, &openstack.OpenstackRenderer{})
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
		expected := map[interface{}]interface{}{
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
		}
		// Series with old cloudinit versions don't support adding
		// users so need the old way to set SSH authorized keys.
		if series == "precise" {
			expected["ssh_authorized_keys"] = []interface{}{
				"wheredidileavemykeys",
			}
		} else {
			expected["users"] = []interface{}{
				map[interface{}]interface{}{
					"name":        "ubuntu",
					"lock_passwd": true,
					"groups": []interface{}{"adm", "audio",
						"cdrom", "dialout", "dip",
						"floppy", "netdev", "plugdev",
						"sudo", "video"},
					"shell":               "/bin/bash",
					"sudo":                []interface{}{"ALL=(ALL) NOPASSWD:ALL"},
					"ssh-authorized-keys": []interface{}{"wheredidileavemykeys"},
				},
			}
		}
		c.Check(config, jc.DeepEquals, expected)
	} else {
		// Just check that the cloudinit config looks good,
		// and that there are more runcmds than the additional
		// ones we passed into ComposeUserData.
		c.Check(config["package_upgrade"], jc.IsTrue)
		c.Check(len(runCmd) > 2, jc.IsTrue)
	}
}

func (s *CloudInitSuite) TestWindowsUserdataEncoding(c *gc.C) {
	series := "win8"
	metricsSpoolDir := must(paths.MetricsSpoolDir("win8"))
	tools := &tools.Tools{
		URL:     "http://foo.com/tools/released/juju1.2.3-win8-amd64.tgz",
		Version: version.MustParseBinary("1.2.3-win8-amd64"),
		Size:    10,
		SHA256:  "1234",
	}
	dataDir, err := paths.DataDir(series)
	c.Assert(err, jc.ErrorIsNil)
	logDir, err := paths.LogDir(series)
	c.Assert(err, jc.ErrorIsNil)

	cfg := instancecfg.InstanceConfig{
		MachineId:        "10",
		AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
		Tools:            tools,
		Series:           series,
		Bootstrap:        false,
		Jobs:             []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		MachineNonce:     "FAKE_NONCE",
		MongoInfo: &mongo.MongoInfo{
			Tag:      names.NewMachineTag("10"),
			Password: "arble",
			Info: mongo.Info{
				CACert: "CA CERT\n" + testing.CACert,
				Addrs:  []string{"state-addr.testing.invalid:12345"},
			},
		},
		APIInfo: &api.Info{
			Addrs:    []string{"state-addr.testing.invalid:54321"},
			Password: "bletch",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      names.NewMachineTag("10"),
			ModelTag: testing.ModelTag,
		},
		MachineAgentServiceName: "jujud-machine-10",
		DataDir:                 dataDir,
		LogDir:                  path.Join(logDir, "juju"),
		MetricsSpoolDir:         metricsSpoolDir,
		CloudInitOutputLog:      path.Join(logDir, "cloud-init-output.log"),
	}

	ci, err := cloudinit.New("win8")
	c.Assert(err, jc.ErrorIsNil)

	udata, err := cloudconfig.NewUserdataConfig(&cfg, ci)
	c.Assert(err, jc.ErrorIsNil)
	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)
	data, err := ci.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)
	base64Data := base64.StdEncoding.EncodeToString(utils.Gzip(data))
	got := []byte(fmt.Sprintf(cloudconfig.UserdataScript, base64Data))

	cicompose, err := cloudinit.New("win8")
	c.Assert(err, jc.ErrorIsNil)
	expected, err := providerinit.ComposeUserData(&cfg, cicompose, openstack.OpenstackRenderer{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(expected), gc.Equals, string(got))
}
