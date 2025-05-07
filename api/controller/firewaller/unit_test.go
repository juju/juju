// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type unitSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&unitSuite{})

func (s *unitSuite) TestUnit(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: "alive"}},
		}
		return nil
	})
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Life(), tc.Equals, life.Alive)
	c.Assert(u.Name(), jc.DeepEquals, "mysql/666")
}

func (s *unitSuite) TestRefresh(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
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
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	err = u.Refresh(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Life(), tc.Equals, life.Dead)
	c.Assert(calls, tc.Equals, 2)
}

func (s *unitSuite) TestAssignedMachine(c *tc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
		})
		if calls > 0 {
			c.Check(request, tc.Equals, "GetAssignedMachine")
			c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
			*(result.(*params.StringResults)) = params.StringResults{
				Results: []params.StringResult{{Result: "machine-666"}},
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
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	m, err := u.AssignedMachine(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), tc.Equals, "666")
	c.Assert(calls, tc.Equals, 2)
}

func (s *unitSuite) TestApplication(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-mysql-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		c.Check(request, tc.Equals, "Life")
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: life.Alive}},
		}
		return nil
	})
	tag := names.NewUnitTag("mysql/666")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	u, err := client.Unit(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	app, err := u.Application()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Name(), tc.Equals, "mysql")
}
