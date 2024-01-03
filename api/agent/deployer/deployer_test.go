// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type deployerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) TestWatchUnits(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Deployer")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchUnits")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "machine-666",
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := deployer.NewClient(apiCaller)
	machineTag := names.NewMachineTag("666")
	machine, err := client.Machine(machineTag)
	c.Assert(err, jc.ErrorIsNil)
	_, err = machine.WatchUnits()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *deployerSuite) TestUnit(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Deployer")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "unit-mysql-666",
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := deployer.NewClient(apiCaller)
	unitTag := names.NewUnitTag("mysql/666")
	unit, err := client.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, life.Alive)
}

func (s *deployerSuite) TestUnitLifeRefresh(c *gc.C) {
	calls := 0
	lifeResult := life.Alive
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Deployer")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "unit-mysql-666",
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		if calls > 0 {
			lifeResult = life.Dying
		}
		calls++
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: lifeResult,
			}},
		}
		return nil
	})

	client := deployer.NewClient(apiCaller)
	unitTag := names.NewUnitTag("mysql/666")
	unit, err := client.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, life.Dying)
	c.Assert(calls, gc.Equals, 2)
}

func (s *deployerSuite) TestUnitRemove(c *gc.C) {
	calls := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Deployer")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "unit-mysql-666",
		}}})
		if calls > 0 {
			c.Check(request, gc.Equals, "Remove")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, gc.Equals, "Life")
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{
					Life: life.Alive,
				}},
			}
		}
		calls++
		return nil
	})

	client := deployer.NewClient(apiCaller)
	unitTag := names.NewUnitTag("mysql/666")
	unit, err := client.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calls, gc.Equals, 2)
}

func (s *deployerSuite) TestUnitSetPassword(c *gc.C) {
	calls := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Deployer")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls > 0 {
			c.Check(arg, jc.DeepEquals, params.EntityPasswords{
				Changes: []params.EntityPassword{
					{Tag: "unit-mysql-666", Password: "secret"},
				},
			})
			c.Check(request, gc.Equals, "SetPasswords")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "unit-mysql-666",
			}}})
			c.Check(request, gc.Equals, "Life")
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{
					Life: life.Alive,
				}},
			}
		}
		calls++
		return nil
	})

	client := deployer.NewClient(apiCaller)
	unitTag := names.NewUnitTag("mysql/666")
	unit, err := client.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword("secret")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calls, gc.Equals, 2)
}

func (s *deployerSuite) TestUnitSetStatus(c *gc.C) {
	calls := 0
	data := map[string]interface{}{"foo": "bar"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Deployer")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls > 0 {
			c.Check(arg, jc.DeepEquals, params.SetStatus{
				Entities: []params.EntityStatusArgs{{Tag: "unit-mysql-666", Status: "active", Info: "is active", Data: data}},
			})
			c.Check(request, gc.Equals, "SetStatus")
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "unit-mysql-666",
			}}})
			c.Check(request, gc.Equals, "Life")
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{
					Life: life.Alive,
				}},
			}
		}
		calls++
		return nil
	})

	client := deployer.NewClient(apiCaller)
	unitTag := names.NewUnitTag("mysql/666")
	unit, err := client.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetStatus(status.Active, "is active", data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calls, gc.Equals, 2)
}
