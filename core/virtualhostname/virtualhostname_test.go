// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package virtualhostname

import (
	"testing"

	"github.com/juju/tc"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type HostnameSuite struct{}

var _ = tc.Suite(&HostnameSuite{})

func (s *HostnameSuite) TestParseHostname(c *tc.C) {
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
				container:       "charm",
				unitNumber:      1,
				applicationName: "postgresql",
				modelUUID:       "8419cd78-4993-4c3a-928e-c646226beeee",
				target:          ContainerTarget,
			},
		},
		{
			desc:     "Unit hostname",
			hostname: "1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			result: Info{
				unitNumber:      1,
				applicationName: "postgresql",
				modelUUID:       "8419cd78-4993-4c3a-928e-c646226beeee",
				target:          UnitTarget,
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
			desc:     "Hostname with dashes in names",
			hostname: "my-charm-container.20.postgresql-k8s.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			result: Info{
				container:       "my-charm-container",
				applicationName: "postgresql-k8s",
				unitNumber:      20,
				modelUUID:       "8419cd78-4993-4c3a-928e-c646226beeee",
				target:          ContainerTarget,
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
			expectedErr: `invalid model UUID: "aaabbb"`,
		},
		{
			desc:        "Invalid application name",
			hostname:    "2.1myapp.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			expectedErr: `invalid application name: "1myapp"`,
		},
		{
			desc:        "Invalid machine number",
			hostname:    "1a.8419cd78-4993-4c3a-928e-c646226beeee.juju.local",
			expectedErr: `could not parse hostname`,
		},
	}
	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		res, err := Parse(tC.hostname)
		if tC.expectedErr == "" {
			c.Assert(err, tc.IsNil)
			c.Assert(res, tc.DeepEquals, tC.result)
			c.Assert(res.String(), tc.Equals, tC.hostname)
		} else {
			c.Assert(err, tc.ErrorMatches, tC.expectedErr)
		}
	}
}

func (s *HostnameSuite) TestNewInfoMachineTarget(c *tc.C) {
	testCases := []struct {
		desc        string
		modelUUID   string
		machine     string
		expected    Info
		expectedErr string
	}{
		{
			desc:      "Valid machine target",
			modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
			machine:   "1",
			expected: Info{
				target:    MachineTarget,
				modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
				machine:   1,
			},
		},
		{
			desc:        "Invalid model UUID",
			modelUUID:   "invalid-uuid",
			machine:     "1",
			expectedErr: ".*invalid model UUID.*",
		},
		{
			desc:        "Invalid machine number",
			modelUUID:   "8419cd78-4993-4c3a-928e-c646226beeee",
			machine:     "-1",
			expectedErr: "invalid machine number: -1",
		},
		{
			desc:        "Invalid machine number: nested container",
			modelUUID:   "8419cd78-4993-4c3a-928e-c646226beeee",
			machine:     "0/lxd/0",
			expectedErr: "container machine not supported",
		},
	}

	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		res, err := NewInfoMachineTarget(tC.modelUUID, tC.machine)
		if tC.expectedErr == "" {
			c.Assert(err, tc.IsNil)
			c.Assert(res, tc.DeepEquals, tC.expected)
		} else {
			c.Assert(err, tc.ErrorMatches, tC.expectedErr)
		}
	}
}

func (s *HostnameSuite) TestNewInfoUnitTarget(c *tc.C) {
	testCases := []struct {
		desc        string
		modelUUID   string
		unit        string
		expected    Info
		expectedErr string
	}{
		{
			desc:      "Valid unit target",
			modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
			unit:      "postgresql/1",
			expected: Info{
				target:          UnitTarget,
				modelUUID:       "8419cd78-4993-4c3a-928e-c646226beeee",
				unitNumber:      1,
				applicationName: "postgresql",
			},
		},
		{
			desc:        "Invalid model UUID",
			modelUUID:   "invalid-uuid",
			unit:        "postgresql/1",
			expectedErr: ".*invalid model UUID.*",
		},
		{
			desc:        "Invalid unit name",
			modelUUID:   "8419cd78-4993-4c3a-928e-c646226beeee",
			unit:        "invalid-unit",
			expectedErr: ".*invalid unit name.*",
		},
	}

	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		res, err := NewInfoUnitTarget(tC.modelUUID, tC.unit)
		if tC.expectedErr == "" {
			c.Assert(err, tc.IsNil)
			c.Assert(res, tc.DeepEquals, tC.expected)
		} else {
			c.Assert(err, tc.ErrorMatches, tC.expectedErr)
		}
	}
}

func (s *HostnameSuite) TestNewInfoContainerTarget(c *tc.C) {
	testCases := []struct {
		desc        string
		modelUUID   string
		unit        string
		container   string
		expected    Info
		expectedErr string
	}{
		{
			desc:      "Valid container target",
			modelUUID: "8419cd78-4993-4c3a-928e-c646226beeee",
			unit:      "postgresql/1",
			container: "charm",
			expected: Info{
				target:          ContainerTarget,
				modelUUID:       "8419cd78-4993-4c3a-928e-c646226beeee",
				unitNumber:      1,
				applicationName: "postgresql",
				container:       "charm",
			},
		},
		{
			desc:        "Invalid model UUID",
			modelUUID:   "invalid-uuid",
			unit:        "postgresql/1",
			container:   "charm",
			expectedErr: ".*invalid model UUID.*",
		},
		{
			desc:        "Invalid unit name",
			modelUUID:   "8419cd78-4993-4c3a-928e-c646226beeee",
			unit:        "invalid-unit",
			container:   "charm",
			expectedErr: ".*invalid unit name.*",
		},
	}

	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		res, err := NewInfoContainerTarget(tC.modelUUID, tC.unit, tC.container)
		if tC.expectedErr == "" {
			if c.Check(err, tc.IsNil) {
				c.Check(res, tc.DeepEquals, tC.expected)
			}
		} else {
			c.Check(err, tc.ErrorMatches, tC.expectedErr)
		}
	}
}

func (s *HostnameSuite) TestnewInfoInvalidTarget(c *tc.C) {
	_, err := newInfo(100, "8419cd78-4993-4c3a-928e-c646226beeee", 1, 1, "1", "charm")
	c.Assert(err, tc.ErrorMatches, "unknown target: 100")
}
