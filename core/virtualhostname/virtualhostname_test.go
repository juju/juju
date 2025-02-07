// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type HostnameSuite struct{}

var _ = gc.Suite(&HostnameSuite{})

func (s *HostnameSuite) TestParseHostname(c *gc.C) {
	testCases := []struct {
		desc        string
		hostname    string
		result      Info
		expectedErr string
	}{
		{
			desc:     "Container hostname",
			hostname: "charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			result: Info{
				container: "charm",
				unit:      1,
				app:       "postgresql",
				modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
				target:    ContainerTarget,
			},
		},
		{
			desc:     "Unit hostname",
			hostname: "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			result: Info{
				unit:      1,
				app:       "postgresql",
				modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
				target:    UnitTarget,
			},
		},
		{
			desc:     "Machine hostname",
			hostname: "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			result: Info{
				// Machine and unit are both set and disambiguated by the target type.
				machine:   1,
				modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
				target:    MachineTarget,
			},
		},
		{
			desc:     "Hostname with long domain",
			hostname: "1.8419cd78-4993-4c3a-928e-c646226beeee.juju.myfavoritecontroller.local",
			result: Info{
				// Machine and unit are both set and disambiguated by the target type.
				machine:   1,
				modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
				target:    MachineTarget,
			},
		},
		{
			desc:     "Hostname with dashes in names",
			hostname: "my-charm-container.20.postgresql-k8s.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			result: Info{
				container: "my-charm-container",
				unit:      20,
				app:       "postgresql-k8s",
				modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
				target:    ContainerTarget,
			},
		},
		{
			desc:        "Invalid URL too many elements",
			hostname:    "foo.bar.1.1.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			expectedErr: "could not parse hostname",
		},
		{
			desc:        "Invalid model UUID",
			hostname:    "my-charm-container.1.postgresql-k8s.aaabbb.juju.local",
			expectedErr: "invalid model UUID",
		},
		{
			desc:        "Invalid application name",
			hostname:    "2.1myapp.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			expectedErr: "invalid application name",
		},
	}
	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		res, err := Parse(tC.hostname)
		if tC.expectedErr == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(res, gc.DeepEquals, tC.result)
		} else {
			c.Assert(err, gc.ErrorMatches, tC.expectedErr)
		}
	}
}
