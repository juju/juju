// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package providerinit_test

import (
	"path"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

type CloudInitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&CloudInitSuite{})

func (s *CloudInitSuite) TestFinishInstanceConfig(c *gc.C) {

	userTag := names.NewLocalUserTag("not-touched")

	expectedMcfg := &instancecfg.InstanceConfig{
		AgentEnvironment: map[string]string{
			agent.ProviderType:  "dummy",
			agent.ContainerType: "",
		},
		APIInfo:                        &api.Info{Tag: userTag},
		DisableSSLHostnameVerification: false,
		EnableOSRefreshUpdate:          true,
		EnableOSUpgrade:                true,
		CloudInitUserData:              cloudInitUserDataMap,
	}

	cfg, err := config.New(config.NoDefaults, testing.FakeConfig().Merge(testing.Attrs{
		"authorized-keys":    "we-are-the-keys",
		"cloudinit-userdata": validCloudInitUserData,
	}))
	c.Assert(err, jc.ErrorIsNil)

	icfg := &instancecfg.InstanceConfig{
		APIInfo: &api.Info{Tag: userTag},
	}
	err = instancecfg.FinishInstanceConfig(icfg, cfg)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg, jc.DeepEquals, expectedMcfg)

	// Test when updates/upgrades are set to false.
	cfg, err = config.New(config.NoDefaults, testing.FakeConfig().Merge(testing.Attrs{
		"enable-os-refresh-update": false,
		"enable-os-upgrade":        false,
	}))
	c.Assert(err, jc.ErrorIsNil)
	err = instancecfg.FinishInstanceConfig(icfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedMcfg.EnableOSRefreshUpdate = false
	expectedMcfg.EnableOSUpgrade = false
	expectedMcfg.CloudInitUserData = nil
	c.Assert(icfg, jc.DeepEquals, expectedMcfg)
}

func (s *CloudInitSuite) TestFinishInstanceConfigNonDefault(c *gc.C) {
	userTag := names.NewLocalUserTag("not-touched")
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"ssl-hostname-verification": false,
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	icfg := &instancecfg.InstanceConfig{
		APIInfo: &api.Info{Tag: userTag},
	}
	err = instancecfg.FinishInstanceConfig(icfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg, jc.DeepEquals, &instancecfg.InstanceConfig{
		AgentEnvironment: map[string]string{
			agent.ProviderType:  "dummy",
			agent.ContainerType: "",
		},
		APIInfo:                        &api.Info{Tag: userTag},
		DisableSSLHostnameVerification: true,
		EnableOSRefreshUpdate:          true,
		EnableOSUpgrade:                true,
	})
}

func (s *CloudInitSuite) TestUserData(c *gc.C) {
	s.testUserData(c, corebase.MakeDefaultBase("ubuntu", "22.04"), false)
}

func (s *CloudInitSuite) TestControllerUserData(c *gc.C) {
	s.testUserData(c, corebase.MakeDefaultBase("ubuntu", "22.04"), true)
}

func (*CloudInitSuite) testUserData(c *gc.C, base corebase.Base, bootstrap bool) {
	// Use actual series paths instead of local defaults
	logDir := paths.LogDir(paths.OSType(base.OS))
	metricsSpoolDir := paths.MetricsSpoolDir(paths.OSType(base.OS))
	dataDir := paths.DataDir(paths.OSType(base.OS))
	toolsList := tools.List{
		&tools.Tools{
			URL:     "http://tools.testing/tools/released/juju.tgz",
			Version: semversion.Binary{semversion.MustParse("1.2.3"), "jammy", "amd64"},
		},
	}
	envConfig, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)

	allJobs := []model.MachineJob{
		model.JobManageModel,
		model.JobHostUnits,
	}
	cfg := &instancecfg.InstanceConfig{
		ControllerTag: testing.ControllerTag,
		MachineId:     "10",
		MachineNonce:  "5432",
		Base:          base,
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
		AgentEnvironment:        map[string]string{agent.ProviderType: "dummy"},
		AuthorizedKeys:          "wheredidileavemykeys",
		MachineAgentServiceName: "jujud-machine-10",
		EnableOSUpgrade:         true,
		CloudInitUserData:       cloudInitUserDataMap,
	}
	err = cfg.SetTools(toolsList)
	c.Assert(err, jc.ErrorIsNil)
	if bootstrap {
		controllerCfg := testing.FakeControllerConfig()
		cfg.Bootstrap = &instancecfg.BootstrapConfig{
			StateInitializationParams: instancecfg.StateInitializationParams{
				ControllerConfig:      controllerCfg,
				ControllerModelConfig: envConfig,
			},
			StateServingInfo: controller.StateServingInfo{
				StatePort:    controllerCfg.StatePort(),
				APIPort:      controllerCfg.APIPort(),
				Cert:         testing.ServerCert,
				PrivateKey:   testing.ServerKey,
				CAPrivateKey: testing.CAKey,
			},
		}
	}
	script1 := "script1"
	script2 := "script2"
	cloudcfg, err := cloudinit.New(base.OS)
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
			"package_upgrade": false,
			"runcmd": []interface{}{
				"mkdir /tmp/preruncmd",
				"mkdir /tmp/preruncmd2",
				"script1", "script2",
				"set -xe",
				"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
				"echo '5432' > '/var/lib/juju/nonce.txt'",
			},
			"users": []interface{}{
				map[interface{}]interface{}{
					"name":        "ubuntu",
					"lock_passwd": true,
					"groups": []interface{}{"adm", "audio",
						"cdrom", "dialout", "dip",
						"floppy", "netdev", "plugdev",
						"sudo", "video"},
					"shell":               "/bin/bash",
					"sudo":                "ALL=(ALL) NOPASSWD:ALL",
					"ssh_authorized_keys": []interface{}{"wheredidileavemykeys"},
				},
			},
		}
		c.Check(config, jc.DeepEquals, expected)
	} else {
		// Just check that the cloudinit config looks good,
		// and that there are more runcmds than the additional
		// ones we passed into ComposeUserData.
		c.Check(config["package_upgrade"], jc.IsFalse)
		runCmd := config["runcmd"].([]interface{})
		c.Assert(runCmd[:4], gc.DeepEquals, []interface{}{
			`mkdir /tmp/preruncmd`,
			`mkdir /tmp/preruncmd2`,
			script1, script2,
		})
		c.Assert(runCmd[len(runCmd)-2:], gc.DeepEquals, []interface{}{
			`mkdir /tmp/postruncmd`,
			`mkdir /tmp/postruncmd2`,
		})
	}
}

var validCloudInitUserData = `
packages:
  - 'python-keystoneclient'
  - 'python-glanceclient'
preruncmd:
  - mkdir /tmp/preruncmd
  - mkdir /tmp/preruncmd2
postruncmd:
  - mkdir /tmp/postruncmd
  - mkdir /tmp/postruncmd2
package_upgrade: false
`[1:]

var cloudInitUserDataMap = map[string]interface{}{
	"package_upgrade": false,
	"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
	"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
	"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
}
