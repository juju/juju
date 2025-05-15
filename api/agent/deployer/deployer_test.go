// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}

type deployerSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&deployerSuite{})

func (s *deployerSuite) TestWatchUnits(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Deployer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchUnits")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "machine-666",
		}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
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
	c.Assert(err, tc.ErrorIsNil)
	_, err = machine.WatchUnits(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *deployerSuite) TestUnit(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Deployer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "unit-mysql-666",
		}}})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := deployer.NewClient(apiCaller)
	unitTag := names.NewUnitTag("mysql/666")
	unit, err := client.Unit(c.Context(), unitTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Life(), tc.Equals, life.Alive)
}

func (s *deployerSuite) TestUnitLifeRefresh(c *tc.C) {
	calls := 0
	lifeResult := life.Alive
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Deployer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "unit-mysql-666",
		}}})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
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
	unit, err := client.Unit(c.Context(), unitTag)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.Life(), tc.Equals, life.Dying)
	c.Assert(calls, tc.Equals, 2)
}

func (s *deployerSuite) TestUnitRemove(c *tc.C) {
	calls := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Deployer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
			Tag: "unit-mysql-666",
		}}})
		if calls > 0 {
			c.Check(request, tc.Equals, "Remove")
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(request, tc.Equals, "Life")
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
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
	unit, err := client.Unit(c.Context(), unitTag)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.Remove(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.Equals, 2)
}

func (s *deployerSuite) TestUnitSetPassword(c *tc.C) {
	calls := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Deployer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		if calls > 0 {
			c.Check(arg, tc.DeepEquals, params.EntityPasswords{
				Changes: []params.EntityPassword{
					{Tag: "unit-mysql-666", Password: "secret"},
				},
			})
			c.Check(request, tc.Equals, "SetPasswords")
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "unit-mysql-666",
			}}})
			c.Check(request, tc.Equals, "Life")
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
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
	unit, err := client.Unit(c.Context(), unitTag)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetPassword(c.Context(), "secret")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.Equals, 2)
}

func (s *deployerSuite) TestUnitSetStatus(c *tc.C) {
	calls := 0
	data := map[string]interface{}{"foo": "bar"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Deployer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		if calls > 0 {
			c.Check(arg, tc.DeepEquals, params.SetStatus{
				Entities: []params.EntityStatusArgs{{Tag: "unit-mysql-666", Status: "active", Info: "is active", Data: data}},
			})
			c.Check(request, tc.Equals, "SetStatus")
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
		} else {
			c.Check(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{
				Tag: "unit-mysql-666",
			}}})
			c.Check(request, tc.Equals, "Life")
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
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
	unit, err := client.Unit(c.Context(), unitTag)
	c.Assert(err, tc.ErrorIsNil)
	err = unit.SetStatus(c.Context(), status.Active, "is active", data)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calls, tc.Equals, 2)
}
