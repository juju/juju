package cloudconfig_test

import (
	"os"
	"path"
	"path/filepath"

	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type fromHostSuite struct {
	testing.BaseSuite

	tempCloudCfgDir   string
	tempCloudInitDir  string
	tempCurtinCfgFile string
}

var _ = tc.Suite(&fromHostSuite{})

func (s *fromHostSuite) SetUpTest(c *tc.C) {
	s.PatchValue(&coreos.HostBase, func() (corebase.Base, error) { return corebase.ParseBaseFromString("ubuntu@22.04") })

	// Pre-seed /etc/cloud/cloud.cfg.d replacement for testing
	s.tempCloudCfgDir = c.MkDir() // will clean up
	seedData(c, s.tempCloudCfgDir, "90_dpkg_local_cloud_config.cfg", dpkgLocalCloudConfig)
	seedData(c, s.tempCloudCfgDir, "50-curtin-networking.cfg", curtinNetworking)
	seedData(c, s.tempCloudCfgDir, "10_random.cfg", otherConfig)
	seedData(c, s.tempCloudCfgDir, "Readme", readmeFile)

	// Pre-seed /var/lib/cloud/instance replacement for testing
	s.tempCloudInitDir = c.MkDir()
	seedData(c, s.tempCloudInitDir, "vendor-data.txt", vendorData)

	curtinDir := c.MkDir()
	curtinFile := "curtin-install-cfg.yaml"
	seedData(c, curtinDir, curtinFile, "")
	s.tempCurtinCfgFile = filepath.Join(curtinDir, curtinFile)
}

func (s *fromHostSuite) TestGetMachineCloudInitData(c *tc.C) {
	obtained, err := s.newMachineInitReader(corebase.MakeDefaultBase("ubuntu", "22.04")).GetInitConfig()
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.DeepEquals, expectedResult)
}

type cloudinitDataVerifyTest struct {
	description   string
	machineBase   corebase.Base
	containerBase corebase.Base
	result        map[string]interface{}
}

var cloudinitDataVerifyTests = []cloudinitDataVerifyTest{
	{
		description:   "focal on focal",
		machineBase:   corebase.MakeDefaultBase("ubuntu", "20.04"),
		containerBase: corebase.MakeDefaultBase("ubuntu", "20.04"),
		result:        expectedResult,
	},
	{
		description:   "jammy on jammy",
		machineBase:   corebase.MakeDefaultBase("ubuntu", "22.04"),
		containerBase: corebase.MakeDefaultBase("ubuntu", "22.04"),
		result:        expectedResult,
	},
	{
		description:   "jammy on focal",
		machineBase:   corebase.MakeDefaultBase("ubuntu", "20.04"),
		containerBase: corebase.MakeDefaultBase("ubuntu", "22.04"),
	},
}

func (s *fromHostSuite) TestGetMachineCloudInitDataVerifySeries(c *tc.C) {
	for i, test := range cloudinitDataVerifyTests {
		c.Logf("Test %d of %d: %s", i, len(cloudinitDataVerifyTests), test.description)
		s.PatchValue(&coreos.HostBase, func() (corebase.Base, error) { return test.machineBase, nil })
		obtained, err := s.newMachineInitReader(test.containerBase).GetInitConfig()
		c.Assert(err, tc.IsNil)
		if test.result != nil {
			c.Assert(obtained, tc.DeepEquals, expectedResult)
		} else {
			c.Assert(obtained, tc.IsNil)
		}
	}
}

func (s *fromHostSuite) TestMissingVendorDataFile(c *tc.C) {
	dir := c.MkDir()
	c.Assert(os.RemoveAll(dir), tc.ErrorIsNil)
	s.tempCloudInitDir = dir

	obtained, err := s.newMachineInitReader(corebase.MakeDefaultBase("ubuntu", "22.04")).GetInitConfig()
	c.Assert(err, tc.ErrorMatches, "reading config from.*vendor-data.txt.*")
	c.Assert(obtained, tc.IsNil)
}

