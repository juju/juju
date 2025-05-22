// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type relationSuite struct {
	testing.BaseSuite
}

func TestRelationSuite(t *stdtesting.T) {
	tc.Run(t, &relationSuite{})
}

func (s *relationSuite) TestRelation(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Relation")
		c.Assert(arg, tc.DeepEquals, params.RelationUnits{
			RelationUnits: []params.RelationUnit{
				{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.RelationResultsV2{})
		*(result.(*params.RelationResultsV2)) = params.RelationResultsV2{
			Results: []params.RelationResultV2{{
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
				OtherApplication: params.RelatedApplicationDetails{
					ApplicationName: "mysql",
					ModelUUID:       testing.ModelTag.Id(),
				},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel, err := client.Relation(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.Id(), tc.Equals, 666)
	c.Assert(rel.Tag(), tc.Equals, tag)
	c.Assert(rel.Life(), tc.Equals, life.Alive)
	c.Assert(rel.String(), tc.Equals, tag.Id())
	c.Assert(rel.OtherApplication(), tc.Equals, "mysql")
	c.Assert(rel.OtherModelUUID(), tc.Equals, testing.ModelTag.Id())
	ep, err := rel.Endpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ep, tc.DeepEquals, &uniter.Endpoint{
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

func (s *relationSuite) TestRefresh(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Relation")
		c.Assert(arg, tc.DeepEquals, params.RelationUnits{
			RelationUnits: []params.RelationUnit{
				{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.RelationResultsV2{})
		*(result.(*params.RelationResultsV2)) = params.RelationResultsV2{
			Results: []params.RelationResultV2{{
				Life:      life.Dying,
				Suspended: true,
			}},
		}
		return nil
	})

	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	err := rel.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.Life(), tc.Equals, life.Dying)
	c.Assert(rel.Suspended(), tc.IsTrue)
}

func (s *relationSuite) TestSuspended(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	c.Assert(rel.Suspended(), tc.IsFalse)
	rel.UpdateSuspended(true)
	c.Assert(rel.Suspended(), tc.IsTrue)
}

func (s *relationSuite) TestSetStatus(c *tc.C) {
	statusSet := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "SetRelationStatus")
		c.Assert(arg, tc.DeepEquals, params.RelationStatusArgs{
			Args: []params.RelationStatusArg{{
				UnitTag:    "unit-mysql-0",
				RelationId: 666,
				Status:     params.Suspended,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		statusSet = true
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	err := rel.SetStatus(c.Context(), relation.Suspended)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusSet, tc.IsTrue)
}

func (s *relationSuite) TestRelationById(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "RelationById")
		c.Assert(arg, tc.DeepEquals, params.RelationIds{RelationIds: []int{666}})
		c.Assert(result, tc.FitsTypeOf, &params.RelationResultsV2{})
		*(result.(*params.RelationResultsV2)) = params.RelationResultsV2{
			Results: []params.RelationResultV2{{
				Id:        666,
				Life:      life.Alive,
				Suspended: true,
				Key:       "wordpress:db mysql:server",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	rel, err := client.RelationById(c.Context(), 666)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rel.Id(), tc.Equals, 666)
	c.Assert(rel.Tag(), tc.Equals, names.NewRelationTag("wordpress:db mysql:server"))
	c.Assert(rel.Life(), tc.Equals, life.Alive)
	c.Assert(rel.Suspended(), tc.IsTrue)
	c.Assert(rel.String(), tc.Equals, "wordpress:db mysql:server")
	c.Assert(err, tc.ErrorIsNil)
}
