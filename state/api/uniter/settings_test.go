// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/uniter"
)

type settingsSuite struct {
	uniterSuite
	commonRelationSuiteMixin
}

var _ = gc.Suite(&settingsSuite{})

func (s *settingsSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	s.commonRelationSuiteMixin.SetUpTest(c, s.uniterSuite)
}

func (s *settingsSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *settingsSuite) TestNewSettingsAndMap(c *gc.C) {
	// Make sure newSettings accepts nil settings.
	settings := uniter.NewSettings(s.uniter, "blah", "foo", nil)
	theMap := settings.Map()
	c.Assert(theMap, gc.NotNil)
	c.Assert(theMap, gc.HasLen, 0)

	// And also accepts a populated map, and returns a converted map.
	rawSettings := params.RelationSettings{
		"some":  "settings",
		"other": "stuff",
	}
	settings = uniter.NewSettings(s.uniter, "blah", "foo", rawSettings)
	theMap = settings.Map()
	c.Assert(theMap, gc.DeepEquals, rawSettings)
}

func (s *settingsSuite) TestSet(c *gc.C) {
	settings := uniter.NewSettings(s.uniter, "blah", "foo", nil)

	settings.Set("foo", "bar")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "bar",
	})
	settings.Set("foo", "qaz")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
	})
	settings.Set("bar", "Cheers")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
		"bar": "Cheers",
	})
}

func (s *settingsSuite) TestDelete(c *gc.C) {
	settings := uniter.NewSettings(s.uniter, "blah", "foo", nil)

	settings.Set("foo", "qaz")
	settings.Set("abc", "tink")
	settings.Set("bar", "tonk")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
		"abc": "tink",
		"bar": "tonk",
	})
	settings.Delete("abc")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
		"bar": "tonk",
	})
	settings.Delete("bar")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
	})
	settings.Set("abc", "123")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
		"abc": "123",
	})
	settings.Delete("missing")
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo": "qaz",
		"abc": "123",
	})
}

func (s *settingsSuite) TestWrite(c *gc.C) {
	wpRelUnit, err := s.stateRelation.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	rawSettings := map[string]interface{}{
		"some":  "stuff",
		"other": "things",
	}
	err = wpRelUnit.EnterScope(rawSettings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, true)

	apiUnit, err := s.uniter.Unit(s.wordpressUnit.Tag())
	c.Assert(err, gc.IsNil)
	apiRelation, err := s.uniter.Relation(s.stateRelation.Tag())
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRelation.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
	settings, err := apiRelUnit.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"some":  "stuff",
		"other": "things",
	})

	settings.Set("some", "bar")
	settings.Delete("foo")
	settings.Delete("some")
	settings.Set("foo", "qaz")
	settings.Set("other", "days")
	err = settings.Write()
	c.Assert(err, gc.IsNil)
	settings, err = apiRelUnit.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings.Map(), gc.DeepEquals, params.RelationSettings{
		"foo":   "qaz",
		"other": "days",
	})
}
