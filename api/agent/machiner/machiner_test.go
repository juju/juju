// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	stdtesting "testing"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/machiner"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type machinerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&machinerSuite{})

func (s *machinerSuite) TestMachineAndMachineTag(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
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
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, life.Alive)
	c.Assert(m.Tag(), jc.DeepEquals, tag)
}

func (s *machinerSuite) TestSetStatus(c *gc.C) {
	data := map[string]interface{}{"foo": "bar"}
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls == 0 {
			c.Check(request, gc.Equals, "Life")
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "machine-666"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: "alive"}},
			}
		} else {
			c.Check(request, gc.Equals, "SetStatus")
			c.Assert(arg, jc.DeepEquals, params.SetStatus{
				Entities: []params.EntityStatusArgs{{
					Tag:    "machine-666",
					Status: "error",
					Info:   "failed",
					Data:   data,
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetStatus(status.Error, "failed", data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calls, gc.Equals, 2)
}

func (s *machinerSuite) TestEnsureDead(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Check(request, gc.Equals, "EnsureDead")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, gc.Equals, "Life")
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machinerSuite) TestRefresh(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		lifeVal := life.Alive
		if calls > 0 {
			lifeVal = life.Dead
		}
		calls++
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: lifeVal}},
		}
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, life.Dead)
	c.Assert(calls, gc.Equals, 2)
}

func (s *machinerSuite) TestSetMachineAddresses(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls > 0 {
			c.Check(request, gc.Equals, "SetMachineAddresses")
			c.Assert(arg, jc.DeepEquals, params.SetMachinesAddresses{
				MachineAddresses: []params.MachineAddresses{{
					Tag: "machine-666",
					Addresses: []params.Address{{
						Value:       "10.0.0.1",
						CIDR:        "10.0.0.0/24",
						Type:        "ipv6",
						Scope:       "local-cloud",
						ConfigType:  "dhcp",
						IsSecondary: true,
					}},
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, gc.Equals, "Life")
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "machine-666"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineAddresses([]network.MachineAddress{{
		Value:       "10.0.0.1",
		Type:        network.IPv6Address,
		Scope:       network.ScopeCloudLocal,
		CIDR:        "10.0.0.0/24",
		ConfigType:  network.ConfigDHCP,
		IsSecondary: true,
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machinerSuite) TestWatch(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
			c.Check(request, gc.Equals, "Watch")
			*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
				Results: []params.NotifyWatchResult{{Error: &params.Error{Message: "FAIL"}}},
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
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	_, err = m.Watch()
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(calls, gc.Equals, 2)
}

func (s *machinerSuite) TestRecordAgentStartInformation(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Machiner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls > 0 {
			c.Check(request, gc.Equals, "RecordAgentStartInformation")
			c.Assert(arg, jc.DeepEquals, params.RecordAgentStartInformationArgs{
				Args: []params.RecordAgentStartInformationArg{
					{
						Tag:      "machine-666",
						Hostname: "hostname",
					},
				},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, gc.Equals, "Life")
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "machine-666"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = m.RecordAgentStartInformation("hostname")
	c.Assert(err, jc.ErrorIsNil)
}
