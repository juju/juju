// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	envtesting "github.com/juju/testing"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var _ = tc.Suite(&PortRangeChangeRecorderSuite{})

type PortRangeChangeRecorderSuite struct {
	envtesting.IsolationSuite
}

type portRangeTest struct {
	about string

	targetEndpoint  string
	targetPortRange network.PortRange

	machinePortRanges, applicationPortRanges map[names.UnitTag]network.GroupedPortRanges
	pendingOpenRanges                        network.GroupedPortRanges
	pendingCloseRanges                       network.GroupedPortRanges

	expectErr          string
	expectPendingOpen  network.GroupedPortRanges
	expectPendingClose network.GroupedPortRanges
	isCAAS             bool
}

func (s *PortRangeChangeRecorderSuite) TestOpenPortRange(c *tc.C) {
	targetUnit := names.NewUnitTag("u/0")

	tests := []portRangeTest{
		{
			about:           "open a new range - all endpoints (no machine ports yet)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			expectPendingOpen: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "open an existing range - all endpoints (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
		},
		{
			about:           "open an existing range - same unit; different endpoint (accepted)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "open an existing range with pending close port",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingCloseRanges: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: network.GroupedPortRanges{
				"foo": []network.PortRange{},
			},
		},
		{
			about:           "sidecar unit: open an existing range - all endpoints (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
		},
		{
			about:           "sidecar unit: open an existing range - same unit; different endpoint (accepted)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "sidecar unit: open an existing range with pending close port",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingCloseRanges: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: network.GroupedPortRanges{
				"foo": []network.PortRange{},
			},
		},
		{
			about:           "open a range pending to be closed",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingCloseRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{},
			},
		},
		{
			about:           "open a range pending to be opened already - same unit; same endpoint (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingOpenRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "open a range conflicting with another unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("other/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot open 10-20/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "other/0"\)`,
		},
		{
			about:           "open a range conflicting with the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot open 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\)`,
		},
		{
			about:           "open a range conflicting with a pending range for the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			pendingOpenRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectErr: `cannot open 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\) requested earlier`,
		},
		{
			about:           "port range is not supported for CAAS",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			isCAAS:          true,
			expectErr:       `port ranges are not supported for k8s applications, please specify a single port`,
		},
		{
			about:           "open a new range - all endpoints - CAAS",
			targetEndpoint:  "",
			isCAAS:          true,
			targetPortRange: network.MustParsePortRange("80/tcp"),
			expectPendingOpen: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("80/tcp")},
			},
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		modelType := model.IAAS
		if test.isCAAS {
			modelType = model.CAAS
		}
		rec := newPortRangeChangeRecorder(loggertesting.WrapCheckLog(c), targetUnit, modelType, test.machinePortRanges, test.applicationPortRanges)
		rec.pendingOpenRanges = test.pendingOpenRanges
		rec.pendingCloseRanges = test.pendingCloseRanges

		err := rec.OpenPortRange(test.targetEndpoint, test.targetPortRange)
		if test.expectErr != "" {
			c.Check(err, tc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, tc.ErrorIsNil)

			pendingOpenRanges, pendingCloseRanges := rec.PendingChanges()
			c.Check(pendingOpenRanges, tc.DeepEquals, test.expectPendingOpen)
			c.Check(pendingCloseRanges, tc.DeepEquals, test.expectPendingClose)
		}
	}
}

func (s *PortRangeChangeRecorderSuite) TestClosePortRange(c *tc.C) {
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
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close an existing range - same unit; different endpoint (accepted even if not opened for that endpoint)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "sidecar unit: close an existing range - all endpoints",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "sidecar unit: close an existing range - same unit; different endpoint (accepted even if not opened for that endpoint)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectPendingClose: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close an non-existing range with pending open port(remove the pending open then do nothing)",
			targetEndpoint:  "foo",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			pendingOpenRanges: network.GroupedPortRanges{
				"foo": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"foo": []network.PortRange{},
			},
		},
		{
			about:           "close a range pending to be opened (removed from pending open)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingOpenRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"": []network.PortRange{},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close a range pending to be closed (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingCloseRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "sidecar unit: close a range pending to be opened (removed from pending open)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingOpenRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingOpen: network.GroupedPortRanges{
				"": []network.PortRange{},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "sidecar unit: close a range pending to be closed (ignored)",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			applicationPortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingCloseRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectPendingClose: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
		},
		{
			about:           "close a range opened by another unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("10-20/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("other/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot close 10-20/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "other/0"\)`,
		},
		{
			about:           "close a range conflicting with the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			expectErr: `cannot close 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\)`,
		},
		{
			about:           "close a range conflicting with a pending range for the same unit",
			targetEndpoint:  "",
			targetPortRange: network.MustParsePortRange("1-200/tcp"),
			machinePortRanges: map[names.UnitTag]network.GroupedPortRanges{
				names.NewUnitTag("u/0"): {
					"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
				},
			},
			pendingCloseRanges: network.GroupedPortRanges{
				"": []network.PortRange{network.MustParsePortRange("10-20/tcp")},
			},
			expectErr: `cannot close 1-200/tcp \(unit "u/0"\): port range conflicts with 10-20/tcp \(unit "u/0"\) requested earlier`,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		rec := newPortRangeChangeRecorder(loggertesting.WrapCheckLog(c), targetUnit, model.IAAS, test.machinePortRanges, test.applicationPortRanges)
		rec.pendingOpenRanges = test.pendingOpenRanges
		rec.pendingCloseRanges = test.pendingCloseRanges

		err := rec.ClosePortRange(test.targetEndpoint, test.targetPortRange)
		if test.expectErr != "" {
			c.Check(err, tc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, tc.ErrorIsNil)

			pendingOpenRanges, pendingCloseRanges := rec.PendingChanges()
			c.Check(pendingOpenRanges, tc.DeepEquals, test.expectPendingOpen)
			c.Check(pendingCloseRanges, tc.DeepEquals, test.expectPendingClose)
		}
	}
}
