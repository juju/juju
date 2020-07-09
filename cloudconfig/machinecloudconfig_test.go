package cloudconfig_test

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/loggo"
	utilsseries "github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/testing"
)

type fromHostSuite struct {
	testing.BaseSuite

	tempCloudCfgDir   string
	tempCloudInitDir  string
	tempCurtinCfgFile string

	reader *cloudconfig.MachineInitReader
}

var _ = gc.Suite(&fromHostSuite{})

func (s *fromHostSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&utilsseries.MustHostSeries, func() string { return "xenial" })

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

func (s *fromHostSuite) TestGetMachineCloudInitData(c *gc.C) {
	obtained, err := s.newMachineInitReader("xenial").GetInitConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expectedResult)
}

type cloudinitDataVerifyTest struct {
	description     string
	machineSeries   string
	containerSeries string
	err             string
	result          map[string]interface{}
}

var cloudinitDataVerifyTests = []cloudinitDataVerifyTest{
	{
		description:     "xenial on xenial",
		machineSeries:   "xenial",
		containerSeries: "xenial",
		result:          expectedResult,
	},
	{
		description:     "trusty on trusty",
		machineSeries:   "trusty",
		containerSeries: "trusty",
		result:          expectedResult,
	},
	{
		description:     "xenial on trusty",
		machineSeries:   "trusty",
		containerSeries: "xenial",
	},
	{
		description:     "opensuseleap on opensuseleap",
		machineSeries:   "opensuseleap",
		containerSeries: "opensuseleap",
		result:          expectedResult,
	},
	{
		description:     "centos7 on centos7",
		machineSeries:   "centos7",
		containerSeries: "centos7",
		result:          expectedResult,
	},
	{
		description:     "centos8 on centos8",
		machineSeries:   "centos8",
		containerSeries: "centos8",
		result:          expectedResult,
	},
	{
		description:     "win2012 on win2012",
		machineSeries:   "win2012",
		containerSeries: "win2012",
	},
	{
		description:     "highsierra on highsierra",
		machineSeries:   "highsierra",
		containerSeries: "highsierra",
	},
}

func (s *fromHostSuite) TestGetMachineCloudInitDataVerifySeries(c *gc.C) {
	for i, test := range cloudinitDataVerifyTests {
		c.Logf("Test %d of %d: %s", i, len(cloudinitDataVerifyTests), test.description)
		s.PatchValue(&utilsseries.MustHostSeries, func() string { return test.machineSeries })
		obtained, err := s.newMachineInitReader(test.containerSeries).GetInitConfig()
		c.Assert(err, gc.IsNil)
		if test.result != nil {
			c.Assert(obtained, gc.DeepEquals, expectedResult)
		} else {
			c.Assert(obtained, gc.IsNil)
		}
	}
}

