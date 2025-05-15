// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type relationSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&relationSuite{})

func (s *relationSuite) TestRelation(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Firewaller")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "relation-mysql.db#wordpress.db"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: "alive"}},
		}
		return nil
	})
	tag := names.NewRelationTag("mysql:db wordpress:db")
	client, err := firewaller.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	r, err := client.Relation(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Life(), tc.Equals, life.Alive)
	c.Assert(r.Tag(), tc.DeepEquals, tag)
}
