// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type uniterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) TestProviderType(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "ProviderType")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringResult{})
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "somecloud",
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	providerType, err := client.ProviderType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerType, gc.Equals, "somecloud")
}

func (s *uniterSuite) TestOpenedMachinePortRangesByEndpoint(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "OpenedMachinePortRangesByEndpoint")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "machine-42"}}})
		c.Assert(result, gc.FitsTypeOf, &params.OpenPortRangesByEndpointResults{})
		*(result.(*params.OpenPortRangesByEndpointResults)) = params.OpenPortRangesByEndpointResults{
			Results: []params.OpenPortRangesByEndpointResult{
				{
					UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
						"unit-mysql-0": {
							{
								Endpoint:   "",
								PortRanges: []params.PortRange{{100, 200, "tcp"}},
							},
							{
								Endpoint:   "server",
								PortRanges: []params.PortRange{{3306, 3306, "tcp"}},
							},
						},
						"unit-wordpress-0": {
							{
								Endpoint:   "monitoring-port",
								PortRanges: []params.PortRange{{1337, 1337, "udp"}},
							},
						},
					},
				},
			},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 17}
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

func (s *uniterSuite) TestOpenedPortRangesByEndpoint(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "OpenedPortRangesByEndpoint")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.OpenPortRangesByEndpointResults{})
		*(result.(*params.OpenPortRangesByEndpointResults)) = params.OpenPortRangesByEndpointResults{
			Results: []params.OpenPortRangesByEndpointResult{
				{
					UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
						"unit-mysql-0": {
							{
								Endpoint:   "",
								PortRanges: []params.PortRange{{100, 200, "tcp"}},
							},
							{
								Endpoint:   "server",
								PortRanges: []params.PortRange{{3306, 3306, "tcp"}},
							},
						},
						"unit-wordpress-0": {
							{
								Endpoint:   "monitoring-port",
								PortRanges: []params.PortRange{{1337, 1337, "udp"}},
							},
						},
					},
				},
			},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 18}
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

func (s *uniterSuite) TestOpenedPortRangesByEndpointOldAPINotSupported(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "OpenedPortRangesByEndpoint")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-gitlab-0"}}})
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 17}
	client := uniter.NewClient(caller, names.NewUnitTag("gitlab/0"))

	_, err := client.OpenedPortRangesByEndpoint(context.Background())
	c.Assert(err, gc.ErrorMatches, `OpenedPortRangesByEndpoint\(\) \(need V18\+\) not implemented`)
}

func (s *uniterSuite) TestUnitWorkloadVersion(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WorkloadVersion")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Result: "mysql-1.2.3"}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 17}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	workloadVersion, err := client.UnitWorkloadVersion(context.Background(), names.NewUnitTag("mysql/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workloadVersion, gc.Equals, "mysql-1.2.3")
}

func (s *uniterSuite) TestSetUnitWorkloadVersion(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SetWorkloadVersion")
		c.Assert(arg, gc.DeepEquals, params.EntityWorkloadVersions{Entities: []params.EntityWorkloadVersion{{Tag: "unit-mysql-0", WorkloadVersion: "mysql-1.2.3"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 17}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	err := client.SetUnitWorkloadVersion(context.Background(), names.NewUnitTag("mysql/0"), "mysql-1.2.3")
	c.Assert(err, jc.ErrorIsNil)
}
