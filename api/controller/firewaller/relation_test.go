// Copyright 2017 Canonical Ltd.
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

type relationSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestRelation(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Firewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "relation-mysql.db#wordpress.db"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: "alive"}},
		}
		return nil
	})
	tag := names.NewRelationTag("mysql:db wordpress:db")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	r, err := client.Relation(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Life(), gc.Equals, life.Alive)
	c.Assert(r.Tag(), jc.DeepEquals, tag)
}
