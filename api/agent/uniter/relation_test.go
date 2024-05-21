// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type relationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) TestRelation(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Relation")
		c.Assert(arg, gc.DeepEquals, params.RelationUnits{
			RelationUnits: []params.RelationUnit{
				{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.RelationResults{})
		*(result.(*params.RelationResults)) = params.RelationResults{
			Results: []params.RelationResult{{
				Life:      life.Alive,
				Suspended: false,
				Id:        666,
				Key:       "wordpress:db mysql:server",
				Endpoint: params.Endpoint{
					Relation: params.CharmRelation{
						Name:      "db",
						Role:      "requirer",
						Interface: "mysql",
						Optional:  true,
						Limit:     1,
						Scope:     "global",
					},
				},
				OtherApplication: "mysql",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel, err := client.Relation(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Id(), gc.Equals, 666)
	c.Assert(rel.Tag(), gc.Equals, tag)
	c.Assert(rel.Life(), gc.Equals, life.Alive)
	c.Assert(rel.String(), gc.Equals, tag.Id())
	c.Assert(rel.OtherApplication(), gc.Equals, "mysql")
	ep, err := rel.Endpoint(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ep, jc.DeepEquals, &uniter.Endpoint{
		charm.Relation{
			Name:      "db",
			Role:      "requirer",
			Interface: "mysql",
			Optional:  true,
			Limit:     1,
			Scope:     "global",
		},
	})
}

func (s *relationSuite) TestRefresh(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Relation")
		c.Assert(arg, gc.DeepEquals, params.RelationUnits{
			RelationUnits: []params.RelationUnit{
				{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.RelationResults{})
		*(result.(*params.RelationResults)) = params.RelationResults{
			Results: []params.RelationResult{{
				Life:      life.Dying,
				Suspended: true,
			}},
		}
		return nil
	})

	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	err := rel.Refresh(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, life.Dying)
	c.Assert(rel.Suspended(), jc.IsTrue)
}

func (s *relationSuite) TestSuspended(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	c.Assert(rel.Suspended(), jc.IsFalse)
	rel.UpdateSuspended(true)
	c.Assert(rel.Suspended(), jc.IsTrue)
}

func (s *relationSuite) TestSetStatus(c *gc.C) {
	statusSet := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SetRelationStatus")
		c.Assert(arg, gc.DeepEquals, params.RelationStatusArgs{
			Args: []params.RelationStatusArg{{
				UnitTag:    "unit-mysql-0",
				RelationId: 666,
				Status:     params.Suspended,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		statusSet = true
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	err := rel.SetStatus(context.Background(), relation.Suspended)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusSet, jc.IsTrue)
}

func (s *relationSuite) TestRelationById(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "RelationById")
		c.Assert(arg, gc.DeepEquals, params.RelationIds{RelationIds: []int{666}})
		c.Assert(result, gc.FitsTypeOf, &params.RelationResults{})
		*(result.(*params.RelationResults)) = params.RelationResults{
			Results: []params.RelationResult{{
				Id:        666,
				Life:      life.Alive,
				Suspended: true,
				Key:       "wordpress:db mysql:server",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	rel, err := client.RelationById(context.Background(), 666)
	c.Assert(rel.Id(), gc.Equals, 666)
	c.Assert(rel.Tag(), gc.Equals, names.NewRelationTag("wordpress:db mysql:server"))
	c.Assert(rel.Life(), gc.Equals, life.Alive)
	c.Assert(rel.Suspended(), jc.IsTrue)
	c.Assert(rel.String(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(err, jc.ErrorIsNil)
}
