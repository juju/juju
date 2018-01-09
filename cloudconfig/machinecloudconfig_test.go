package cloudconfig_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/juju/errors"
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
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "90_dpkg_local_cloud_config.cfg"), []byte(dpkg_local_cloud_config), 0644)
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "50-curtin-networking.cfg"), []byte(curtin_networking), 0644)
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "10_random.cfg"), []byte(other_config), 0644)
	ioutil.WriteFile(path.Join(s.tempCloudCfgDir, "Readme"), []byte(other_config), 0644)

	// Pre-seed /var/lib/cloud/instance replacement for testing
	s.tempCloudInitDir = c.MkDir()
	ioutil.WriteFile(path.Join(s.tempCloudInitDir, "vendor-data.txt"), []byte(vendor_data), 0644)
}

func (s *fromHostSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&cloudconfig.CloudInitCfgDir, func(_ string) (string, error) { return s.tempCloudCfgDir, nil })
	s.PatchValue(&cloudconfig.MachineCloudInitDir, func(string) (string, error) { return s.tempCloudInitDir, nil })
}

func (s *fromHostSuite) TestGetMachineCloudInitData(c *gc.C) {
	obtained, err := cloudconfig.GetMachineCloudInitData("xenial")
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expectedResult)
}

func (s *fromHostSuite) TestGetMachineCloudInitDataVerifySeries(c *gc.C) {
	for _, series := range []string{"xenial", "opensuseleap", "centos7"} {
		obtained, err := cloudconfig.GetMachineCloudInitData(series)
		c.Assert(err, gc.IsNil)
		c.Assert(obtained, gc.DeepEquals, expectedResult)
	}

	for _, series := range []string{"win2012", "highsierra"} {
		obtained, err := cloudconfig.GetMachineCloudInitData(series)
		c.Assert(err, gc.IsNil)
		c.Assert(obtained, gc.IsNil)
	}
}

func (s *fromHostSuite) TestMissingVendorDataFile(c *gc.C) {
	dir := c.MkDir()
	s.PatchValue(&cloudconfig.MachineCloudInitDir, func(string) (string, error) { return dir, nil })
	obtained, err := cloudconfig.GetMachineData("xenial", "vendor-data.txt")
	c.Assert(err, gc.ErrorMatches, "cannot read \"vendor-data.txt\" from machine .*")
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

var dpkg_local_cloud_config = `
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

var curtin_networking = `
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

var other_config = `
#cloud-config
packages:
  - ‘python-novaclient’
write_files:
  - path: /tmp/juju-test
    permissions: 0644
    content: |
      Hello World!
apt_preserve_sources_list: false
`[1:]

var vendor_data = `
#cloud-config
ntp:
  pools: []
  servers: [10.10.76.2]
`[1:]

var readme_file = `
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
