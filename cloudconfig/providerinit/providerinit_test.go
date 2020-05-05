// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package providerinit_test

import (
	"encoding/base64"
	"fmt"
	"path"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
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
	testing.FakeJujuXDGDataHomeSuite
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
		APIInfo:                        &api.Info{Tag: userTag},
		DisableSSLHostnameVerification: false,
		EnableOSRefreshUpdate:          true,
		EnableOSUpgrade:                true,
		CloudInitUserData:              cloudInitUserDataMap,
	}

	cfg, err := config.New(config.NoDefaults, dummySampleConfig().Merge(testing.Attrs{
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
	expectedMcfg.CloudInitUserData = nil
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
		APIInfo: &api.Info{Tag: userTag},
	}
	err = instancecfg.FinishInstanceConfig(icfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg, jc.DeepEquals, &instancecfg.InstanceConfig{
		AuthorizedKeys: "we-are-the-keys",
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
	s.testUserData(c, "quantal", false)
}

func (s *CloudInitSuite) TestControllerUserData(c *gc.C) {
	s.testUserData(c, "quantal", true)
}

func (s *CloudInitSuite) TestControllerUserDataPrecise(c *gc.C) {
	s.testUserData(c, "precise", true)
}

func (*CloudInitSuite) testUserData(c *gc.C, series string, bootstrap bool) {
	// Use actual series paths instead of local defaults
	logDir := must(paths.LogDir(series))
	metricsSpoolDir := must(paths.MetricsSpoolDir(series))
	dataDir := must(paths.DataDir(series))
	toolsList := tools.List{
		&tools.Tools{
			URL:     "http://tools.testing/tools/released/juju.tgz",
			Version: version.Binary{version.MustParse("1.2.3"), "quantal", "amd64"},
		},
	}
	envConfig, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, jc.ErrorIsNil)

	allJobs := []model.MachineJob{
		model.JobManageModel,
		model.JobHostUnits,
	}
	cfg := &instancecfg.InstanceConfig{
		ControllerTag: testing.ControllerTag,
		MachineId:     "10",
		MachineNonce:  "5432",
		Series:        series,
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
		cfg.Controller = &instancecfg.ControllerConfig{
			MongoInfo: &mongo.MongoInfo{
				Info: mongo.Info{
					Addrs:  []string{"127.0.0.1:1234"},
					CACert: "CA CERT\n" + testing.CACert,
				},
				Password: "pw1",
				Tag:      names.NewMachineTag("10"),
			},
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

func (s *CloudInitSuite) TestWindowsUserdataEncoding(c *gc.C) {
	series := "win8"
	metricsSpoolDir := must(paths.MetricsSpoolDir("win8"))
	toolsList := tools.List{
		&tools.Tools{
			URL:     "http://foo.com/tools/released/juju1.2.3-win8-amd64.tgz",
			Version: version.MustParseBinary("1.2.3-win8-amd64"),
			Size:    10,
			SHA256:  "1234",
		},
	}
	dataDir, err := paths.DataDir(series)
	c.Assert(err, jc.ErrorIsNil)
	logDir, err := paths.LogDir(series)
	c.Assert(err, jc.ErrorIsNil)

	cfg := instancecfg.InstanceConfig{
		ControllerTag:    testing.ControllerTag,
		MachineId:        "10",
		AgentEnvironment: map[string]string{agent.ProviderType: "dummy"},
		Series:           series,
		Jobs:             []model.MachineJob{model.JobHostUnits},
		MachineNonce:     "FAKE_NONCE",
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
	err = cfg.SetTools(toolsList)
	c.Assert(err, jc.ErrorIsNil)

	ci, err := cloudinit.New("win8")
	c.Assert(err, jc.ErrorIsNil)

	udata, err := cloudconfig.NewUserdataConfig(&cfg, ci)
	c.Assert(err, jc.ErrorIsNil)

	err = udata.Configure()
	c.Assert(err, jc.ErrorIsNil)

	data, err := ci.RenderYAML()
	c.Assert(err, jc.ErrorIsNil)

	cicompose, err := cloudinit.New("win8")
	c.Assert(err, jc.ErrorIsNil)

	base64Data := base64.StdEncoding.EncodeToString(utils.Gzip(data))
	got := []byte(fmt.Sprintf(cloudconfig.UserDataScript, base64Data))
	expected, err := providerinit.ComposeUserData(&cfg, cicompose, openstack.OpenstackRenderer{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(got), gc.Equals, string(expected))
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
