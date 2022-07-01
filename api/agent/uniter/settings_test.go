// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/api/agent/uniter"
	basetesting "github.com/juju/juju/v3/api/base/testing"
	"github.com/juju/juju/v3/rpc/params"
	coretesting "github.com/juju/juju/v3/testing"
)

type settingsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&settingsSuite{})

func (s *settingsSuite) TestNewSettingsAndMap(c *gc.C) {
	// Make sure newSettings accepts nil settings.
	settings := uniter.NewSettings("blah", "foo", nil)
	theMap := settings.Map()
	c.Assert(theMap, gc.NotNil)
	c.Assert(theMap, gc.HasLen, 0)

	// And also accepts a populated map, and returns a converted map.
	rawSettings := params.Settings{
		"some":  "settings",
		"other": "stuff",
	}
	settings = uniter.NewSettings("blah", "foo", rawSettings)
	theMap = settings.Map()
	c.Assert(theMap, gc.DeepEquals, rawSettings)
}

func (s *settingsSuite) TestSet(c *gc.C) {
	settings := uniter.NewSettings("blah", "foo", nil)

	settings.Set("foo", "bar")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "bar",
	})
	settings.Set("foo", "qaz")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
	})
	settings.Set("bar", "Cheers")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
		"bar": "Cheers",
	})
}

func (s *settingsSuite) TestDelete(c *gc.C) {
	settings := uniter.NewSettings("blah", "foo", nil)

	settings.Set("foo", "qaz")
	settings.Set("abc", "tink")
	settings.Set("bar", "tonk")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
		"abc": "tink",
		"bar": "tonk",
	})
	settings.Delete("abc")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
		"bar": "tonk",
	})
	settings.Delete("bar")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
	})
	settings.Set("abc", "123")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
		"abc": "123",
	})
	settings.Delete("missing")
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"foo": "qaz",
		"abc": "123",
	})
}

func (s *settingsSuite) TestWrite(c *gc.C) {
	settingsUpdated := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		switch request {
		case "ReadSettings":
			c.Assert(arg, gc.DeepEquals, params.RelationUnits{
				RelationUnits: []params.RelationUnit{{
					Relation: "relation-mysql.database#wordpress.server",
					Unit:     "unit-mysql-0",
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.SettingsResults{})
			*(result.(*params.SettingsResults)) = params.SettingsResults{
				Results: []params.SettingsResult{{
					Settings: params.Settings{
						"some":  "stuff",
						"other": "things",
					},
				}},
			}
		case "UpdateSettings":
			c.Assert(arg, gc.DeepEquals, params.RelationUnitsSettings{
				RelationUnits: []params.RelationUnitSettings{{
					Relation: "relation-mysql.database#wordpress.server",
					Unit:     "unit-mysql-0",
					Settings: params.Settings{
						"foo":   "qaz",
						"other": "days",
						"some":  "",
					},
					ApplicationSettings: params.Settings{"foo": "bar"},
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			settingsUpdated = true
		default:
			c.Fatalf("unexpected api call %q", request)
		}
		return nil
	})
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))

	relUnit := uniter.CreateRelationUnit(client, names.NewRelationTag("mysql:database wordpress:server"), names.NewUnitTag("mysql/0"))
	settings, err := relUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), gc.DeepEquals, params.Settings{
		"some":  "stuff",
		"other": "things",
	})

	settings.Set("some", "bar")
	settings.Delete("foo")
	settings.Delete("some")
	settings.Set("foo", "qaz")
	settings.Set("other", "days")
	err = relUnit.UpdateRelationSettings(settings.FinalResult(), params.Settings{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsUpdated, jc.IsTrue)
}
