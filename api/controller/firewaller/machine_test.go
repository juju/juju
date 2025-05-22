// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type machineSuite struct {
	coretesting.BaseSuite
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

func (s *machineSuite) TestMachine(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
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
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, life.Alive)
	c.Assert(m.Tag(), tc.DeepEquals, tag)
}

func (s *machineSuite) TestInstanceId(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls == 0 {
			c.Check(request, tc.Equals, "Life")
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{Life: "alive"}},
			}
		} else {
			c.Check(request, tc.Equals, "InstanceId")
			c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
			*(result.(*params.StringResults)) = params.StringResults{
				Results: []params.StringResult{{Result: "inst-666"}},
			}
		}
		calls++
		return nil
	})
	tag := names.NewMachineTag("666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	id, err := m.InstanceId(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Life(), tc.Equals, life.Alive)
	c.Assert(id, tc.Equals, instance.Id("inst-666"))
	c.Assert(calls, tc.Equals, 2)
}

func (s *machineSuite) TestWatchUnits(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
			c.Check(request, tc.Equals, "WatchUnits")
			*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
				Results: []params.StringsWatchResult{{Error: &params.Error{Message: "FAIL"}}},
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
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	_, err = m.WatchUnits(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
	c.Assert(calls, tc.Equals, 2)
}

func (s *machineSuite) TestIsManual(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		if calls > 0 {
			c.Assert(result, tc.FitsTypeOf, &params.BoolResults{})
			c.Check(request, tc.Equals, "AreManuallyProvisioned")
			*(result.(*params.BoolResults)) = params.BoolResults{
				Results: []params.BoolResult{{Result: true}},
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
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	m, err := client.Machine(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	result, err := m.IsManual(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.IsTrue)
	c.Assert(calls, tc.Equals, 2)
}
