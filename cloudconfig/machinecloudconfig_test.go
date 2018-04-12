package cloudconfig_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	utilsseries "github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/testing"
)

type fromHostSuite struct {
	testing.BaseSuite

	tempCloudCfgDir  string
	tempCloudInitDir string
}

var _ = gc.Suite(&fromHostSuite{})

func (s *fromHostSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	// Pre-seed /etc/cloud/cloud.cfg.d replacement for testing
	s.tempCloudCfgDir = c.MkDir() // will clean up
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "90_dpkg_local_cloud_config.cfg"), []byte(dpkgLocalCloudConfig078), 0644)
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "50-curtin-networking.cfg"), []byte(curtinNetworking), 0644)
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "10_random.cfg"), []byte(otherConfig), 0644)
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "Readme"), []byte(readmeFile), 0644)

	// Pre-seed /var/lib/cloud/instance replacement for testing
	s.tempCloudInitDir = c.MkDir()
	ioutil.WriteFile(path.Join(s.tempCloudInitDir, "vendor-data.txt"), []byte(vendorData), 0644)
}

func (s *fromHostSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&cloudconfig.CloudInitCfgDir, func(_ string) (string, error) { return s.tempCloudCfgDir, nil })
	s.PatchValue(&cloudconfig.MachineCloudInitDir, func(string) (string, error) { return s.tempCloudInitDir, nil })
	s.PatchValue(&utilsseries.MustHostSeries, func() string { return "xenial" })
}

func (s *fromHostSuite) TestGetMachineCloudInitData(c *gc.C) {
	obtained, err := cloudconfig.GetMachineCloudInitData("xenial")
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expectedResult078)
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
		result:          expectedResult078,
	},
	{
		description:     "trusty on trusty",
		machineSeries:   "trusty",
		containerSeries: "trusty",
		result:          expectedResult078,
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
		result:          expectedResult078,
	},
	{
		description:     "centos7 on centos7",
		machineSeries:   "centos7",
		containerSeries: "centos7",
		result:          expectedResult078,
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
		obtained, err := cloudconfig.GetMachineCloudInitData(test.containerSeries)
		c.Assert(err, gc.IsNil)
		if test.result != nil {
			c.Assert(obtained, gc.DeepEquals, expectedResult078)
		} else {
			c.Assert(obtained, gc.IsNil)
		}
	}
}

func (s *fromHostSuite) TestMissingVendorDataFile(c *gc.C) {
	dir := c.MkDir()
	s.PatchValue(&cloudconfig.MachineCloudInitDir, func(string) (string, error) { return dir, nil })
	obtained, err := cloudconfig.GetMachineData("xenial", "vendor-data.txt")
	c.Assert(err, gc.ErrorMatches, "cannot read \"vendor-data.txt\" from machine .*")
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestMissingVendorDataFileTrusty(c *gc.C) {
	ioutil.WriteFile(path.Join(s.tempCloudInitDir, "vendor-data.txt"), []byte(vendorDataTrusty), 0644)
	obtained, err := cloudconfig.GetMachineData("trusty", "vendor-data.txt")
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestGetMachineCloudCfgDirDataReadDirFailed(c *gc.C) {
	dir := c.MkDir()
	os.RemoveAll(dir)
	s.PatchValue(&cloudconfig.CloudInitCfgDir, func(string) (string, error) { return dir, nil })
	obtained, err := cloudconfig.GetMachineCloudCfgDirData("xenial")
	c.Assert(err, gc.ErrorMatches, "cannot determine files in CloudInitCfgDir for the machine: .* no such file or directory")
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestGetMachineCloudCfgDirDataReadDirNotFound(c *gc.C) {
	s.PatchValue(&cloudconfig.CloudInitCfgDir, func(string) (string, error) { return "", errors.New("test failure") })
	obtained, err := cloudconfig.GetMachineCloudCfgDirData("xenial")
	c.Assert(err, gc.ErrorMatches, "cannot determine CloudInitCfgDir for the machine: test failure")
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestGetMachineDataReadDirNotFound(c *gc.C) {
	s.PatchValue(&cloudconfig.MachineCloudInitDir, func(string) (string, error) { return "", errors.New("test failure") })
	obtained, err := cloudconfig.GetMachineData("xenial", "")
	c.Assert(err, gc.ErrorMatches, "cannot determine MachineCloudInitDir for the machine: test failure")
	c.Assert(obtained, gc.IsNil)
}

func (s *fromHostSuite) TestCloudConfigVersionV078(c *gc.C) {
	obtained, err := cloudconfig.GetMachineCloudInitData("xenial")
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expectedResult078)

	resultMap := cloudconfig.CloudConfigByVersionFunc("xenial")("apt-primary,ca-certs,apt-security", obtained,
		loggo.GetLogger("juju.machinecloudconfig"))
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

func (s *fromHostSuite) TestCloudConfigVersionNoContainerInheritPropertiesV078(c *gc.C) {
	resultMap := cloudconfig.CloudConfigByVersionFunc("xenial")("", nil, loggo.GetLogger("juju.machinecloudconfig"))
	c.Assert(resultMap, gc.IsNil)
}

func (s *fromHostSuite) TestCloudConfigVersionV077(c *gc.C) {
	s.PatchValue(&utilsseries.MustHostSeries, func() string { return "trusty" })
	obtained, err := cloudconfig.GetMachineCloudInitData("trusty")
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expectedResult078)

	resultMap := cloudconfig.CloudConfigByVersionFunc("xenial")("apt-primary,ca-certs,apt-security", obtained,
		loggo.GetLogger("juju.machinecloudconfig"))
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

func (s *fromHostSuite) TestCloudConfigVersionNoContainerInheritPropertiesV077(c *gc.C) {
	resultMap := cloudconfig.CloudConfigByVersionFunc("trusty")("", nil, loggo.GetLogger("juju.machinecloudconfig"))
	c.Assert(resultMap, gc.IsNil)
}

var dpkgLocalCloudConfig077 = `
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

var dpkgLocalCloudConfig078 = `
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

var expectedResult078 = map[string]interface{}{
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
	"system_info": map[interface{}]interface{}{
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
	},
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
	"network": map[interface{}]interface{}{
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
	},
}
