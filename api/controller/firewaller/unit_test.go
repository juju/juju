// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type unitSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) TestUnit(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: "alive"}},
		}
		return nil
	})
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Life(), gc.Equals, life.Alive)
	c.Assert(u.Name(), jc.DeepEquals, "mysql/666")
	c.Assert(u.Tag(), jc.DeepEquals, tag)
}

func (s *unitSuite) TestRefresh(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
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
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Life(), gc.Equals, life.Dead)
	c.Assert(calls, gc.Equals, 2)
}

func (s *unitSuite) TestAssignedMachine(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
		})
		if calls > 0 {
			c.Check(request, gc.Equals, "GetAssignedMachine")
			c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
			*(result.(*params.StringResults)) = params.StringResults{
				Results: []params.StringResult{{Result: "machine-666"}},
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
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(tag)
	c.Assert(err, jc.ErrorIsNil)
	m, err := u.AssignedMachine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "666")
	c.Assert(calls, gc.Equals, 2)
}

func (s *unitSuite) TestApplication(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		c.Check(request, gc.Equals, "Life")
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: life.Alive}},
		}
		return nil
	})
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(tag)
	c.Assert(err, jc.ErrorIsNil)
	app, err := u.Application()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Name(), gc.Equals, "mysql")
}