func (s *fromHostSuite) TestGetMachineCloudCfgDirDataReadDirFailed(c *tc.C) {
	dir := c.MkDir()
	c.Assert(os.RemoveAll(dir), tc.ErrorIsNil)
	s.tempCloudCfgDir = dir

	obtained, err := s.newMachineInitReader(corebase.MakeDefaultBase("ubuntu", "22.04")).GetInitConfig()
	c.Assert(err, tc.ErrorMatches, "determining files in CloudInitCfgDir for the machine: .* no such file or directory")
	c.Assert(obtained, tc.IsNil)
}

func (s *fromHostSuite) TestCloudConfig(c *tc.C) {
	reader := s.newMachineInitReader(corebase.MakeDefaultBase("ubuntu", "22.04"))
	obtained, err := reader.GetInitConfig()
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.DeepEquals, expectedResult)

	resultMap := reader.ExtractPropertiesFromConfig(
		[]string{"apt-primary", "ca-certs", "apt-security"}, obtained, loggertesting.WrapCheckLog(c))
	c.Assert(resultMap, tc.DeepEquals,
		map[string]interface{}{
			"apt": map[string]interface{}{
				"primary": []interface{}{
					map[interface{}]interface{}{
						"arches": []interface{}{"default"},
						"uri":    "http://archive.ubuntu.com/ubuntu",
					},
				},
				"security": []interface{}{
					map[interface{}]interface{}{
						"arches": []interface{}{"default"},
						"uri":    "http://archive.ubuntu.com/ubuntu",
					},
				},
			},
			"ca-certs": map[interface{}]interface{}{
				"remove-defaults": true,
				"trusted":         []interface{}{"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
			},
		})
}

func (s *fromHostSuite) TestCloudConfigVersionNoContainerInheritProperties(c *tc.C) {
	reader := s.newMachineInitReader(corebase.MakeDefaultBase("ubuntu", "22.04"))
	resultMap := reader.ExtractPropertiesFromConfig(nil, nil, loggertesting.WrapCheckLog(c))
	c.Assert(resultMap, tc.HasLen, 0)
}

func (s *fromHostSuite) TestCurtinConfigAptProperties(c *tc.C) {
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	// Seed the curtin install config as for MAAS 2.5+
	curtinDir := c.MkDir()
	curtinFile := "curtin-install-cfg.yaml"
	seedData(c, curtinDir, curtinFile, curtinConfig)
	s.tempCurtinCfgFile = filepath.Join(curtinDir, curtinFile)

	// Remove the data for prior MAAS versions.
	seedData(c, s.tempCloudCfgDir, "90_dpkg_local_cloud_config.cfg", "")

	expectedSources := `deb http://us.archive.ubuntu.com/ubuntu $RELEASE universe main multiverse restricted
# deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE universe main multiverse restricted
deb http://us.archive.ubuntu.com/ubuntu $RELEASE-updates universe main multiverse restricted
# deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE-updates universe main multiverse restricted
deb http://us.archive.ubuntu.com/ubuntu $RELEASE-security universe main multiverse restricted
# deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE-security universe main multiverse restricted
deb http://us.archive.ubuntu.com/ubuntu $RELEASE-backports universe main multiverse restricted
# deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE-backports universe main multiverse restricted
`
	expected := map[interface{}]interface{}{
		"proxy":                 "http://10-0-0-0--24.maas-internal:8000/",
		"sources_list":          expectedSources,
		"preserve_sources_list": false,
	}

	reader := s.newMachineInitReader(corebase.MakeDefaultBase("ubuntu", "22.04"))
	obtained, err := reader.GetInitConfig()
	c.Assert(err, tc.IsNil)
	c.Assert(obtained["apt"], tc.DeepEquals, expected)

	resultMap := reader.ExtractPropertiesFromConfig(
		[]string{"apt-sources_list"}, obtained, loggertesting.WrapCheckLog(c))

	c.Assert(resultMap["apt"], tc.HasLen, 1)
	c.Assert(resultMap["apt"].(map[string]interface{})["sources_list"], tc.Equals, expectedSources)
}

