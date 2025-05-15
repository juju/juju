// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type relationUnitSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&relationUnitSuite{})

func (s *relationUnitSuite) getRelationUnit(c *tc.C) *uniter.RelationUnit {
	relUnitArg := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
		},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		switch request {
		case "Relation":
			c.Assert(arg, tc.DeepEquals, relUnitArg)
			c.Assert(result, tc.FitsTypeOf, &params.RelationResultsV2{})
			*(result.(*params.RelationResultsV2)) = params.RelationResultsV2{
				Results: []params.RelationResultV2{{
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
					OtherApplication: params.RelatedApplicationDetails{
						ModelUUID:       testing.ModelTag.Id(),
						ApplicationName: "mysql",
					},
				}},
			}
		case "EnterScope":
			c.Assert(arg, tc.DeepEquals, relUnitArg)
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: &params.Error{Message: "boom"}}},
			}
		case "LeaveScope":
			c.Assert(arg, tc.DeepEquals, relUnitArg)
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: &params.Error{Message: "bam"}}},
			}
		case "ReadSettings":
			c.Assert(arg, tc.DeepEquals, relUnitArg)
			c.Assert(result, tc.FitsTypeOf, &params.SettingsResults{})
			*(result.(*params.SettingsResults)) = params.SettingsResults{
				Results: []params.SettingsResult{{
					Settings: params.Settings{
						"some":  "settings",
						"other": "things",
					},
				}},
			}
		case "ReadLocalApplicationSettings":
			c.Assert(arg, tc.DeepEquals, params.RelationUnit{
				Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0",
			})
			c.Assert(result, tc.FitsTypeOf, &params.SettingsResult{})
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
	relUnit, err := rel.Unit(c.Context(), names.NewUnitTag("mysql/0"))
	c.Assert(err, tc.ErrorIsNil)
	return relUnit
}

func (s *relationUnitSuite) TestRelation(c *tc.C) {
	relUnit := s.getRelationUnit(c)
	apiRel := relUnit.Relation()
	c.Assert(apiRel, tc.NotNil)
	c.Assert(apiRel.String(), tc.Equals, "wordpress:db mysql:server")
}

func (s *relationUnitSuite) TestEndpoint(c *tc.C) {
	relUnit := s.getRelationUnit(c)

	apiEndpoint := relUnit.Endpoint()
	c.Assert(apiEndpoint, tc.DeepEquals, uniter.Endpoint{
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

func (s *relationUnitSuite) TestEnterScope(c *tc.C) {
	relUnit := s.getRelationUnit(c)
	err := relUnit.EnterScope(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *relationUnitSuite) TestLeaveScope(c *tc.C) {
	relUnit := s.getRelationUnit(c)
	err := relUnit.LeaveScope(c.Context())
	c.Assert(err, tc.ErrorMatches, "bam")
}

func (s *relationUnitSuite) TestSettings(c *tc.C) {
	relUnit := s.getRelationUnit(c)
	gotSettings, err := relUnit.Settings(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSettings.Map(), tc.DeepEquals, params.Settings{
		"some":  "settings",
		"other": "things",
	})
}

func (s *relationUnitSuite) TestApplicationSettings(c *tc.C) {
	relUnit := s.getRelationUnit(c)
	gotSettings, err := relUnit.ApplicationSettings(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSettings.Map(), tc.DeepEquals, params.Settings{
		"foo": "bar",
		"baz": "1",
	})
}

func (s *relationUnitSuite) TestWatchRelationUnits(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if request == "Stop" || request == "Next" {
			return nil
		}
		c.Assert(request, tc.Equals, "WatchRelationUnits")
		c.Assert(arg, tc.DeepEquals, params.RelationUnits{
			RelationUnits: []params.RelationUnit{
				{Relation: "relation-wordpress.db#mysql.server", Unit: "unit-mysql-0"},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.RelationUnitsWatchResults{})
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
	w, err := client.WatchRelationUnits(c.Context(), tag, names.NewUnitTag("mysql/0"))
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewRelationUnitsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange([]string{"mysql/0"}, []string{"mysql"}, []string{"666"})
}