func (s *fromHostSuite) TestMissingVendorDataFile(c *gc.C) {
	dir := c.MkDir()
	c.Assert(os.RemoveAll(dir), jc.ErrorIsNil)
	s.tempCloudInitDir = dir

	obtained, err := s.newMachineInitReader("xenial").GetInitConfig()
	c.Assert(err, gc.ErrorMatches, "reading config from.*vendor-data.txt.*")
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestMissingVendorDataFileTrusty(c *gc.C) {
	seedData(c, s.tempCloudInitDir, "vendor-data.txt", vendorDataTrusty)

	obtained, err := s.newMachineInitReader("trusty").GetInitConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestGetMachineCloudCfgDirDataReadDirFailed(c *gc.C) {
	dir := c.MkDir()
	c.Assert(os.RemoveAll(dir), jc.ErrorIsNil)
	s.tempCloudCfgDir = dir

	obtained, err := s.newMachineInitReader("xenial").GetInitConfig()
	c.Assert(err, gc.ErrorMatches, "determining files in CloudInitCfgDir for the machine: .* no such file or directory")
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestCloudConfigVersionV078(c *gc.C) {
	reader := s.newMachineInitReader("xenial")
	obtained, err := reader.GetInitConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expectedResult)

	resultMap := reader.ExtractPropertiesFromConfig(
		[]string{"apt-primary", "ca-certs", "apt-security"}, obtained, loggo.GetLogger("juju.machinecloudconfig"))
	c.Assert(resultMap, gc.DeepEquals,
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

func (s *fromHostSuite) TestCloudConfigVersionNoContainerInheritProperties(c *gc.C) {
	reader := s.newMachineInitReader("xenial")
	resultMap := reader.ExtractPropertiesFromConfig(nil, nil, loggo.GetLogger("juju.machinecloudconfig"))
	c.Assert(resultMap, gc.HasLen, 0)
}

func (s *fromHostSuite) TestCloudConfigVersionV077(c *gc.C) {
	s.PatchValue(&utilsseries.MustHostSeries, func() string { return "trusty" })
	seedData(c, s.tempCloudCfgDir, "90_dpkg_local_cloud_config.cfg", dpkgLocalCloudConfigLegacy)

	reader := s.newMachineInitReader("trusty")
	obtained, err := reader.GetInitConfig()
	c.Assert(err, gc.IsNil)

	resultMap := reader.ExtractPropertiesFromConfig(
		[]string{"apt-primary", "ca-certs", "apt-security"}, obtained, loggo.GetLogger("juju.machinecloudconfig"))

	// Can't compare map-to-map equality directly due to one of the values
	// being a slice - it fails intermittently.
	c.Assert(resultMap, gc.HasLen, 5)
	c.Assert(resultMap["apt_mirror"], gc.Equals, expectedResultLegacy["apt_mirror"])
	c.Assert(resultMap["apt_mirror_search_dns"], gc.Equals, expectedResultLegacy["apt_mirror_search_dns"])
	c.Assert(resultMap["apt_mirror_search"], jc.SameContents, expectedResultLegacy["apt_mirror_search"])
	c.Assert(resultMap["apt_sources"], gc.DeepEquals, expectedResultLegacy["apt_sources"])
}

func (s *fromHostSuite) TestCloudConfigVersionNoContainerInheritPropertiesLegacy(c *gc.C) {
	reader := s.newMachineInitReader("trusty")
	resultMap := reader.ExtractPropertiesFromConfig(nil, nil, loggo.GetLogger("juju.machinecloudconfig"))
	c.Assert(resultMap, gc.HasLen, 0)
}

func (s *fromHostSuite) TestCurtinConfigAptProperties(c *gc.C) {
	s.PatchValue(&utilsseries.MustHostSeries, func() string { return "bionic" })

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

	reader := s.newMachineInitReader("bionic")
	obtained, err := reader.GetInitConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained["apt"], gc.DeepEquals, expected)

	resultMap := reader.ExtractPropertiesFromConfig(
		[]string{"apt-sources_list"}, obtained, loggo.GetLogger("juju.machinecloudconfig"))

	c.Assert(resultMap["apt"], gc.HasLen, 1)
	c.Assert(resultMap["apt"].(map[string]interface{})["sources_list"], gc.Equals, expectedSources)
}

func (s *fromHostSuite) newMachineInitReader(series string) cloudconfig.InitReader {
	cfg := cloudconfig.MachineInitReaderConfig{
		Series:                     series,
		CloudInitConfigDir:         s.tempCloudCfgDir,
		CloudInitInstanceConfigDir: s.tempCloudInitDir,
		CurtinInstallConfigFile:    s.tempCurtinCfgFile,
	}
	return cloudconfig.NewMachineInitReaderFromConfig(cfg)
}

func seedData(c *gc.C, dir, name, data string) {
	c.Assert(ioutil.WriteFile(path.Join(dir, name), []byte(data), 0644), jc.ErrorIsNil)
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
  - arches: [i386, amd64]
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

var dpkgLocalCloudConfigLegacy = `
# cloud-init/local-cloud-config
apt_mirror: http://archive.ubuntu.com/ubuntu
apt_mirror_search:
 - http://local-mirror.mydomain
 - http://archive.ubuntu.com
apt_mirror_search_dns: False
apt_sources:
 - source: "deb http://apt.opscode.com/ $RELEASE-0.10 main"
   key: |
     -----BEGIN PGP PUBLIC KEY BLOCK-----
     Version: GnuPG v1.4.9 (GNU/Linux)
     -----END PGP PUBLIC KEY BLOCK-----
apt_preserve_sources_list: true
reporting:
  maas: {consumer_key: mpU9YZLWDG7ZQubksN, endpoint: 'http://10.10.101.2/MAAS/metadata/status/cmfcxx',
    token_key: tgEn5v5TcakKwWKwCf, token_secret: jzLdPTuh7hHqHTG9kGEHSG7F25GMAmzJ,
    type: webhook}
system_info:
  package_mirrors:
  - arches: [i386, amd64]
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

var vendorDataTrusty = `
None
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

var expectedResultLegacy = map[string]interface{}{
	"apt_mirror": "http://archive.ubuntu.com/ubuntu",
	"apt_mirror_search": []interface{}{
		"http://local-mirror.mydomain",
		"http://archive.ubuntu.com",
	},
	"apt_mirror_search_dns": false,
	"apt_sources": []interface{}{
		map[interface{}]interface{}{
			"source": "deb http://apt.opscode.com/ $RELEASE-0.10 main",
			"key":    "-----BEGIN PGP PUBLIC KEY BLOCK-----\nVersion: GnuPG v1.4.9 (GNU/Linux)\n-----END PGP PUBLIC KEY BLOCK-----\n",
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
		"arches": []interface{}{"i386", "amd64"},
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
