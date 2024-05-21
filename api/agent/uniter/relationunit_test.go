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
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type relationUnitSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&relationUnitSuite{})

func (s *relationUnitSuite) getRelationUnit(c *gc.C) *uniter.RelationUnit {
	relUnitArg := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
		},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		switch request {
		case "Relation":
			c.Assert(arg, gc.DeepEquals, relUnitArg)
			c.Assert(result, gc.FitsTypeOf, &params.RelationResults{})
			*(result.(*params.RelationResults)) = params.RelationResults{
				Results: []params.RelationResult{{
					Id:        666,
					Key:       "wordpress:db mysql:server",
					Life:      life.Alive,
					Suspended: true,
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
		case "EnterScope":
			c.Assert(arg, gc.DeepEquals, relUnitArg)
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: &params.Error{Message: "boom"}}},
			}
		case "LeaveScope":
			c.Assert(arg, gc.DeepEquals, relUnitArg)
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: &params.Error{Message: "bam"}}},
			}
		case "ReadSettings":
			c.Assert(arg, gc.DeepEquals, relUnitArg)
			c.Assert(result, gc.FitsTypeOf, &params.SettingsResults{})
			*(result.(*params.SettingsResults)) = params.SettingsResults{
				Results: []params.SettingsResult{{
					Settings: params.Settings{
						"some":  "settings",
						"other": "things",
					},
				}},
			}
		case "ReadLocalApplicationSettings":
			c.Assert(arg, gc.DeepEquals, params.RelationUnit{
				Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0",
			})
			c.Assert(result, gc.FitsTypeOf, &params.SettingsResult{})
			*(result.(*params.SettingsResult)) = params.SettingsResult{
				Settings: params.Settings{
					"foo": "bar",
					"baz": "1",
				},
			}
		default:
			c.Fatalf("unexpected api call %q", request)
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	rel := uniter.CreateRelation(client, tag)
	relUnit, err := rel.Unit(context.Background(), names.NewUnitTag("mysql/0"))
	c.Assert(err, jc.ErrorIsNil)
	return relUnit
}

func (s *relationUnitSuite) TestRelation(c *gc.C) {
	relUnit := s.getRelationUnit(c)
	apiRel := relUnit.Relation()
	c.Assert(apiRel, gc.NotNil)
	c.Assert(apiRel.String(), gc.Equals, "wordpress:db mysql:server")
}

func (s *relationUnitSuite) TestEndpoint(c *gc.C) {
	relUnit := s.getRelationUnit(c)

	apiEndpoint := relUnit.Endpoint()
	c.Assert(apiEndpoint, gc.DeepEquals, uniter.Endpoint{
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

func (s *relationUnitSuite) TestEnterScope(c *gc.C) {
	relUnit := s.getRelationUnit(c)
	err := relUnit.EnterScope()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *relationUnitSuite) TestLeaveScope(c *gc.C) {
	relUnit := s.getRelationUnit(c)
	err := relUnit.LeaveScope()
	c.Assert(err, gc.ErrorMatches, "bam")
}

func (s *relationUnitSuite) TestSettings(c *gc.C) {
	relUnit := s.getRelationUnit(c)
	gotSettings, err := relUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"some":  "settings",
		"other": "things",
	})
}

func (s *relationUnitSuite) TestApplicationSettings(c *gc.C) {
	relUnit := s.getRelationUnit(c)
	gotSettings, err := relUnit.ApplicationSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"foo": "bar",
		"baz": "1",
	})
}

func (s *relationUnitSuite) TestWatchRelationUnits(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if request == "Stop" || request == "Next" {
			return nil
		}
		c.Assert(request, gc.Equals, "WatchRelationUnits")
		c.Assert(arg, gc.DeepEquals, params.RelationUnits{
			RelationUnits: []params.RelationUnit{
				{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.RelationUnitsWatchResults{})
		*(result.(*params.RelationUnitsWatchResults)) = params.RelationUnitsWatchResults{
			Results: []params.RelationUnitsWatchResult{{
				RelationUnitsWatcherId: "1",
				Changes: params.RelationUnitsChange{
					Changed:    map[string]params.UnitSettings{"mysql/0": {}},
					AppChanged: map[string]int64{"mysql": 665},
					Departed:   []string{"666"},
				},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	tag := names.NewRelationTag("wordpress:db mysql:server")
	w, err := client.WatchRelationUnits(context.Background(), tag, names.NewUnitTag("mysql/0"))
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewRelationUnitsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange([]string{"mysql/0"}, []string{"mysql"}, []string{"666"})
}
