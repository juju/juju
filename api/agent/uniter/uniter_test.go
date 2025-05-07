// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type uniterSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&uniterSuite{})

func (s *uniterSuite) TestProviderType(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "ProviderType")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringResult{})
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "somecloud",
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	providerType, err := client.ProviderType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerType, tc.Equals, "somecloud")
}

func (s *uniterSuite) TestOpenedMachinePortRangesByEndpoint(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "OpenedMachinePortRangesByEndpoint")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "machine-42"}}})
		c.Assert(result, tc.FitsTypeOf, &params.OpenPortRangesByEndpointResults{})
		*(result.(*params.OpenPortRangesByEndpointResults)) = params.OpenPortRangesByEndpointResults{
			Results: []params.OpenPortRangesByEndpointResult{
				{
					UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
						"unit-mysql-0": {
							{
								Endpoint:   "",
								PortRanges: []params.PortRange{{FromPort: 100, ToPort: 200, Protocol: "tcp"}},
							},
							{
								Endpoint:   "server",
								PortRanges: []params.PortRange{{FromPort: 3306, ToPort: 3306, Protocol: "tcp"}},
							},
						},
						"unit-wordpress-0": {
							{
								Endpoint:   "monitoring-port",
								PortRanges: []params.PortRange{{FromPort: 1337, ToPort: 1337, Protocol: "udp"}},
							},
						},
					},
				},
			},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 17}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	portRangesMap, err := client.OpenedMachinePortRangesByEndpoint(context.Background(), names.NewMachineTag("42"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(portRangesMap, jc.DeepEquals, map[names.UnitTag]network.GroupedPortRanges{
		names.NewUnitTag("mysql/0"): {
			"":       []network.PortRange{network.MustParsePortRange("100-200/tcp")},
			"server": []network.PortRange{network.MustParsePortRange("3306/tcp")},
		},
		names.NewUnitTag("wordpress/0"): {
			"monitoring-port": []network.PortRange{network.MustParsePortRange("1337/udp")},
		},
	})
}

func (s *uniterSuite) TestOpenedPortRangesByEndpoint(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "OpenedPortRangesByEndpoint")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.OpenPortRangesByEndpointResults{})
		*(result.(*params.OpenPortRangesByEndpointResults)) = params.OpenPortRangesByEndpointResults{
			Results: []params.OpenPortRangesByEndpointResult{
				{
					UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
						"unit-mysql-0": {
							{
								Endpoint:   "",
								PortRanges: []params.PortRange{{FromPort: 100, ToPort: 200, Protocol: "tcp"}},
							},
							{
								Endpoint:   "server",
								PortRanges: []params.PortRange{{FromPort: 3306, ToPort: 3306, Protocol: "tcp"}},
							},
						},
						"unit-wordpress-0": {
							{
								Endpoint:   "monitoring-port",
								PortRanges: []params.PortRange{{FromPort: 1337, ToPort: 1337, Protocol: "udp"}},
							},
						},
					},
				},
			},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 18}
	client := uniter.NewClient(caller, names.NewUnitTag("gitlab/0"))

	result, err := client.OpenedPortRangesByEndpoint(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, map[names.UnitTag]network.GroupedPortRanges{
		names.NewUnitTag("mysql/0"): {
			"":       []network.PortRange{network.MustParsePortRange("100-200/tcp")},
			"server": []network.PortRange{network.MustParsePortRange("3306/tcp")},
		},
		names.NewUnitTag("wordpress/0"): {
			"monitoring-port": []network.PortRange{network.MustParsePortRange("1337/udp")},
		},
	})
}

func (s *uniterSuite) TestOpenedPortRangesByEndpointOldAPINotSupported(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "OpenedPortRangesByEndpoint")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-gitlab-0"}}})
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 17}
	client := uniter.NewClient(caller, names.NewUnitTag("gitlab/0"))

	_, err := client.OpenedPortRangesByEndpoint(context.Background())
	c.Assert(err, tc.ErrorMatches, `OpenedPortRangesByEndpoint\(\) \(need V18\+\) not implemented`)
}

func (s *uniterSuite) TestUnitWorkloadVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WorkloadVersion")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Result: "mysql-1.2.3"}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 17}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	workloadVersion, err := client.UnitWorkloadVersion(context.Background(), names.NewUnitTag("mysql/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workloadVersion, tc.Equals, "mysql-1.2.3")
}

func (s *uniterSuite) TestSetUnitWorkloadVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetWorkloadVersion")
		c.Assert(arg, tc.DeepEquals, params.EntityWorkloadVersions{Entities: []params.EntityWorkloadVersion{{Tag: "unit-mysql-0", WorkloadVersion: "mysql-1.2.3"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 17}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	err := client.SetUnitWorkloadVersion(context.Background(), names.NewUnitTag("mysql/0"), "mysql-1.2.3")
	c.Assert(err, jc.ErrorIsNil)
}
