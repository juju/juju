// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

type ProfileSuite struct{}

var _ = gc.Suite(&ProfileSuite{})

func (s *ProfileSuite) TestValidate(c *gc.C) {
	var profileTests = []struct {
		description   string
		profile       *charm.LXDProfile
		expectedError string
	}{
		{
			description: "success",
			profile: &charm.LXDProfile{
				Config: map[string]string{
					"security.nesting":     "true",
					"security.privileged":  "true",
					"linux.kernel_modules": "openvswitch,ip_tables,ip6_tables",
				},
				Description: "success",
				Devices: map[string]map[string]string{
					"tun": {
						"path": "/dev/net/tun",
						"type": "unix-char",
					},
					"sony": {
						"type":      "usb",
						"vendorid":  "0fce",
						"productid": "51da",
					},
					"bdisk": {
						"type":   "unix-block",
						"source": "/dev/loop0",
					},
					"gpu": {
						"type": "gpu",
					},
				},
			},
			expectedError: "",
		}, {
			description: "fail on boot config",
			profile: &charm.LXDProfile{
				Config: map[string]string{
					"security.privileged":  "true",
					"linux.kernel_modules": "openvswitch,ip_tables,ip6_tables",
					"boot.autostart":       "true",
				},
			},
			expectedError: "invalid lxd-profile.yaml: contains config value \"boot.autostart\"",
		}, {
			description: "fail on limits config",
			profile: &charm.LXDProfile{
				Config: map[string]string{
					"security.privileged":  "true",
					"linux.kernel_modules": "openvswitch,ip_tables,ip6_tables",
					"limits.memory":        "256MB",
				},
			},
			expectedError: "invalid lxd-profile.yaml: contains config value \"limits.memory\"",
		}, {
			description: "fail on migration config",
			profile: &charm.LXDProfile{
				Config: map[string]string{
					"security.privileged":          "true",
					"linux.kernel_modules":         "openvswitch,ip_tables,ip6_tables",
					"migration.incremental.memory": "true",
				},
			},
			expectedError: "invalid lxd-profile.yaml: contains config value \"migration.incremental.memory\"",
		}, {
			description: "fail on unix-disk device",
			profile: &charm.LXDProfile{
				Config: map[string]string{
					"security.privileged":  "true",
					"linux.kernel_modules": "openvswitch,ip_tables,ip6_tables",
				},
				Devices: map[string]map[string]string{
					"bdisk": {
						"type":   "unix-disk",
						"source": "/dev/loop0",
					},
				},
			},
			expectedError: "invalid lxd-profile.yaml: contains device type \"unix-disk\"",
		},
	}
	for i, test := range profileTests {
		c.Logf("test %d: %s", i, test.description)
		err := test.profile.ValidateConfigDevices()
		if err != nil {
			c.Assert(err.Error(), gc.Equals, test.expectedError)
		} else {

			c.Assert(err, jc.ErrorIsNil)
		}
	}

}

func (s *ProfileSuite) TestReadLXDProfile(c *gc.C) {
	profile, err := charm.ReadLXDProfile(strings.NewReader(`
config:
  security.nesting: "true"
  security.privileged: "true"
  linux.kernel_modules: openvswitch,nbd,ip_tables,ip6_tables
devices:
  kvm:
    path: /dev/kvm
    type: unix-char
  mem:
    path: /dev/mem
    type: unix-char
  tun:
    path: /dev/net/tun
    type: unix-char
`))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(profile, gc.NotNil)
}

func (s *ProfileSuite) TestLXDProfileEmptyFile(c *gc.C) {
	profile, err := charm.ReadLXDProfile(strings.NewReader(`
 
`))
	c.Assert(profile, gc.DeepEquals, charm.NewLXDProfile())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(profile.Empty(), jc.IsTrue)
	c.Assert(profile.ValidateConfigDevices(), jc.ErrorIsNil)
}

func (s *ProfileSuite) TestReadLXDProfileFailUnmarshall(c *gc.C) {
	profile, err := charm.ReadLXDProfile(strings.NewReader(`
config:
  security.nesting: "true"
  security.privileged: "true"
  linux.kernel_modules: openvswitch,nbd,ip_tables,ip6_tables
 devices:
  kvm:
    path: /dev/kvm
    type: unix-char
  mem:
    path: /dev/mem
    type: unix-char
  tun:
    path: /dev/net/tun
    type: unix-char
`))
	c.Assert(err, gc.ErrorMatches, "failed to unmarshall lxd-profile.yaml: yaml: .*")
	c.Assert(profile, gc.IsNil)
}
