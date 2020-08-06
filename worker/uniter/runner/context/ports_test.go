// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/names/v4"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

var _ = gc.Suite(&PortRangeChangeRecorderSuite{})

type PortRangeChangeRecorderSuite struct {
	envtesting.IsolationSuite
}

type portRangeTest struct {
	about string

	targetEndpoint  string
	targetPortRange network.PortRange

	machinePortRanges  map[names.UnitTag]map[string][]network.PortRange
	pendingOpenRanges  map[string][]network.PortRange
	pendingCloseRanges map[string][]network.PortRange

	expectErr          string
	expectPendingOpen  map[string][]network.PortRange
	expectPendingClose map[string][]network.PortRange
}

func (s *PortRangeChangeRecorderSuite) TestOpenPortRange(c *gc.C) {
	targetUnit := names.NewUnitTag("u/0")

	tests := []portRangeTest{
		{
			about:           "open a new range - all endpoints (no machine ports yet)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			expectPendingOpen: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "open an existing range - all endpoints (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
		},
		{
			about:           "open an existing range - same unit; different endpoint (accepted)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingOpen: map[string][]network.PortRange{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "open a range pending to be closed",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingCloseRanges: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingClose: map[string][]network.PortRange{
				"": []network.PortRange{},
			},
		},
		{
			about:           "open a range pending to be opened already - same unit; same endpoint (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingOpenRanges: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "open a range conflicting with another unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("other/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot open 10-20/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "other/0"\)`,
		},
		{
			about:           "open a range conflicting with the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot open 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\)`,
		},
		{
			about:           "open a range conflicting with a pending range for the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			pendingOpenRanges: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectErr: `cannot open 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\) requested earlier`,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		rec := newPortRangeChangeRecorder(targetUnit, test.machinePortRanges)
		rec.pendingOpenRanges = test.pendingOpenRanges
		rec.pendingCloseRanges = test.pendingCloseRanges

		err := rec.OpenPortRange(test.targetEndpoint, test.targetPortRange)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)

			pendingOpenRanges, pendingCloseRanges := rec.PendingChanges()
			c.Check(pendingOpenRanges, jc.DeepEquals, test.expectPendingOpen)
			c.Check(pendingCloseRanges, jc.DeepEquals, test.expectPendingClose)
		}
	}
}

func (s *PortRangeChangeRecorderSuite) TestClosePortRange(c *gc.C) {
	targetUnit := names.NewUnitTag("u/0")

	tests := []portRangeTest{
		{
			about:           "close a new range (no machine ports yet; ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
		},
		{
			about:           "close an existing range - all endpoints",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close an existing range - same unit; different endpoint (accepted even if not opened for that endpoint)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: map[string][]network.PortRange{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close a range pending to be opened (removed from pending open)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingOpenRanges: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: map[string][]network.PortRange{
				"": []network.PortRange{},
			},
			expectPendingClose: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close a range pending to be closed (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingCloseRanges: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingClose: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close a range opened by another unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("other/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot close 10-20/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "other/0"\)`,
		},
		{
			about:           "close a range conflicting with the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot close 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\)`,
		},
		{
			about:           "close a range conflicting with a pending range for the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			machinePortRanges: map[names.UnitTag]map[string][]network.PortRange{
				names.NewUnitTag("u/0"): map[string][]network.PortRange{
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingCloseRanges: map[string][]network.PortRange{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectErr: `cannot close 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\) requested earlier`,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		rec := newPortRangeChangeRecorder(targetUnit, test.machinePortRanges)
		rec.pendingOpenRanges = test.pendingOpenRanges
		rec.pendingCloseRanges = test.pendingCloseRanges

		err := rec.ClosePortRange(test.targetEndpoint, test.targetPortRange)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)

			pendingOpenRanges, pendingCloseRanges := rec.PendingChanges()
			c.Check(pendingOpenRanges, jc.DeepEquals, test.expectPendingOpen)
			c.Check(pendingCloseRanges, jc.DeepEquals, test.expectPendingClose)
		}
	}
}
