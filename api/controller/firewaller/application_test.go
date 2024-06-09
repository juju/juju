// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type applicationSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) TestWatch(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls > 0 {
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "application-mysql"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
			c.Check(request, gc.Equals, "Watch")
			*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
				Results: []params.NotifyWatchResult{{Error: &params.Error{Message: "FAIL"}}},
			}
		} else {
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "unit-mysql-666"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			c.Check(request, gc.Equals, "Life")
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
	app, err := u.Application()
	c.Assert(err, jc.ErrorIsNil)
	_, err = app.Watch(context.Background())
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(calls, gc.Equals, 2)
}

func (s *applicationSuite) TestExposeInfo(c *gc.C) {
	calls := 0
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		if calls > 0 {
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "application-mysql"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ExposeInfoResults{})
			c.Check(request, gc.Equals, "GetExposeInfo")
			*(result.(*params.ExposeInfoResults)) = params.ExposeInfoResults{
				Results: []params.ExposeInfoResult{{
					Exposed: true,
					ExposedEndpoints: map[string]params.ExposedEndpoint{
						"database": {
							ExposeToSpaces: []string{"space"},
							ExposeToCIDRs:  []string{"cidr"},
						},
					},
				}},
			}
		} else {
			c.Assert(arg, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: "unit-mysql-666"}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			c.Check(request, gc.Equals, "Life")
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
	app, err := u.Application()
	c.Assert(err, jc.ErrorIsNil)
	exposed, info, err := app.ExposeInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exposed, jc.IsTrue)
	c.Assert(info, jc.DeepEquals, map[string]params.ExposedEndpoint{
		"database": {
			ExposeToSpaces: []string{"space"},
			ExposeToCIDRs:  []string{"cidr"},
		},
	})
	c.Assert(calls, gc.Equals, 2)
}