func (s *fromHostSuite) newMachineInitReader(base corebase.Base) cloudconfig.InitReader {
	cfg := cloudconfig.MachineInitReaderConfig{
		Base:                       base,
		CloudInitConfigDir:         s.tempCloudCfgDir,
		CloudInitInstanceConfigDir: s.tempCloudInitDir,
		CurtinInstallConfigFile:    s.tempCurtinCfgFile,
	}
	return cloudconfig.NewMachineInitReaderFromConfig(cfg)
}

func seedData(c *tc.C, dir, name, data string) {
	c.Assert(os.WriteFile(path.Join(dir, name), []byte(data), 0644), tc.ErrorIsNil)
}

var dpkgLocalCloudConfig = `
# cloud-init/local-cloud-config
apt:
  preserve_sources_list: false
  primary:
  - arches: [default]
    uri: http://archive.ubuntu.com/ubuntu
  security:
  - arches: [default]
    uri: http://archive.ubuntu.com/ubuntu
apt_preserve_sources_list: true
reporting:
  maas: {consumer_key: mpU9YZLWDG7ZQubksN, endpoint: 'http://10.10.101.2/MAAS/metadata/status/cmfcxx',
    token_key: tgEn5v5TcakKwWKwCf, token_secret: jzLdPTuh7hHqHTG9kGEHSG7F25GMAmzJ,
    type: webhook}
system_info:
  package_mirrors:
  - arches: [s390x, amd64]
    failsafe: {primary: 'http://archive.ubuntu.com/ubuntu', security: 'http://security.ubuntu.com/ubuntu'}
    search:
      primary: ['http://archive.ubuntu.com/ubuntu']
      security: ['http://archive.ubuntu.com/ubuntu']
  - arches: [default]
    failsafe: {primary: 'http://ports.ubuntu.com/ubuntu-ports', security: 'http://ports.ubuntu.com/ubuntu-ports'}
    search:
      primary: ['http://ports.ubuntu.com/ubuntu-ports']
      security: ['http://ports.ubuntu.com/ubuntu-ports']
`[1:]

var curtinConfig = `
apply_net_commands:
  builtin: []
apt:
  preserve_sources_list: false
  proxy: http://10-0-0-0--24.maas-internal:8000/
  sources_list: 'deb http://us.archive.ubuntu.com/ubuntu $RELEASE universe main multiverse
    restricted

    # deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE universe main multiverse
    restricted

    deb http://us.archive.ubuntu.com/ubuntu $RELEASE-updates universe main multiverse
    restricted

    # deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE-updates universe main multiverse
    restricted

    deb http://us.archive.ubuntu.com/ubuntu $RELEASE-security universe main multiverse
    restricted

    # deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE-security universe main
    multiverse restricted

    deb http://us.archive.ubuntu.com/ubuntu $RELEASE-backports universe main multiverse
    restricted

    # deb-src http://us.archive.ubuntu.com/ubuntu $RELEASE-backports universe main
    multiverse restricted

    '
`[1:]

var curtinNetworking = `
network:
  config:
  - id: ens3
    mac_address: 52:54:00:0c:xx:e0
    mtu: 1500
    name: ens3
    subnets:
    - address: 10.10.76.124/24
      dns_nameservers:
      - 10.10.76.45
      gateway: 10.10.76.1
      type: static
    type: physical
`[1:]

