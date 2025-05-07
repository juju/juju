// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
)

type ProfileSuite struct{}

var _ = tc.Suite(&ProfileSuite{})

func (s *ProfileSuite) TestValidate(c *tc.C) {
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
			c.Assert(err.Error(), tc.Equals, test.expectedError)
		} else {

			c.Assert(err, tc.ErrorIsNil)
		}
	}

}

func (s *ProfileSuite) TestReadLXDProfile(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(profile, tc.NotNil)
}

func (s *ProfileSuite) TestLXDProfileEmptyFile(c *tc.C) {
	profile, err := charm.ReadLXDProfile(strings.NewReader(`
 
`))
	c.Assert(profile, tc.DeepEquals, charm.NewLXDProfile())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(profile.Empty(), tc.IsTrue)
	c.Assert(profile.ValidateConfigDevices(), tc.ErrorIsNil)
}

func (s *ProfileSuite) TestReadLXDProfileFailUnmarshall(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, "failed to unmarshall lxd-profile.yaml: yaml: .*")
	c.Assert(profile, tc.IsNil)
}
