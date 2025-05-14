// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/machiner"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}

type machinerSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&machinerSuite{})

func (s *machinerSuite) TestMachineAndMachineTag(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: "alive"}},
		}
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, life.Alive)
	c.Assert(m.Tag(), tc.DeepEquals, tag)
}

func (s *machinerSuite) TestSetStatus(c *tc.C) {
	data := map[string]interface{}{"foo": "bar"}
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		if calls == 0 {
			c.Check(request, tc.Equals, "Life")
			c.Assert(arg, tc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "machine-666"}},
			})
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: "alive"}},
			}
		} else {
			c.Check(request, tc.Equals, "SetStatus")
			c.Assert(arg, tc.DeepEquals, params.SetStatus{
				Entities: []params.EntityStatusArgs{{
					Tag:    "machine-666",
					Status: "error",
					Info:   "failed",
					Data:   data,
				}},
			})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	err = m.SetStatus(c.Context(), status.Error, "failed", data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.Equals, 2)
}

func (s *machinerSuite) TestEnsureDead(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Check(request, tc.Equals, "EnsureDead")
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, tc.Equals, "Life")
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	err = m.EnsureDead(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machinerSuite) TestRefresh(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
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
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	err = m.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, life.Dead)
	c.Assert(calls, tc.Equals, 2)
}

func (s *machinerSuite) TestSetMachineAddresses(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		if calls > 0 {
			c.Check(request, tc.Equals, "SetMachineAddresses")
			c.Assert(arg, tc.DeepEquals, params.SetMachinesAddresses{
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
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, tc.Equals, "Life")
			c.Assert(arg, tc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "machine-666"}},
			})
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	err = m.SetMachineAddresses(c.Context(), []network.MachineAddress{{
		Value:       "10.0.0.1",
		Type:        network.IPv6Address,
		Scope:       network.ScopeCloudLocal,
		CIDR:        "10.0.0.0/24",
		ConfigType:  network.ConfigDHCP,
		IsSecondary: true,
	}})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machinerSuite) TestWatch(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
			c.Check(request, tc.Equals, "Watch")
			*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
				Results: []params.NotifyWatchResult{{Error: &params.Error{Message: "FAIL"}}},
			}
		} else {
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			c.Check(request, tc.Equals, "Life")
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	_, err = m.Watch(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
	c.Assert(calls, tc.Equals, 2)
}

func (s *machinerSuite) TestRecordAgentStartInformation(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Machiner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		if calls > 0 {
			c.Check(request, tc.Equals, "RecordAgentStartInformation")
			c.Assert(arg, tc.DeepEquals, params.RecordAgentStartInformationArgs{
				Args: []params.RecordAgentStartInformationArg{
					{
						Tag:      "machine-666",
						Hostname: "hostname",
					},
				},
			})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, tc.Equals, "Life")
			c.Assert(arg, tc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "machine-666"}},
			})
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: life.Alive}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client := machiner.NewClient(apiCaller)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	err = m.RecordAgentStartInformation(c.Context(), "hostname")
	c.Assert(err, tc.ErrorIsNil)
}
