// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type machineSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) TestMachine(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: "alive"}},
		}
		return nil
	})
	tag := names.NewMachineTag("666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, life.Alive)
	c.Assert(m.Tag(), jc.DeepEquals, tag)
}

func (s *machineSuite) TestInstanceId(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls == 0 {
			c.Check(request, gc.Equals, "Life")
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: "alive"}},
			}
		} else {
			c.Check(request, gc.Equals, "InstanceId")
			c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
			*(result.(*params.StringResults)) = params.StringResults{
				Results: []params.StringResult{{Result: "inst-666"}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	id, err := m.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, life.Alive)
	c.Assert(id, gc.Equals, instance.Id("inst-666"))
	c.Assert(calls, gc.Equals, 2)
}

func (s *machineSuite) TestWatchUnits(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
			c.Check(request, gc.Equals, "WatchUnits")
			*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
				Results: []params.StringsWatchResult{{Error: &params.Error{Message: "FAIL"}}},
			}
		} else {
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			c.Check(request, gc.Equals, "Life")
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	_, err = m.WatchUnits()
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(calls, gc.Equals, 2)
}

func (s *machineSuite) TestIsManual(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, gc.FitsTypeOf, &params.BoolResults{})
			c.Check(request, gc.Equals, "AreManuallyProvisioned")
			*(result.(*params.BoolResults)) = params.BoolResults{
				Results: []params.BoolResult{{Result: true}},
			}
		} else {
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			c.Check(request, gc.Equals, "Life")
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	result, err := m.IsManual()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsTrue)
	c.Assert(calls, gc.Equals, 2)
}

func (s *machineSuite) TestOpenedPortRanges(c *gc.C) {
	results := map[string][]params.OpenUnitPortRanges{
		"unit-mysql-0": {
			{
				Endpoint:    "server",
				SubnetCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
				PortRanges: []params.PortRange{
					params.FromNetworkPortRange(network.MustParsePortRange("3306/tcp")),
				},
			},
		},
		"unit-wordpress-0": {
			{
				Endpoint:    "website",
				SubnetCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
				PortRanges: []params.PortRange{
					params.FromNetworkPortRange(network.MustParsePortRange("80/tcp")),
				},
			},
			{
				Endpoint:    "metrics",
				SubnetCIDRs: []string{"10.0.0.0/24", "10.0.1.0/24", "192.168.0.0/24", "192.168.1.0/24"},
				PortRanges: []params.PortRange{
					params.FromNetworkPortRange(network.MustParsePortRange("1337/tcp")),
				},
			},
		},
	}

	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, gc.FitsTypeOf, &params.OpenMachinePortRangesResults{})
			c.Check(request, gc.Equals, "OpenedMachinePortRanges")
			*(result.(*params.OpenMachinePortRangesResults)) = params.OpenMachinePortRangesResults{
				Results: []params.OpenMachinePortRangesResult{{
					UnitPortRanges: results,
				}},
			}
		} else {
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			c.Check(request, gc.Equals, "Life")
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})

	tag := names.NewMachineTag("666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)

	byUnitAndCIDR, byUnitAndEndpoint, err := m.OpenedMachinePortRanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(byUnitAndCIDR, jc.DeepEquals, map[names.UnitTag]network.GroupedPortRanges{
		names.NewUnitTag("mysql/0"): {
			"192.168.0.0/24": []network.PortRange{
				network.MustParsePortRange("3306/tcp"),
			},
			"192.168.1.0/24": []network.PortRange{
				network.MustParsePortRange("3306/tcp"),
			},
		},
		names.NewUnitTag("wordpress/0"): {
			"10.0.0.0/24": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
			"10.0.1.0/24": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
			"192.168.0.0/24": []network.PortRange{
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("1337/tcp"),
			},
			"192.168.1.0/24": []network.PortRange{
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("1337/tcp"),
			},
		},
	})

	c.Assert(byUnitAndEndpoint, jc.DeepEquals, map[names.UnitTag]network.GroupedPortRanges{
		names.NewUnitTag("mysql/0"): {
			"server": []network.PortRange{
				network.MustParsePortRange("3306/tcp"),
			},
		},
		names.NewUnitTag("wordpress/0"): {
			"website": []network.PortRange{
				network.MustParsePortRange("80/tcp"),
			},
			"metrics": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
		},
	})
	c.Assert(calls, gc.Equals, 2)
}