var otherConfig = `
#cloud-config
packages:
  - ‘python-novaclient’
write_files:
  - path: /tmp/juju-test
    permissions: 0644
    content: |
      Hello World!
apt_preserve_sources_list: false
ca-certs:
  remove-defaults: true
  trusted:
  - |
   -----BEGIN CERTIFICATE-----
   YOUR-ORGS-TRUSTED-CA-CERT-HERE
   -----END CERTIFICATE-----
`[1:]

var vendorData = `
#cloud-config
ntp:
  pools: []
  servers: [10.10.76.2]
`[1:]

var readmeFile = `
# All files in this directory will be read by cloud-init
# They are read in lexical order.  Later files overwrite values in
# earlier files.
`[1:]

var expectedResult = map[string]interface{}{
	"apt": map[interface{}]interface{}{
		"preserve_sources_list": false,
		"primary": []interface{}{
			map[interface{}]interface{}{
				"arches": []interface{}{"default"},
				"uri":    "http://archive.ubuntu.com/ubuntu",
			},
		},
		"security": []interface{}{
			map[interface{}]interface{}{
				"arches": []interface{}{"default"},
				"uri":    "http://archive.ubuntu.com/ubuntu",
			},
		},
	},
	"reporting": map[interface{}]interface{}{
		"maas": map[interface{}]interface{}{
			"endpoint":     "http://10.10.101.2/MAAS/metadata/status/cmfcxx",
			"token_key":    "tgEn5v5TcakKwWKwCf",
			"token_secret": "jzLdPTuh7hHqHTG9kGEHSG7F25GMAmzJ",
			"type":         "webhook",
			"consumer_key": "mpU9YZLWDG7ZQubksN",
		},
	},
	"system_info": expectedSystemInfoCommon,
	"ntp": map[interface{}]interface{}{
		"servers": []interface{}{"10.10.76.2"},
		"pools":   []interface{}{},
	},
	"write_files": []interface{}{
		map[interface{}]interface{}{
			"path":        "/tmp/juju-test",
			"permissions": 420,
			"content":     "Hello World!\n",
		}},
	"apt_preserve_sources_list": true,
	"packages":                  []interface{}{"‘python-novaclient’"},
	"ca-certs": map[interface{}]interface{}{
		"remove-defaults": true,
		"trusted":         []interface{}{"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
	},
	"network": expectedNetworkCommon,
}

var expectedSystemInfoCommon = map[interface{}]interface{}{
	"package_mirrors": []interface{}{map[interface{}]interface{}{
		"arches": []interface{}{"s390x", "amd64"},
		"failsafe": map[interface{}]interface{}{
			"primary":  "http://archive.ubuntu.com/ubuntu",
			"security": "http://security.ubuntu.com/ubuntu",
		},
		"search": map[interface{}]interface{}{
			"primary":  []interface{}{"http://archive.ubuntu.com/ubuntu"},
			"security": []interface{}{"http://archive.ubuntu.com/ubuntu"},
		},
	},
		map[interface{}]interface{}{
			"arches": []interface{}{"default"},
			"failsafe": map[interface{}]interface{}{
				"primary":  "http://ports.ubuntu.com/ubuntu-ports",
				"security": "http://ports.ubuntu.com/ubuntu-ports",
			},
			"search": map[interface{}]interface{}{
				"primary":  []interface{}{"http://ports.ubuntu.com/ubuntu-ports"},
				"security": []interface{}{"http://ports.ubuntu.com/ubuntu-ports"},
			},
		},
	},
}

var expectedNetworkCommon = map[interface{}]interface{}{
	"config": []interface{}{map[interface{}]interface{}{
		"mtu":  1500,
		"name": "ens3",
		"subnets": []interface{}{
			map[interface{}]interface{}{
				"type":            "static",
				"address":         "10.10.76.124/24",
				"dns_nameservers": []interface{}{"10.10.76.45"},
				"gateway":         "10.10.76.1",
			},
		},
		"type":        "physical",
		"id":          "ens3",
		"mac_address": "52:54:00:0c:xx:e0"},
	},
}
